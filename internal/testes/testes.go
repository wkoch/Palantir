// Package testes monta um Palantír completo dentro do processo de teste: o
// daemon real, o proxy de saída real, as ferramentas MCP reais ligadas em
// memória, um IMAP em memória e um modelo simulado.
package testes

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/daemon"
	"github.com/wkoch/Palantir/internal/ferramentas"
	"github.com/wkoch/Palantir/internal/segredos"
	"github.com/wkoch/Palantir/internal/servidores"
	"github.com/wkoch/Palantir/internal/simulador"
)

// Chave é a chave fictícia usada nos testes.
const Chave = "chave-de-teste-ultrassecreta"

// Memoria executa as ferramentas MCP no próprio processo, com o mesmo ambiente
// que receberiam num contêiner.
func Memoria() ferramentas.Memoria {
	return ferramentas.Memoria{Criar: func(_ context.Context, e ferramentas.Execucao) (*mcp.Server, error) {
		return servidores.Novo(e.Servidor, func(nome string) string {
			switch nome {
			case ferramentas.EnvProxy:
				return e.ProxySocket
			case ferramentas.EnvEntrada:
				return e.Entrada
			}
			return e.Env[nome]
		})
	}}
}

// Resolver resolve qualquer nome para a própria máquina e anota o que foi
// consultado. O proxy só resolve um nome depois de liberá-lo.
type Resolver struct {
	mu        sync.Mutex
	consultas []string
}

// LookupNetIP implementa proxy.Resolver.
func (r *Resolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.consultas = append(r.consultas, host)
	return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
}

// Consultas devolve os nomes resolvidos.
func (r *Resolver) Consultas() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.consultas)
}

// Opcoes ajusta a montagem.
type Opcoes struct {
	Emails []simulador.Email
	// SemChave não grava a chave do OpenRouter.
	SemChave bool
	// Modelos e Config acrescentam linhas à seção [modelos] e ao fim do arquivo.
	Modelos  string
	Config   string
	Executor ferramentas.Executor
	// URLModelos e Chave trocam o modelo simulado por um serviço real.
	URLModelos string
	Chave      string
}

// Sistema é um Palantír de teste.
type Sistema struct {
	T        testing.TB
	D        *daemon.Daemon
	Modelo   *simulador.Modelo
	Correio  *simulador.Correio
	Web      *httptest.Server
	Resolver *Resolver
	Caminhos config.Caminhos
	Contexto context.Context
}

