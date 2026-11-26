// Package mcpweb é o servidor MCP de leitura de páginas. Só alcança a rede
// pelo proxy de saída.
package mcpweb

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wkoch/Palantir/internal/extrator"
	"github.com/wkoch/Palantir/internal/proxy"
	"github.com/wkoch/Palantir/internal/textohtml"
)

const (
	limiteCorpo = 20 << 20
	limiteTexto = 40000
)

// Entrada são os argumentos de "buscar_url".
type Entrada struct {
	URL string `json:"url" jsonschema:"endereço http ou https da página"`
}

// Pagina é o resultado de "buscar_url".
type Pagina struct {
	URL      string `json:"url"`
	Status   int    `json:"status"`
	Titulo   string `json:"titulo"`
	Texto    string `json:"texto"`
	Truncado bool   `json:"truncado"`
}

// Opcoes configura o servidor.
type Opcoes struct {
	Socket   string
	Extrator extrator.Extrator
	// Busca é a base do SearXNG; vazia desliga "pesquisar".
	Busca string
	// TLS substitui a configuração de certificados (testes).
	TLS *tls.Config
}

// ValidarURL aceita somente endereços http e https sem credenciais.
func ValidarURL(bruta string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(bruta))
	if err != nil {
		return nil, fmt.Errorf("URL inválida: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("URL inválida: use http ou https")
	}
	if u.Hostname() == "" {
		return nil, errors.New("URL inválida: sem host")
	}
	if u.User != nil {
		return nil, errors.New("URL inválida: credenciais no endereço não são aceitas")
	}
	return u, nil
}

// Buscar baixa e converte uma página.
func Buscar(ctx context.Context, o Opcoes, bruta string) (Pagina, error) {
	u, err := ValidarURL(bruta)
	if err != nil {
		return Pagina{}, err
	}
	transporte := &http.Transport{
		DialContext:     proxy.Discador{Socket: o.Socket}.DialContext,
		TLSClientConfig: o.TLS,
		// Nenhuma variável de ambiente desvia do proxy de saída.
		Proxy: nil, ForceAttemptHTTP2: true, DisableKeepAlives: true,
	}
	cliente := &http.Client{Transport: transporte, Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("redirecionamentos demais")
			}
			_, err := ValidarURL(req.URL.String())
			return err
		}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Pagina{}, err
	}
	req.Header.Set("User-Agent", "Palantir/1.0 (leitor de paginas)")
	req.Header.Set("Accept", "text/html,text/plain,application/pdf,application/json;q=0.9,*/*;q=0.1")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.5")
	res, err := cliente.Do(req)
	if err != nil {
		return Pagina{}, err
	}
	defer res.Body.Close()
	corpo, err := io.ReadAll(io.LimitReader(res.Body, limiteCorpo))
	if err != nil {
		return Pagina{}, err
	}
	p := Pagina{URL: res.Request.URL.String(), Status: res.StatusCode}
	if res.StatusCode >= 400 {
		return p, fmt.Errorf("o servidor respondeu HTTP %d", res.StatusCode)
	}
	tipo, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	switch {
	case tipo == "text/html" || tipo == "application/xhtml+xml" || tipo == "":
		leitor, err := emUTF8(corpo, res.Header.Get("Content-Type"))
		if err != nil {
			return p, err
		}
		if p.Titulo, p.Texto, err = textohtml.Extrair(leitor); err != nil {
			return p, err
		}
	case tipo == "application/pdf":
		if p.Texto, err = textoDePDF(ctx, o.Extrator, corpo); err != nil {
			return p, err
		}
	case strings.HasPrefix(tipo, "text/") || tipo == "application/json" || strings.HasSuffix(tipo, "xml"):
		leitor, err := emUTF8(corpo, res.Header.Get("Content-Type"))
		if err != nil {
			return p, err
		}
		b, err := io.ReadAll(leitor)
		if err != nil {
			return p, err
		}
		p.Texto = string(b)
	default:
		return p, fmt.Errorf("tipo de conteúdo não suportado: %s", tipo)
	}
	if utf8.RuneCountInString(p.Texto) > limiteTexto {
		p.Texto, p.Truncado = string([]rune(p.Texto)[:limiteTexto]), true
	}
	return p, nil
}

func textoDePDF(ctx context.Context, e extrator.Extrator, corpo []byte) (string, error) {
	f, err := os.CreateTemp("", "pagina-*.pdf")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	_, errEscrever := f.Write(corpo)
	if err := errors.Join(errEscrever, f.Close()); err != nil {
		return "", err
	}
	r, err := e.Extrair(ctx, f.Name(), "pagina.pdf")
	if err != nil {
		return "", err
	}
	var s strings.Builder
	for _, p := range r.Paginas {
		fmt.Fprintf(&s, "[página %d]\n%s\n\n", p.Numero, p.Texto)
	}
	return strings.TrimSpace(s.String()), nil
}

// Novo cria o servidor MCP.
func Novo(o Opcoes) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "palantir-web", Version: "1"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "buscar_url", Description: "Lê uma página da web e devolve o texto."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in Entrada) (*mcp.CallToolResult, Pagina, error) {
			p, err := Buscar(ctx, o, in.URL)
			return nil, p, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "pesquisar", Description: "Pesquisa na web e devolve títulos, endereços e trechos."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in Consulta) (*mcp.CallToolResult, Resultados, error) {
			r, err := Pesquisar(ctx, o, in.Consulta)
			return nil, r, err
		})
	return s
}
