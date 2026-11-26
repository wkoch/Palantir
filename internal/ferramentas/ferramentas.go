// Package ferramentas executa os servidores MCP do Palantír, normalmente em
// contêineres Podman sem rede.
package ferramentas

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wkoch/Palantir/internal/config"
)

// Servidores MCP disponíveis no binário palantir-tool.
const (
	Web     = "web"
	IMAP    = "imap"
	Extrair = "extrair"
)

// Variáveis de ambiente que o daemon entrega às ferramentas.
const (
	EnvProxy   = "PALANTIR_PROXY"
	EnvEntrada = "PALANTIR_ENTRADA"
	EnvBusca   = "PALANTIR_BUSCA"
)

// Execucao descreve o que uma sessão de ferramenta recebe. Nada além disso fica
// acessível a ela.
type Execucao struct {
	Servidor string
	// Env leva configuração e segredos da ferramenta, fora da linha de comando.
	Env map[string]string
	// ProxySocket é o socket de saída no host; vazio deixa a ferramenta sem rede.
	ProxySocket string
	// Entrada é um arquivo do host, montado somente para leitura.
	Entrada string
}

// Sessao é uma conexão MCP aberta.
type Sessao interface {
	// Chamar executa uma ferramenta e decodifica o resultado em destino.
	Chamar(ctx context.Context, ferramenta string, args, destino any) error
	Close() error
}

// Executor abre sessões.
type Executor interface {
	Abrir(ctx context.Context, e Execucao) (Sessao, error)
}

// ErrDaFerramenta marca um erro informado pela própria ferramenta.
var ErrDaFerramenta = errors.New("a ferramenta informou um erro")

// Processo executa cada sessão num processo novo: um contêiner Podman ou, em
// desenvolvimento, o binário direto no host.
type Processo struct {
	Config func() config.Ferramentas
	// Prazo limita a duração de cada sessão.
	Prazo time.Duration
}

const (
	proxyNoConteiner   = "/run/proxy"
	entradaNoConteiner = "/entrada/arquivo"
)

// comando monta o processo da sessão.
func (p *Processo) comando(ctx context.Context, e Execucao) (*exec.Cmd, error) {
	if !slices.Contains([]string{Web, IMAP, Extrair}, e.Servidor) {
		return nil, fmt.Errorf("servidor de ferramentas desconhecido: %q", e.Servidor)
	}
	cfg := p.Config()
	nomes := make([]string, 0, len(e.Env))
	for n := range e.Env {
		nomes = append(nomes, n)
	}
	slices.Sort(nomes)

	if cfg.Isolamento == "nenhum" {
		cmd := exec.CommandContext(ctx, cfg.Binario, e.Servidor)
		// O ambiente do daemon não é herdado.
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.TempDir(), "LANG=C.UTF-8"}
		if v := os.Getenv("GOCOVERDIR"); v != "" {
			cmd.Env = append(cmd.Env, "GOCOVERDIR="+v)
		}
		for _, n := range nomes {
			cmd.Env = append(cmd.Env, n+"="+e.Env[n])
		}
		if e.ProxySocket != "" {
			cmd.Env = append(cmd.Env, EnvProxy+"="+e.ProxySocket)
		}
		if e.Entrada != "" {
			cmd.Env = append(cmd.Env, EnvEntrada+"="+e.Entrada)
		}
		return cmd, nil
	}

	var sufixo [6]byte
	if _, err := rand.Read(sufixo[:]); err != nil {
		return nil, err
	}
	args := []string{"run", "--rm", "--interactive", "--pull=never",
		"--name=palantir-" + e.Servidor + "-" + hex.EncodeToString(sufixo[:]),
		"--network=none", "--read-only", "--cap-drop=all",
		"--security-opt=no-new-privileges", "--security-opt=label=disable",
		"--pids-limit=256", "--memory=1g", "--tmpfs=/tmp:size=512m",
	}
	if e.ProxySocket != "" {
		args = append(args, "--volume="+filepath.Dir(e.ProxySocket)+":"+proxyNoConteiner,
			"--env="+EnvProxy+"="+proxyNoConteiner+"/"+filepath.Base(e.ProxySocket))
	}
	if e.Entrada != "" {
		args = append(args, "--volume="+e.Entrada+":"+entradaNoConteiner+":ro",
			"--env="+EnvEntrada+"="+entradaNoConteiner)
	}
	// "--env NOME" copia o valor do ambiente do podman: segredos não aparecem
	// na lista de processos.
	for _, n := range nomes {
		args = append(args, "--env="+n)
	}
	args = append(args, cfg.Imagem, "palantir-tool", e.Servidor)
	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Env = os.Environ()
	for _, n := range nomes {
		cmd.Env = append(cmd.Env, n+"="+e.Env[n])
	}
	return cmd, nil
}

