package simulador

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/segredos"
)

var configDemo = template.Must(template.New("config").Parse(`# Demonstração: tudo local e sem custo.
[servidor]
endereco = {{printf "%q" .Endereco}}

[modelos]
url = "http://{{.Modelo}}/api/v1"

[ferramentas]
isolamento = {{printf "%q" .Isolamento}}
binario = {{printf "%q" .Binario}}
permitir_enderecos_locais = {{.Locais}}

[ferramentas.web]
hosts = ["127.0.0.1"]

[imap]
servidor = "127.0.0.1"
porta = {{.PortaIMAP}}
usuario = {{printf "%q" .Usuario}}
seguranca = "nenhum"

[acervo]
pasta_entrada = {{printf "%q" .Entrada}}
intervalo_segundos = 2

[segredos]
armazenamento = "arquivo"
`))

// Executar prepara uma instalação de demonstração e mantém os serviços
// simulados no ar até o contexto acabar. pronto é chamado quando tudo atende.
func Executar(ctx context.Context, args []string, saida io.Writer, pronto func()) error {
	f := flag.NewFlagSet("palantir-simulador", flag.ContinueOnError)
	f.SetOutput(saida)
	dir := f.String("dir", "", "pasta da demonstração (obrigatória)")
	endereco := f.String("endereco", "127.0.0.1:7800", "endereço que o palantird da demonstração usará")
	modeloEm := f.String("modelo", "127.0.0.1:7801", "endereço do modelo simulado")
	imapEm := f.String("imap", "127.0.0.1:7802", "endereço do IMAP simulado")
	webEm := f.String("web", "127.0.0.1:7803", "endereço da página de exemplo")
	binario := f.String("ferramentas", "palantir-tool", "binário palantir-tool")
	isolamento := f.String("isolamento", "nenhum", "podman ou nenhum")
	atraso := f.Duration("atraso", 300*time.Millisecond, "tempo de resposta do modelo")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return errors.New("informe --dir com a pasta da demonstração")
	}
	c, err := config.CaminhosPadrao(*dir)
	if err != nil {
		return err
	}
	if err := c.Criar(); err != nil {
		return err
	}
	raiz := filepath.Dir(c.Config)

	correio, err := NovoCorreio(*imapEm, Emails, nil)
	if err != nil {
		return fmt.Errorf("IMAP simulado: %w", err)
	}
	defer correio.Fechar()
	var servidores []*http.Server
	defer func() {
		for _, s := range servidores {
			s.Close()
		}
	}()
	servir := func(endereco string, h http.Handler) (string, error) {
		l, err := net.Listen("tcp", endereco)
		if err != nil {
			return "", err
		}
		s := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
		servidores = append(servidores, s)
		go func() { _ = s.Serve(l) }()
		return l.Addr().String(), nil
	}
	modelo, err := servir(*modeloEm, &Modelo{Atraso: *atraso, Custo: 0.0004})
	if err != nil {
		return fmt.Errorf("modelo simulado: %w", err)
	}
	web, err := servir(*webEm, Web(correio))
	if err != nil {
		return fmt.Errorf("página simulada: %w", err)
	}

	bin := *binario
	if strings.ContainsRune(bin, os.PathSeparator) {
		if bin, err = filepath.Abs(bin); err != nil {
			return err
		}
	}
	var buf bytes.Buffer
	if err := configDemo.Execute(&buf, map[string]any{
		"Endereco": *endereco, "Modelo": modelo, "PortaIMAP": strconv.Itoa(correio.Porta()), "Usuario": Usuario,
		"Isolamento": *isolamento, "Binario": bin, "Locais": true, "Entrada": filepath.Join(raiz, "Entrada"),
	}); err != nil {
		return err
	}
	// Uma pasta com dados reais nunca recebe a configuração de demonstração.
	marca := filepath.Join(raiz, ".demonstracao")
	if _, err := os.Stat(marca); errors.Is(err, fs.ErrNotExist) {
		if _, err := os.Stat(c.Arquivo()); err == nil {
			return fmt.Errorf("%s já tem uma configuração que não é de demonstração", raiz)
		}
		if err := os.WriteFile(marca, nil, 0o600); err != nil {
			return err
		}
	}
	if err := os.WriteFile(c.Arquivo(), buf.Bytes(), 0o600); err != nil {
		return err
	}
	if _, err := config.Carregar(c); err != nil {
		return err
	}
	cofre := segredos.Novo("arquivo", c.Segredos())
	if err := cofre.Definir(ctx, segredos.OpenRouter, "chave-simulada"); err != nil {
		return err
	}
	if err := cofre.Definir(ctx, segredos.SenhaIMAP, Senha); err != nil {
		return err
	}
	exemplos := filepath.Join(raiz, "exemplos")
	if err := GravarExemplos(exemplos); err != nil {
		return err
	}
	fmt.Fprintf(saida, `Simulação no ar. Em outro terminal:

  palantird --dir %s

Depois abra http://%s

Documentos de exemplo: %s
Página de exemplo:     http://%s/pagina
E-mail novo:           curl -X POST http://%s/entregar
`, raiz, *endereco, exemplos, web, web)
	if pronto != nil {
		pronto()
	}
	<-ctx.Done()
	return nil
}
