// Package segredos guarda chaves e senhas. Só o daemon lê os valores, que
// nunca entram no contexto de um modelo nem saem pela interface.
package segredos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

// Nomes dos segredos que o Palantír usa.
const (
	OpenRouter = "openrouter"
	SenhaIMAP  = "imap"
)

// Conhecidos lista os segredos aceitos.
var Conhecidos = []string{OpenRouter, SenhaIMAP}

// ErrDesconhecido indica um nome fora de Conhecidos.
var ErrDesconhecido = errors.New("segredo desconhecido")

// Cofre guarda segredos por nome. Ler um segredo ausente devolve "".
type Cofre interface {
	Ler(ctx context.Context, nome string) (string, error)
	Definir(ctx context.Context, nome, valor string) error
	Remover(ctx context.Context, nome string) error
}

// Validar confere o nome antes de qualquer operação vinda de fora.
func Validar(nome string) error {
	if !slices.Contains(Conhecidos, nome) {
		return fmt.Errorf("%w: %q", ErrDesconhecido, nome)
	}
	return nil
}

// Novo cria o cofre indicado pela configuração.
func Novo(armazenamento, arquivo string) Cofre {
	if armazenamento == "secret-service" {
		return &SecretService{Programa: "secret-tool"}
	}
	return &Arquivo{Caminho: arquivo}
}

// Arquivo guarda os segredos num JSON legível só pelo dono. A proteção em
// repouso vem do disco cifrado com LUKS.
type Arquivo struct {
	Caminho string
	mu      sync.Mutex
}

func (a *Arquivo) carregar() (map[string]string, error) {
	b, err := os.ReadFile(a.Caminho)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("arquivo de segredos corrompido: %w", err)
	}
	return m, nil
}

func (a *Arquivo) gravar(m map[string]string) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := a.Caminho + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.Caminho)
}

// Ler implementa Cofre.
func (a *Arquivo) Ler(_ context.Context, nome string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	m, err := a.carregar()
	if err != nil {
		return "", err
	}
	return m[nome], nil
}

// Definir implementa Cofre.
func (a *Arquivo) Definir(_ context.Context, nome, valor string) error {
	if err := Validar(nome); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	m, err := a.carregar()
	if err != nil {
		return err
	}
	m[nome] = valor
	return a.gravar(m)
}

// Remover implementa Cofre.
func (a *Arquivo) Remover(_ context.Context, nome string) error {
	if err := Validar(nome); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	m, err := a.carregar()
	if err != nil {
		return err
	}
	delete(m, nome)
	return a.gravar(m)
}

// SecretService usa o secret-tool (libsecret) para falar com o keyring.
type SecretService struct{ Programa string }

// prazoDoKeyring evita esperar para sempre por um pedido de senha que não tem
// onde aparecer.
var prazoDoKeyring = 30 * time.Second

func (s *SecretService) executar(ctx context.Context, entrada string, args ...string) (string, error) {
	ctx, cancelar := context.WithTimeout(ctx, prazoDoKeyring)
	defer cancelar()
	cmd := exec.CommandContext(ctx, s.Programa, args...)
	cmd.Stdin = strings.NewReader(entrada)
	var saida, erros bytes.Buffer
	cmd.Stdout, cmd.Stderr = &saida, &erros
	err := cmd.Run()
	var saiu *exec.ExitError
	if errors.As(err, &saiu) && args[0] == "lookup" && erros.Len() == 0 {
		return "", nil // não encontrado
	}
	if err != nil {
		return "", fmt.Errorf("secret-tool %s: %w: %s", args[0], err, strings.TrimSpace(erros.String()))
	}
	return saida.String(), nil
}

// Ler implementa Cofre.
func (s *SecretService) Ler(ctx context.Context, nome string) (string, error) {
	v, err := s.executar(ctx, "", "lookup", "aplicacao", "palantir", "nome", nome)
	return strings.TrimSuffix(v, "\n"), err
}

// Definir implementa Cofre. O valor segue pela entrada padrão, fora da lista
// de processos.
func (s *SecretService) Definir(ctx context.Context, nome, valor string) error {
	if err := Validar(nome); err != nil {
		return err
	}
	_, err := s.executar(ctx, valor, "store", "--label=Palantír: "+nome, "aplicacao", "palantir", "nome", nome)
	return err
}

// Remover implementa Cofre.
func (s *SecretService) Remover(ctx context.Context, nome string) error {
	if err := Validar(nome); err != nil {
		return err
	}
	_, err := s.executar(ctx, "", "clear", "aplicacao", "palantir", "nome", nome)
	return err
}
