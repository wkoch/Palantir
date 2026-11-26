package ferramentas

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wkoch/Palantir/internal/config"
)

func processo(cfg config.Ferramentas) *Processo {
	return &Processo{Config: func() config.Ferramentas { return cfg }}
}

func TestComandoPodmanIsolaAFerramenta(t *testing.T) {
	p := processo(config.Ferramentas{Isolamento: "podman", Imagem: "img:1"})
	cmd, err := p.comando(context.Background(), Execucao{Servidor: IMAP, ProxySocket: "/run/u/proxy-imap/proxy.sock",
		Env: map[string]string{"B": "dois", "SENHA": "muito-secreta"}})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(cmd.Args, " ")
	for _, quer := range []string{"run --rm --interactive --pull=never", "--network=none", "--read-only", "--cap-drop=all",
		"--security-opt=no-new-privileges", "--volume=/run/u/proxy-imap:/run/proxy", "--env=PALANTIR_PROXY=/run/proxy/proxy.sock",
		"--env=B --env=SENHA img:1 palantir-tool imap"} {
		if !strings.Contains(args, quer) {
			t.Errorf("falta %q em %s", quer, args)
		}
	}
	if strings.Contains(args, "muito-secreta") {
		t.Error("o segredo apareceu na linha de comando")
	}
	if !slices.Contains(cmd.Env, "SENHA=muito-secreta") {
		t.Error("o segredo precisa chegar pelo ambiente")
	}
	cmd, err = p.comando(context.Background(), Execucao{Servidor: Extrair, Entrada: "/dados/originais/abc"})
	if err != nil {
		t.Fatal(err)
	}
	args = strings.Join(cmd.Args, " ")
	if !strings.Contains(args, "--volume=/dados/originais/abc:/entrada/arquivo:ro") || strings.Contains(args, "/run/proxy") {
		t.Error("a extração monta só o original, sem rede:", args)
	}
	if _, err := p.comando(context.Background(), Execucao{Servidor: "sh"}); err == nil {
		t.Error("servidor desconhecido")
	}
}

func TestComandoDireto(t *testing.T) {
	t.Setenv("SEGREDO_DO_DAEMON", "nao-herdar")
	t.Setenv("GOCOVERDIR", "/tmp/c")
	cmd, err := processo(config.Ferramentas{Isolamento: "nenhum", Binario: "/bin/x"}).comando(context.Background(),
		Execucao{Servidor: Web, ProxySocket: "/s", Entrada: "/e", Env: map[string]string{"A": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cmd.Args, []string{"/bin/x", "web"}) {
		t.Error(cmd.Args)
	}
	for _, quer := range []string{"A=1", "PALANTIR_PROXY=/s", "PALANTIR_ENTRADA=/e", "GOCOVERDIR=/tmp/c"} {
		if !slices.Contains(cmd.Env, quer) {
			t.Error("falta", quer, cmd.Env)
		}
	}
	if strings.Contains(strings.Join(cmd.Env, " "), "nao-herdar") {
		t.Error("o ambiente do daemon vazou para a ferramenta")
	}
}

// TestProcessoReal compila o palantir-tool e o executa como o daemon faz.
func TestProcessoReal(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "palantir-tool")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/palantir-tool").CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	entrada := filepath.Join(t.TempDir(), "doc")
	if err := os.WriteFile(entrada, []byte("conteúdo do documento"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := processo(config.Ferramentas{Isolamento: "nenhum", Binario: bin})
	var r struct {
		Paginas []struct{ Texto string } `json:"paginas"`
	}
	ctx := context.Background()
	err := Chamar(ctx, p, Execucao{Servidor: Extrair, Entrada: entrada}, "extrair", map[string]any{"nome": "d.txt"}, &r)
	if err != nil || len(r.Paginas) != 1 || r.Paginas[0].Texto != "conteúdo do documento" {
		t.Fatal(r, err)
	}
	err = Chamar(ctx, p, Execucao{Servidor: Extrair}, "extrair", map[string]any{"nome": "d.txt"}, &r)
	if !errors.Is(err, ErrDaFerramenta) {
		t.Error(err)
	}
	// Falha ao iniciar mostra a saída de erro da ferramenta.
	err = Chamar(ctx, p, Execucao{Servidor: IMAP}, "buscar", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "palantir-tool:") {
		t.Error(err)
	}
	if err := Chamar(ctx, p, Execucao{Servidor: "outra"}, "x", nil, nil); err == nil {
		t.Error("desconhecida")
	}
	p = processo(config.Ferramentas{Isolamento: "nenhum", Binario: "/nao/existe"})
	if err := Chamar(ctx, p, Execucao{Servidor: Web}, "x", nil, nil); err == nil {
		t.Error("binário ausente")
	}
}

func TestMemoriaErros(t *testing.T) {
	m := Memoria{Criar: func(context.Context, Execucao) (*mcp.Server, error) { return nil, errors.New("não") }}
	if _, err := m.Abrir(context.Background(), Execucao{}); err == nil {
		t.Error("deveria falhar")
	}
	m.Criar = func(context.Context, Execucao) (*mcp.Server, error) {
		return mcp.NewServer(&mcp.Implementation{Name: "x", Version: "1"}, nil), nil
	}
	cancelado, cancelar := context.WithCancel(context.Background())
	cancelar()
	if s, err := m.Abrir(cancelado, Execucao{}); err == nil {
		s.Close()
		t.Error("contexto cancelado")
	}
	s, err := m.Abrir(context.Background(), Execucao{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Chamar(context.Background(), "nada", nil, nil) == nil {
		t.Error("ferramenta inexistente")
	}
}

func TestCauda(t *testing.T) {
	c := &cauda{}
	c.Write([]byte(strings.Repeat("a", 5000)))
	c.Write([]byte(" fim \n"))
	if s := c.String(); len(s) > 4096 || !strings.HasSuffix(s, "fim") {
		t.Error(len(s))
	}
}