// Pasta cria um diretório temporário curto, como exigem os sockets Unix.
func Pasta(t testing.TB) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// Novo monta e inicia o sistema.
func Novo(t testing.TB, o Opcoes) *Sistema {
	t.Helper()
	ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancelar)
	falhar := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	s := &Sistema{T: t, Modelo: &simulador.Modelo{Custo: 0.001}, Resolver: &Resolver{}, Contexto: ctx}
	modelo := httptest.NewServer(s.Modelo)
	t.Cleanup(modelo.Close)
	if o.Emails == nil {
		o.Emails = simulador.Emails
	}
	var err error
	s.Correio, err = simulador.NovoCorreio("127.0.0.1:0", o.Emails, nil)
	falhar(err)
	t.Cleanup(func() { _ = s.Correio.Fechar() })
	s.Web = httptest.NewServer(simulador.Web(s.Correio))
	t.Cleanup(s.Web.Close)

	var niveis string
	if o.URLModelos == "" {
		o.URLModelos = modelo.URL + "/api/v1"
		// Nomes distintos permitem conferir qual nível cada tarefa usa.
		niveis = "barato = \"teste/barato\"\nprincipal = \"teste/principal\"\nforte = \"teste/forte\""
		if strings.Contains(o.Modelos, "barato =") {
			niveis = ""
		}
	}
	if o.Chave == "" {
		o.Chave = Chave
	}
	dir := Pasta(t)
	s.Caminhos, err = config.CaminhosPadrao(dir)
	falhar(err)
	falhar(s.Caminhos.Criar())
	cfg := `[servidor]
endereco = "127.0.0.1:0"
[modelos]
url = "` + o.URLModelos + `"
` + niveis + `
` + o.Modelos + `
[ferramentas]
isolamento = "nenhum"
permitir_enderecos_locais = true
[ferramentas.web]
hosts = ["127.0.0.1", "liberado.test"]
[imap]
servidor = "127.0.0.1"
porta = ` + strconv.Itoa(s.Correio.Porta()) + `
usuario = "` + simulador.Usuario + `"
seguranca = "nenhum"
[acervo]
pasta_entrada = "` + dir + `/Entrada"
intervalo_segundos = 3600
` + o.Config
	falhar(os.WriteFile(s.Caminhos.Arquivo(), []byte(cfg), 0o600))
	cofre := segredos.Novo("arquivo", s.Caminhos.Segredos())
	if !o.SemChave {
		falhar(cofre.Definir(ctx, segredos.OpenRouter, o.Chave))
	}
	falhar(cofre.Definir(ctx, segredos.SenhaIMAP, simulador.Senha))

	var executor ferramentas.Executor = Memoria()
	if o.Executor != nil {
		executor = o.Executor
	}
	s.D, err = daemon.Novo(ctx, s.Caminhos, daemon.Opcoes{Executor: executor, Resolver: s.Resolver,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	falhar(err)
	s.D.Roteador.Espera = time.Millisecond
	ctxDaemon, parar := context.WithCancel(ctx)
	esperar, err := s.D.Iniciar(ctxDaemon)
	falhar(err)
	t.Cleanup(func() {
		parar()
		esperar()
		if err := s.D.Fechar(); err != nil {
			t.Error(err)
		}
	})
	return s
}

// Importar grava um documento no acervo e o processa.
func (s *Sistema) Importar(nome, conteudo string) int64 {
	s.T.Helper()
	d, _, err := s.D.Acervo.Importar(s.Contexto, strings.NewReader(conteudo), nome, "teste")
	if err != nil {
		s.T.Fatal(err)
	}
	if err := s.D.Acervo.Processar(s.Contexto); err != nil {
		s.T.Fatal(err)
	}
	return d.ID
}

// Criar inicia uma atividade e espera o trabalho terminar.
func (s *Sistema) Criar(tipo, objetivo string, opcoes string) atividades.Atividade {
	s.T.Helper()
	var o []byte
	if opcoes != "" {
		o = []byte(opcoes)
	}
	x, err := s.D.Gerente.Criar(s.Contexto, tipo, "", objetivo, o)
	if err != nil {
		s.T.Fatal(err)
	}
	return s.Esperar(x.ID)
}

// Esperar aguarda as execuções e devolve a atividade.
func (s *Sistema) Esperar(id int64) atividades.Atividade {
	s.T.Helper()
	s.D.Gerente.Esperar()
	x, err := s.D.Armazem.Obter(s.Contexto, id)
	if err != nil {
		s.T.Fatal(err)
	}
	return x
}

// Agir executa uma ação e espera o trabalho que ela agendar.
func (s *Sistema) Agir(id int64, acao, corpo string) (any, atividades.Atividade) {
	s.T.Helper()
	r, err := s.D.Gerente.Agir(s.Contexto, id, acao, []byte(corpo))
	if err != nil {
		s.T.Fatalf("ação %s: %v", acao, err)
	}
	return r, s.Esperar(id)
}

// Dados decodifica o estado de uma atividade.
func Dados[T any](t testing.TB, x atividades.Atividade) T {
	t.Helper()
	d, err := atividades.Dados[T](x)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// PortaFechada devolve um endereço local sem ninguém escutando.
func PortaFechada(t testing.TB) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endereco := l.Addr().String()
	_ = l.Close()
	return endereco
}

// Obter faz um GET e devolve o código e o corpo.
func Obter(t testing.TB, url string) (int, string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(b)
}