// cauda guarda o fim da saída de erro de um processo.
type cauda struct {
	mu sync.Mutex
	b  []byte
}

func (c *cauda) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.b = append(c.b, p...)
	if excesso := len(c.b) - 4096; excesso > 0 {
		c.b = c.b[excesso:]
	}
	return len(p), nil
}

func (c *cauda) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(string(c.b))
}

// Abrir implementa Executor.
func (p *Processo) Abrir(ctx context.Context, e Execucao) (Sessao, error) {
	prazo := p.Prazo
	if prazo == 0 {
		prazo = 5 * time.Minute
	}
	ctx, cancelar := context.WithTimeout(ctx, prazo)
	cmd, err := p.comando(ctx, e)
	if err != nil {
		cancelar()
		return nil, err
	}
	erros := &cauda{}
	cmd.Stderr = erros
	cmd.WaitDelay = 5 * time.Second
	s, err := Conectar(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: 2 * time.Second})
	if err != nil {
		cancelar()
		return nil, fmt.Errorf("iniciar a ferramenta %s: %w %s", e.Servidor, err, erros)
	}
	s.(*sessao).fim = cancelar
	return s, nil
}

// Conectar abre uma sessão MCP sobre um transporte.
func Conectar(ctx context.Context, t mcp.Transport) (Sessao, error) {
	cliente := mcp.NewClient(&mcp.Implementation{Name: "palantird", Version: "1"}, nil)
	cs, err := cliente.Connect(ctx, t, nil)
	if err != nil {
		return nil, err
	}
	return &sessao{cs: cs}, nil
}

type sessao struct {
	cs  *mcp.ClientSession
	fim context.CancelFunc
}

func (s *sessao) Chamar(ctx context.Context, ferramenta string, args, destino any) error {
	res, err := s.cs.CallTool(ctx, &mcp.CallToolParams{Name: ferramenta, Arguments: args})
	if err != nil {
		return fmt.Errorf("ferramenta %s: %w", ferramenta, err)
	}
	if res.IsError {
		var b strings.Builder
		for _, c := range res.Content {
			if t, ok := c.(*mcp.TextContent); ok {
				b.WriteString(t.Text)
			}
		}
		return fmt.Errorf("%w: %s: %s", ErrDaFerramenta, ferramenta, b.String())
	}
	if destino == nil {
		return nil
	}
	bruto, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return err
	}
	return json.Unmarshal(bruto, destino)
}

func (s *sessao) Close() error {
	err := s.cs.Close()
	if s.fim != nil {
		s.fim()
	}
	var saida *exec.ExitError
	if errors.As(err, &saida) || errors.Is(err, io.ErrClosedPipe) {
		return nil
	}
	return err
}

// Chamar abre uma sessão, faz uma chamada e fecha.
func Chamar(ctx context.Context, x Executor, e Execucao, ferramenta string, args, destino any) error {
	s, err := x.Abrir(ctx, e)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Chamar(ctx, ferramenta, args, destino)
}

// Memoria executa servidores no próprio processo. Serve aos testes.
type Memoria struct {
	Criar func(ctx context.Context, e Execucao) (*mcp.Server, error)
}

// Abrir implementa Executor.
func (m Memoria) Abrir(ctx context.Context, e Execucao) (Sessao, error) {
	srv, err := m.Criar(ctx, e)
	if err != nil {
		return nil, err
	}
	doServidor, doCliente := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, doServidor, nil)
	if err != nil {
		return nil, err
	}
	s, err := Conectar(ctx, doCliente)
	if err != nil {
		ss.Close()
		return nil, err
	}
	return s, nil
}
