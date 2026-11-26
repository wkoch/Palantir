package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/wkoch/Palantir/internal/config"
)

// esperaDoDaemon cobre o primeiro início, logo depois do login.
const esperaDoDaemon = 60 * time.Second

// abrir leva o navegador ao Palantír já autorizado. A chave não pode passar
// pela linha de comando, que os outros usuários da máquina enxergam: ela vai
// num arquivo privado que redireciona o navegador.
func abrir(ctx context.Context, c config.Caminhos, args []string, a Ambiente) error {
	f := flag.NewFlagSet("abrir", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	navegador := f.String("navegador", os.Getenv("PALANTIR_NAVEGADOR"), "")
	imprimir := f.Bool("imprimir", false, "")
	espera := f.Duration("espera", esperaDoDaemon, "")
	if err := f.Parse(args); err != nil {
		return erroDeUso{err.Error()}
	}
	if err := semArgumentos(f.Args()); err != nil {
		return err
	}
	endereco, err := aguardarDaemon(ctx, c, *espera)
	if err != nil {
		return err
	}
	chave, err := c.LerChaveDeAcesso()
	if err != nil {
		return err
	}
	destino := "http://" + endereco + "/#entrar=" + chave
	arquivo := filepath.Join(c.Execucao, "entrar.html")
	pagina := `<!doctype html><meta charset="utf-8"><title>Palantír</title>` +
		`<meta http-equiv="refresh" content="0;url=` + html.EscapeString(destino) + `">`
	if err := os.WriteFile(arquivo, []byte(pagina), 0o600); err != nil {
		return err
	}
	if *imprimir {
		fmt.Fprintln(a.Saida, arquivo)
		return nil
	}
	if *navegador == "" {
		*navegador = "xdg-open"
		if p, err := exec.LookPath("chromium"); err == nil {
			*navegador = p
		}
	}
	cmd := exec.Command(*navegador, "file://"+arquivo)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("abrir o navegador: %w", err)
	}
	// O navegador continua depois que o comando termina.
	return cmd.Process.Release()
}

// enderecoAtual é o endereço anotado pelo daemon em execução.
func enderecoAtual(c config.Caminhos) (string, error) {
	b, err := os.ReadFile(c.Endereco())
	return strings.TrimSpace(string(b)), err
}

func aguardarDaemon(ctx context.Context, c config.Caminhos, espera time.Duration) (string, error) {
	prazo := time.Now().Add(espera)
	for {
		if endereco, err := enderecoAtual(c); err == nil {
			if _, ok := saude(ctx, endereco); ok {
				return endereco, nil
			}
		}
		if time.Now().After(prazo) {
			return "", errors.New("o daemon não está respondendo; veja: systemctl --user status palantird")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// saude pergunta a um daemon a sua versão.
func saude(ctx context.Context, endereco string) (string, bool) {
	ctx, cancelar := context.WithTimeout(ctx, 2*time.Second)
	defer cancelar()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+endereco+"/api/saude", nil)
	if err != nil {
		return "", false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false
	}
	defer res.Body.Close()
	var v struct {
		OK     bool   `json:"ok"`
		Versao string `json:"versao"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&v) != nil {
		return "", false
	}
	return v.Versao, v.OK
}
