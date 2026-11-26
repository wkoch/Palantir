package daemon_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/daemon"
	"github.com/wkoch/Palantir/internal/simulador"
	"github.com/wkoch/Palantir/internal/testes"
	"github.com/wkoch/Palantir/internal/textohtml"
	"github.com/wkoch/Palantir/internal/ui"
)

func caminhos(t *testing.T) config.Caminhos {
	t.Helper()
	c, err := config.CaminhosPadrao(testes.Pasta(t))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestMontagemComErros(t *testing.T) {
	ctx := context.Background()
	falha := func(nome string, preparar func(c config.Caminhos)) {
		t.Helper()
		c := caminhos(t)
		if err := c.Criar(); err != nil {
			t.Fatal(err)
		}
		preparar(c)
		if d, err := daemon.Novo(ctx, c, daemon.Opcoes{}); err == nil {
			d.Fechar()
			t.Error(nome, "deveria falhar")
		}
	}
	falha("configuração", func(c config.Caminhos) { os.WriteFile(c.Arquivo(), []byte("x ="), 0o600) })
	falha("diário", func(c config.Caminhos) { os.Mkdir(c.Diario(), 0o700) })
	falha("banco", func(c config.Caminhos) { os.Mkdir(c.Banco(), 0o700) })
	if _, err := daemon.Novo(ctx, config.Caminhos{Config: "/proc/x"}, daemon.Opcoes{}); err == nil {
		t.Error("diretórios")
	}
	// Sem opções, usa o executor de processos e o log padrão.
	c := caminhos(t)
	d, err := daemon.Novo(ctx, c, daemon.Opcoes{})
	if err != nil {
		t.Fatal(err)
	}
	// Um socket que não pode ser criado impede a partida.
	if err := os.WriteFile(filepath.Join(c.Execucao, "proxy-web"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Iniciar(ctx); err == nil {
		t.Error("deveria falhar")
	}
	if err := d.Fechar(); err != nil {
		t.Error(err)
	}
}

func TestReinicioRetomaEPreserva(t *testing.T) {
	ctx := context.Background()
	s := testes.Novo(t, testes.Opcoes{})
	trava := make(chan struct{})
	s.Modelo.Atraso = 0
	s.Modelo.Definir(func(simulador.PedidoRecebido) (simulador.Resposta, bool) {
		<-trava
		return simulador.Resposta{}, false
	})
	x, err := s.D.Gerente.Criar(ctx, "conversa", "", "Continue depois do reinício", nil)
	if err != nil {
		t.Fatal(err)
	}
	for len(s.Modelo.Pedidos()) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	// "Desliga" o daemon no meio da chamada.
	s.D.Gerente.Parar()
	close(trava)
	y, _ := s.D.Armazem.Obter(ctx, x.ID)
	if y.Estado != atividades.Rodando {
		t.Fatal(y.Estado)
	}
	s.Modelo.Definir(nil)
	novo, err := daemon.Novo(ctx, s.Caminhos, daemon.Opcoes{Executor: testes.Memoria()})
	if err != nil {
		t.Fatal(err)
	}
	ctx2, parar := context.WithCancel(ctx)
	esperar, err := novo.Iniciar(ctx2)
	if err != nil {
		t.Fatal(err)
	}
	novo.Gerente.Esperar()
	y, _ = novo.Armazem.Obter(ctx, x.ID)
	if y.Estado != atividades.Pronta {
		t.Error(y.Estado, y.Aviso)
	}
	parar()
	esperar()
	if err := novo.Fechar(); err != nil {
		t.Error(err)
	}
}

func TestOrcamentoMensalPausa(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	s.Modelo.Custo = 3
	if err := s.D.Ajustar(config.Ajustes{Orcamento: &config.Orcamento{MensalUSD: 2.5, PorAtividadeUSD: 10}}); err != nil {
		t.Fatal(err)
	}
	a := s.Criar("conversa", "primeira", "")
	b := s.Criar("conversa", "segunda", "")
	if a.Estado != atividades.Pronta || b.Estado != atividades.Pausada {
		t.Error(a.Estado, b.Estado, b.Aviso)
	}
	if err := s.D.Livro.Verificar(s.Contexto, 0); err == nil {
		t.Error("o acervo também respeita o teto")
	}
	if v, err := s.D.Livro.DaAtividade(s.Contexto, a.ID); err != nil || v != 3 {
		t.Error(v, err)
	}
	if r, err := s.D.Livro.Resumo(s.Contexto, "modelo"); err != nil || len(r) != 1 || r[0].Chamadas != 1 {
		t.Error(r, err)
	}
	if err := s.D.Ajustar(config.Ajustes{Orcamento: &config.Orcamento{MensalUSD: -1}}); err == nil {
		t.Error("ajuste inválido")
	}
	s.D.DB.Close()
	if _, err := s.D.Livro.NoMes(s.Contexto); err == nil {
		t.Error("banco fechado")
	}
	if s.D.Livro.Verificar(s.Contexto, 1) == nil {
		t.Error("banco fechado")
	}
	if _, err := s.D.Livro.Resumo(s.Contexto, "x"); err == nil {
		t.Error("banco fechado")
	}
	if s.D.Exportar(s.Contexto, nil) == nil {
		t.Error("banco fechado")
	}
	if _, err := s.D.Confirmacoes.Pendentes(s.Contexto); err == nil {
		t.Error("banco fechado")
	}
}

func TestInterfaceEmbutidaETextoHTML(t *testing.T) {
	if ui.Arquivos() == nil {
		t.Error("sem interface")
	}
	titulo, texto, err := textohtml.Extrair(errReader{})
	if err == nil {
		t.Error(titulo, texto)
	}
	if got := textohtml.Arrumar("  a   b \n\n\n\n c \n"); got != "a b\n\nc" {
		t.Errorf("%q", got)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, os.ErrClosed }
