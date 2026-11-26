package daemon_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wkoch/Palantir/internal/copia"
	"github.com/wkoch/Palantir/internal/daemon"
	"github.com/wkoch/Palantir/internal/discos"
	"github.com/wkoch/Palantir/internal/segredos"
	"github.com/wkoch/Palantir/internal/testes"
)

func terminar(t *testing.T, d *daemon.Daemon) daemon.EstadoDaCopia {
	t.Helper()
	for prazo := time.Now().Add(20 * time.Second); time.Now().Before(prazo); time.Sleep(5 * time.Millisecond) {
		if c := d.Copia(); !c.Rodando {
			return c
		}
	}
	t.Fatal("a cópia não terminou")
	return daemon.EstadoDaCopia{}
}

func TestCopiaERestauracaoCompletas(t *testing.T) {
	disco := &testes.Disco{Pasta: testes.Pasta(t)}
	s := testes.Novo(t, testes.Opcoes{Discos: disco})
	s.Importar("contrato.txt", "O aluguel vence no dia 8.")
	x := s.Criar("conversa", "O que há sobre aluguel?", "")
	if err := s.D.Copiar(s.Contexto, testes.Dispositivo); err != nil {
		t.Fatal(err)
	}
	c := terminar(t, s.D)
	if c.Erro != "" || c.Arquivo == "" || c.Disco != "Pendrive de teste" || disco.Desmontagens() != 1 {
		t.Fatalf("%+v %d", c, disco.Desmontagens())
	}
	arquivo := filepath.Join(disco.Pasta, copia.Pasta, c.Arquivo)
	b, err := os.ReadFile(arquivo)
	if err != nil {
		t.Fatal(err)
	}
	for _, segredo := range []string{testes.Chave, s.D.Chave, "segredos.json"} {
		if strings.Contains(string(b), segredo) {
			t.Error("a cópia levou um segredo:", segredo[:4])
		}
	}

	// Numa instalação nova, tudo volta, menos os segredos.
	destino := caminhos(t)
	n, err := copia.Restaurar(s.Contexto, destino, arquivo)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	novo, err := daemon.Novo(s.Contexto, destino, daemon.Opcoes{Discos: disco})
	if err != nil {
		t.Fatal(err)
	}
	defer novo.Fechar()
	voltou, err := novo.Armazem.Obter(s.Contexto, x.ID)
	if err != nil || voltou.Objetivo != x.Objetivo {
		t.Fatal(voltou, err)
	}
	if ok, problemas, err := novo.Acervo.Verificar(s.Contexto); err != nil || ok != 1 || len(problemas) != 0 {
		t.Fatal(ok, problemas, err)
	}
	if achados, err := novo.Acervo.Buscar(s.Contexto, 0, "aluguel", 3); err != nil || len(achados) == 0 {
		t.Error(achados, err)
	}
	if cfg := novo.Config(); cfg.Segredos.Armazenamento != "arquivo" {
		t.Error("a configuração não voltou", cfg.Segredos)
	}
	if v, _ := segredos.Novo("arquivo", destino.Segredos()).Ler(s.Contexto, segredos.OpenRouter); v != "" {
		t.Error("o segredo não devia voltar")
	}
	// Com dados, a restauração é recusada.
	if _, err := copia.Restaurar(s.Contexto, destino, arquivo); !errors.Is(err, copia.ErrNaoVazia) {
		t.Error(err)
	}
}

func TestFalhasDaCopia(t *testing.T) {
	disco := &testes.Disco{Pasta: testes.Pasta(t), Segurar: make(chan struct{})}
	s := testes.Novo(t, testes.Opcoes{Discos: disco})
	if err := s.D.Copiar(s.Contexto, "/dev/outro"); !errors.Is(err, discos.ErrDesconhecido) {
		t.Error(err)
	}
	if err := s.D.Copiar(s.Contexto, testes.Dispositivo); err != nil {
		t.Fatal(err)
	}
	if err := s.D.Copiar(s.Contexto, testes.Dispositivo); !errors.Is(err, daemon.ErrCopiaEmAndamento) {
		t.Error(err)
	}
	disco.FalhaAoDesmontar = errors.New("alvo ocupado")
	close(disco.Segurar)
	// A cópia só está pronta se o disco foi liberado.
	if c := terminar(t, s.D); !strings.Contains(c.Erro, "alvo ocupado") {
		t.Errorf("%+v", c)
	}
	disco.Segurar, disco.FalhaAoMontar = nil, errors.New("sem permissão para montar")
	if err := s.D.Copiar(s.Contexto, testes.Dispositivo); err != nil {
		t.Fatal(err)
	}
	if c := terminar(t, s.D); !strings.Contains(c.Erro, "sem permissão") || c.Arquivo != "" {
		t.Errorf("%+v", c)
	}
}

func TestEncerrarDuranteUmaCopia(t *testing.T) {
	disco := &testes.Disco{Pasta: testes.Pasta(t), Segurar: make(chan struct{})}
	s := testes.Novo(t, testes.Opcoes{Discos: disco})
	if err := s.D.Copiar(s.Contexto, testes.Dispositivo); err != nil {
		t.Fatal(err)
	}
	// O encerramento do teste cancela a cópia e espera por ela.
}
