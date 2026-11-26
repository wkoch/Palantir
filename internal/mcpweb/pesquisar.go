package mcpweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wkoch/Palantir/internal/proxy"
)

const (
	maxResultados = 8
	maxTrecho     = 400
)

// Consulta são os argumentos de "pesquisar".
type Consulta struct {
	Consulta string `json:"consulta" jsonschema:"o que procurar na web"`
}

// Resultado é um item da pesquisa.
type Resultado struct {
	Titulo string `json:"titulo"`
	URL    string `json:"url"`
	Trecho string `json:"trecho"`
}

// Resultados é a resposta de "pesquisar".
type Resultados struct {
	Itens []Resultado `json:"itens"`
}

func resumir(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		s = string(r[:n-1]) + "…"
	}
	return s
}

// Pesquisar consulta o SearXNG configurado, sempre pelo proxy de saída.
func Pesquisar(ctx context.Context, o Opcoes, consulta string) (Resultados, error) {
	var r Resultados
	if o.Busca == "" {
		return r, errors.New("a pesquisa na web não está configurada")
	}
	consulta = strings.TrimSpace(consulta)
	if consulta == "" {
		return r, errors.New("consulta vazia")
	}
	u, err := ValidarURL(strings.TrimRight(o.Busca, "/") + "/search")
	if err != nil {
		return r, err
	}
	u.RawQuery = url.Values{"q": {consulta}, "format": {"json"}, "language": {"pt-BR"}, "safesearch": {"1"}}.Encode()
	cliente := &http.Client{Timeout: 30 * time.Second,
		Transport: &http.Transport{DialContext: proxy.Discador{Socket: o.Socket}.DialContext,
			TLSClientConfig: o.TLS, Proxy: nil, DisableKeepAlives: true},
		// O mecanismo de busca responde direto; um desvio levaria a consulta a
		// outro destino.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return r, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Palantir/1.0 (pesquisa local)")
	res, err := cliente.Do(req)
	if err != nil {
		return r, fmt.Errorf("o mecanismo de busca não respondeu: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return r, fmt.Errorf("o mecanismo de busca respondeu HTTP %d", res.StatusCode)
	}
	var corpo struct {
		Itens []struct {
			URL    string `json:"url"`
			Titulo string `json:"title"`
			Trecho string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, limiteCorpo)).Decode(&corpo); err != nil {
		return r, fmt.Errorf("resposta inesperada do mecanismo de busca: %w", err)
	}
	r.Itens = []Resultado{}
	vistos := map[string]bool{}
	for _, it := range corpo.Itens {
		alvo, err := ValidarURL(it.URL)
		if err != nil || vistos[alvo.String()] || strings.ContainsAny(alvo.String(), " \n\r\t") {
			continue
		}
		vistos[alvo.String()] = true
		r.Itens = append(r.Itens, Resultado{Titulo: resumir(it.Titulo, 200), URL: alvo.String(),
			Trecho: resumir(it.Trecho, maxTrecho)})
		if len(r.Itens) == maxResultados {
			break
		}
	}
	return r, nil
}
