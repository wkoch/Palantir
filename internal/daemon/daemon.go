// Package daemon monta o palantird: liga configuração, banco, ferramentas,
// atividades e servidor.
package daemon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wkoch/Palantir/internal/acervo"
	"github.com/wkoch/Palantir/internal/agente"
	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/banco"
	"github.com/wkoch/Palantir/internal/capacidades"
	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/confirmacoes"
	"github.com/wkoch/Palantir/internal/conversa"
	"github.com/wkoch/Palantir/internal/diario"
	"github.com/wkoch/Palantir/internal/discos"
	"github.com/wkoch/Palantir/internal/email"
	"github.com/wkoch/Palantir/internal/estudo"
	"github.com/wkoch/Palantir/internal/eventos"
	"github.com/wkoch/Palantir/internal/ferramentas"
	"github.com/wkoch/Palantir/internal/modelos"
	"github.com/wkoch/Palantir/internal/orcamento"
	"github.com/wkoch/Palantir/internal/politica"
	"github.com/wkoch/Palantir/internal/proxy"
	"github.com/wkoch/Palantir/internal/relatorio"
	"github.com/wkoch/Palantir/internal/segredos"
)

// Daemon reúne os módulos em execução.
type Daemon struct {
	Caminhos     config.Caminhos
	DB           *sql.DB
	Diario       *diario.Diario
	Cofre        segredos.Cofre
	Livro        *orcamento.Livro
	Roteador     *modelos.Roteador
	Acervo       *acervo.Acervo
	Armazem      *atividades.Armazem
	Gerente      *atividades.Gerente
	Confirmacoes *confirmacoes.Fila
	Eventos      *eventos.Central
	Log          *slog.Logger
	// Chave é o segredo que a interface apresenta à API.
	Chave string
	// Discos são os destinos das cópias de segurança.
	Discos Discos

	mu    sync.RWMutex
	cfg   config.Config
	copia EstadoDaCopia
	// vida termina quando o daemon para; copias aguarda o que estiver gravando.
	vida   context.Context
	copias sync.WaitGroup

	proxies []*proxy.Proxy
	vigia   *acervo.Vigia
}

// Opcoes ajusta a montagem, principalmente para testes.
type Opcoes struct {
	// Executor substitui a execução das ferramentas.
	Executor ferramentas.Executor
	Log      *slog.Logger
	// Resolver substitui a resolução de nomes do proxy de saída.
	Resolver proxy.Resolver
	// Discos substitui o acesso aos discos.
	Discos Discos
}

// Config devolve a configuração vigente.
func (d *Daemon) Config() config.Config {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg
}

// Ajustar grava os ajustes feitos pelo usuário e passa a usá-los.
func (d *Daemon) Ajustar(a config.Ajustes) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	novo, err := config.GravarAjustes(d.Caminhos, a)
	if err != nil {
		return err
	}
	d.cfg = novo
	d.Eventos.Publicar(eventos.Evento{Tipo: eventos.Ajustes})
	return d.Diario.Registrar(diario.Entrada{Tipo: "sistema", Nome: "ajustes", Origem: "usuario", Decisao: "ok"})
}

func (d *Daemon) socket(ferramenta string) string {
	return filepath.Join(d.Caminhos.Execucao, "proxy-"+ferramenta, "proxy.sock")
}

// Novo abre os arquivos e monta os módulos. Nada roda até Iniciar.
func Novo(ctx context.Context, caminhos config.Caminhos, o Opcoes) (*Daemon, error) {
	if err := caminhos.Criar(); err != nil {
		return nil, err
	}
	cfg, err := config.Carregar(caminhos)
	if err != nil {
		return nil, err
	}
	d := &Daemon{Caminhos: caminhos, cfg: cfg, Eventos: eventos.Nova(), Log: o.Log}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Chave, err = caminhos.LerChaveDeAcesso(); err != nil {
		return nil, fmt.Errorf("chave de acesso: %w", err)
	}
	if d.Diario, err = diario.Abrir(caminhos.Diario()); err != nil {
		return nil, fmt.Errorf("diário: %w", err)
	}
	if d.DB, err = banco.Abrir(ctx, caminhos.Banco()); err != nil {
		return nil, fmt.Errorf("banco: %w", err)
	}
	d.Discos, d.vida = o.Discos, context.Background()
	if d.Discos == nil {
		// As variáveis servem aos testes de ponta a ponta.
		d.Discos = discos.Sistema{Lsblk: os.Getenv("PALANTIR_LSBLK"), Udisksctl: os.Getenv("PALANTIR_UDISKSCTL")}
	}
	d.Cofre = segredos.Novo(cfg.Segredos.Armazenamento, caminhos.Segredos())
	d.Livro = orcamento.Novo(d.DB, func() float64 { return d.Config().Orcamento.MensalUSD })
	d.Roteador = &modelos.Roteador{Config: func() config.Modelos { return d.Config().Modelos },
		Cofre: d.Cofre, Livro: d.Livro, Diario: d.Diario, DB: d.DB}
	d.Confirmacoes = &confirmacoes.Fila{DB: d.DB, Diario: d.Diario}

	executor := o.Executor
	if executor == nil {
		executor = &ferramentas.Processo{Config: func() config.Ferramentas { return d.Config().Ferramentas }}
	}
	novoProxy := func(nome string, regras func() []proxy.Regra) *proxy.Proxy {
		p := &proxy.Proxy{Ferramenta: nome, Diario: d.Diario, Fixas: regras, Resolver: o.Resolver,
			PermitirLocais: func() bool { return d.Config().Ferramentas.PermitirEnderecosLocais }}
		d.proxies = append(d.proxies, p)
		return p
	}
	proxyWeb := novoProxy(ferramentas.Web, func() []proxy.Regra {
		var regras []proxy.Regra
		for _, h := range d.Config().Ferramentas.Web.Hosts {
			regras = append(regras, proxy.Regra{Host: h})
		}
		return regras
	})
	// A ferramenta de e-mail só alcança o servidor configurado, que pode estar
	// na rede local.
	novoProxy(ferramentas.IMAP, func() []proxy.Regra {
		imap := d.Config().IMAP
		if imap.Servidor == "" {
			return nil
		}
		return []proxy.Regra{{Host: imap.Servidor, Portas: []int{imap.Porta}, Local: true}}
	})

	d.Acervo = &acervo.Acervo{DB: d.DB, Originais: caminhos.Originais(), Executor: executor, Modelos: d.Roteador,
		Privadas: []string{caminhos.Config, caminhos.Dados, caminhos.Execucao},
		Config:   func() config.Modelos { return d.Config().Modelos }, Diario: d.Diario,
		Notificar: func() { d.Eventos.Publicar(eventos.Evento{Tipo: eventos.Acervo}) }}
	d.vigia = &acervo.Vigia{Acervo: d.Acervo,
		Pasta: func() string { return config.Expandir(d.Config().Acervo.PastaEntrada) }}

	d.Armazem = &atividades.Armazem{DB: d.DB, Notificar: func(id int64) {
		d.Eventos.Publicar(eventos.Evento{Tipo: eventos.Atividade, Atividade: id})
	}}
	d.Gerente = atividades.NovoGerente(d.Armazem, 4, func() float64 { return d.Config().Orcamento.PorAtividadeUSD })
	d.Gerente.Log = d.Log

	web := &capacidades.Web{Executor: executor, Proxy: proxyWeb, Socket: d.socket(ferramentas.Web),
		Busca: func() string { return d.Config().Busca.URL }}
	hosts := func() []string { return d.Config().Ferramentas.Web.Hosts }
	ag := &agente.Agente{Modelos: d.Roteador, Decisor: d.Roteador, Confirmacoes: d.Confirmacoes, Diario: d.Diario,
		HostsLiberados: hosts, Juiz: func() bool { return d.Config().Modelos.Juiz },
		Ferramentas: capacidades.Ferramentas(d.Acervo, web)}

	d.Gerente.Registrar(politica.Conversa, &conversa.Executor{Armazem: d.Armazem, Agente: ag})
	d.Gerente.Registrar(politica.Relatorio, &relatorio.Executor{Armazem: d.Armazem, Modelos: d.Roteador,
		Acervo: d.Acervo, Web: web, Confirmacoes: d.Confirmacoes, Diario: d.Diario, HostsLiberados: hosts})
	d.Gerente.Registrar(politica.Triagem, &email.Executor{Armazem: d.Armazem, DB: d.DB, Modelos: d.Roteador,
		Decisor:  d.Roteador,
		Executor: executor, Cofre: d.Cofre, Diario: d.Diario, Socket: d.socket(ferramentas.IMAP),
		Config: func() config.IMAP { return d.Config().IMAP }})
	d.Gerente.Registrar(politica.Estudo, &estudo.Executor{Armazem: d.Armazem, DB: d.DB, Modelos: d.Roteador,
		Acervo: d.Acervo})
	return d, nil
}

// Iniciar abre os sockets de saída, retoma as atividades e processa o acervo
// até o contexto acabar. A função devolvida aguarda o encerramento.
func (d *Daemon) Iniciar(ctx context.Context) (func(), error) {
	for _, p := range d.proxies {
		caminho := d.socket(p.Ferramenta)
		if err := mkdirPrivado(filepath.Dir(caminho)); err != nil {
			return nil, err
		}
		if err := p.Escutar(caminho); err != nil {
			return nil, fmt.Errorf("proxy de saída (%s): %w", p.Ferramenta, err)
		}
	}
	if err := d.Gerente.Retomar(ctx); err != nil {
		return nil, fmt.Errorf("retomar atividades: %w", err)
	}
	ctx, cancelar := context.WithCancel(ctx)
	d.mu.Lock()
	d.vida = ctx
	d.mu.Unlock()
	feito := make(chan struct{})
	go func() {
		defer close(feito)
		d.Acervo.Trabalhar(ctx, d.vigia, func() time.Duration {
			return time.Duration(d.Config().Acervo.IntervaloSegundos) * time.Second
		}, func(err error) { d.Log.Warn("acervo", "erro", err) })
	}()
	if err := d.Diario.Registrar(diario.Entrada{Tipo: "sistema", Nome: "palantird", Decisao: "iniciado"}); err != nil {
		cancelar()
		return nil, err
	}
	return func() {
		cancelar()
		<-feito
		d.copias.Wait()
	}, nil
}

// Fechar interrompe o trabalho e fecha os arquivos.
func (d *Daemon) Fechar() error {
	d.copias.Wait()
	d.Gerente.Parar()
	var erros []error
	for _, p := range d.proxies {
		erros = append(erros, p.Fechar())
	}
	erros = append(erros, d.DB.Close())
	return errors.Join(erros...)
}

// DecidirConfirmacao registra a resposta do usuário e avisa a atividade.
func (d *Daemon) DecidirConfirmacao(ctx context.Context, id int64, aprovar bool) error {
	c, err := d.Confirmacoes.Decidir(ctx, id, aprovar)
	if err != nil {
		return err
	}
	d.Eventos.Publicar(eventos.Evento{Tipo: eventos.Atividade, Atividade: c.Atividade})
	return d.agirInterno(ctx, c)
}
