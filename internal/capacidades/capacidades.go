// Package capacidades implementa as ferramentas que os agentes usam, ligando o
// acervo e as ferramentas MCP ao loop do agente.
package capacidades

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/wkoch/Palantir/internal/acervo"
	"github.com/wkoch/Palantir/internal/agente"
	"github.com/wkoch/Palantir/internal/esquema"
	"github.com/wkoch/Palantir/internal/ferramentas"
	"github.com/wkoch/Palantir/internal/mcpweb"
	"github.com/wkoch/Palantir/internal/politica"
	"github.com/wkoch/Palantir/internal/proveniencia"
	"github.com/wkoch/Palantir/internal/proxy"
	"github.com/wkoch/Palantir/internal/telas"
)

// Web lê páginas pela ferramenta isolada, liberando no proxy somente o host da
// chamada em curso.
type Web struct {
	Executor ferramentas.Executor
	Proxy    *proxy.Proxy
	Socket   string
}

// Marca devolve a etiqueta de citação estável de um endereço.
func Marca(endereco string) string {
	soma := sha256.Sum256([]byte(endereco))
	return "W" + hex.EncodeToString(soma[:3])
}

// Buscar lê uma página. A política já deve ter autorizado o endereço.
func (w *Web) Buscar(ctx context.Context, endereco string) (mcpweb.Pagina, error) {
	var p mcpweb.Pagina
	u, err := mcpweb.ValidarURL(endereco)
	if err != nil {
		return p, err
	}
	portas := []int{80, 443}
	if u.Port() != "" {
		n, err := strconv.Atoi(u.Port())
		if err != nil {
			return p, err
		}
		portas = append(portas, n)
	}
	host := proxy.Normalizar(u.Hostname())
	// A variante com ou sem "www." é o mesmo site, e o redirecionamento entre
	// elas é comum.
	outro, tinha := strings.CutPrefix(host, "www.")
	if !tinha {
		outro = "www." + host
	}
	for _, h := range []string{host, outro} {
		revogar := w.Proxy.Conceder(proxy.Regra{Host: h, Portas: portas})
		defer revogar()
	}
	err = ferramentas.Chamar(ctx, w.Executor, ferramentas.Execucao{Servidor: ferramentas.Web, ProxySocket: w.Socket},
		"buscar_url", mcpweb.Entrada{URL: u.String()}, &p)
	return p, err
}

// CitacaoDeTrecho é a citação de um trecho do acervo.
func CitacaoDeTrecho(t acervo.Trecho) agente.Citacao {
	return agente.Citacao{Marca: fmt.Sprintf("T%d", t.ID), Titulo: t.Nome, Documento: t.Documento, Pagina: t.Pagina}
}

func formatarTrechos(trechos []acervo.Trecho) (string, []agente.Citacao) {
	var b strings.Builder
	citacoes := make([]agente.Citacao, len(trechos))
	for i, t := range trechos {
		fmt.Fprintf(&b, "[T%d] %s, página %d (documento %d)\n%s\n\n", t.ID, t.Nome, t.Pagina, t.Documento, t.Texto)
		citacoes[i] = CitacaoDeTrecho(t)
	}
	return strings.TrimSpace(b.String()), citacoes
}

func decodificar[T any](args json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(args, &v); err != nil {
		return v, fmt.Errorf("argumentos inválidos: %w", err)
	}
	return v, nil
}

const maxLeitura = 14000

// Ferramentas monta o conjunto oferecido aos agentes.
func Ferramentas(a *acervo.Acervo, w *Web) map[string]agente.Ferramenta {
	lista := []agente.Ferramenta{
		{
			Nome: politica.BuscarAcervo,
			Descricao: "Procura nos documentos do usuário. Cite cada informação usada com a marca " +
				"entre colchetes, como [T12].",
			Esquema: esquema.JSON(esquema.Objeto(map[string]any{"consulta": esquema.Texto})),
			Executar: func(ctx context.Context, atividade int64, args json.RawMessage) (agente.Saida, error) {
				in, err := decodificar[struct {
					Consulta string `json:"consulta"`
				}](args)
				if err != nil {
					return agente.Saida{}, err
				}
				achados, err := a.Buscar(ctx, atividade, in.Consulta, 8)
				if err != nil {
					return agente.Saida{}, err
				}
				if len(achados) == 0 {
					return agente.Saida{Origem: proveniencia.Sistema, Texto: "Nada encontrado no acervo para essa consulta."}, nil
				}
				trechos := make([]acervo.Trecho, len(achados))
				for i, x := range achados {
					trechos[i] = x.Trecho
				}
				texto, citacoes := formatarTrechos(trechos)
				return agente.Saida{Origem: proveniencia.Documento, Fonte: "acervo", Texto: texto, Citacoes: citacoes}, nil
			},
		},
		{
			Nome:      politica.LerDocumento,
			Descricao: "Lê o texto de um documento do acervo a partir de uma página.",
			Esquema: esquema.JSON(esquema.Objeto(map[string]any{
				"documento": esquema.Inteiro, "pagina": esquema.Inteiro})),
			Executar: func(ctx context.Context, _ int64, args json.RawMessage) (agente.Saida, error) {
				in, err := decodificar[struct {
					Documento int64 `json:"documento"`
					Pagina    int   `json:"pagina"`
				}](args)
				if err != nil {
					return agente.Saida{}, err
				}
				todos, err := a.TrechosDoDocumento(ctx, in.Documento)
				if err != nil {
					return agente.Saida{}, err
				}
				var escolhidos []acervo.Trecho
				total, proxima := 0, 0
				for _, t := range todos {
					if t.Pagina < in.Pagina {
						continue
					}
					if total += len(t.Texto); total > maxLeitura && len(escolhidos) > 0 {
						proxima = t.Pagina
						break
					}
					escolhidos = append(escolhidos, t)
				}
				if len(escolhidos) == 0 {
					return agente.Saida{Origem: proveniencia.Sistema, Texto: "Não há texto a partir dessa página."}, nil
				}
				texto, citacoes := formatarTrechos(escolhidos)
				if proxima > 0 {
					texto += fmt.Sprintf("\n\n(o documento continua; leia a partir da página %d)", proxima)
				}
				return agente.Saida{Origem: proveniencia.Documento, Fonte: escolhidos[0].Nome,
					Texto: texto, Citacoes: citacoes}, nil
			},
		},
		{
			Nome: politica.BuscarURL,
			Descricao: "Lê uma página da web. Só hosts liberados ou citados pelo usuário; outros pedem " +
				"confirmação. Cite com a marca informada no resultado.",
			Esquema: esquema.JSON(esquema.Objeto(map[string]any{"url": esquema.Texto})),
			Executar: func(ctx context.Context, _ int64, args json.RawMessage) (agente.Saida, error) {
				in, err := decodificar[struct {
					URL string `json:"url"`
				}](args)
				if err != nil {
					return agente.Saida{}, err
				}
				if w == nil {
					return agente.Saida{}, errors.New("a pesquisa na web não está disponível")
				}
				p, err := w.Buscar(ctx, in.URL)
				if err != nil {
					return agente.Saida{}, err
				}
				marca := Marca(p.URL)
				titulo := p.Titulo
				if titulo == "" {
					if u, err := url.Parse(p.URL); err == nil {
						titulo = u.Hostname()
					}
				}
				texto := fmt.Sprintf("[%s] %s\n%s\n\n%s", marca, titulo, p.URL, p.Texto)
				if p.Truncado {
					texto += "\n(texto cortado)"
				}
				return agente.Saida{Origem: proveniencia.Ferramenta, Fonte: p.URL, Texto: texto,
					Citacoes: []agente.Citacao{{Marca: marca, Titulo: titulo, URL: p.URL}}}, nil
			},
		},
		ferramentaTela(),
	}
	m := map[string]agente.Ferramenta{}
	for _, f := range lista {
		m[f.Nome] = f
	}
	return m
}

func ferramentaTela() agente.Ferramenta {
	esquemas := map[string]json.RawMessage{}
	for _, n := range telas.DoModelo {
		if e, err := telas.Esquema(n); err == nil {
			esquemas[n] = e
		}
	}
	descricoes, _ := json.Marshal(esquemas)
	return agente.Ferramenta{
		Nome: politica.MostrarTela,
		Descricao: "Mostra dados numa tela pronta. Escolha a tela e envie em dados_json o JSON que " +
			"segue o esquema dela. Esquemas: " + string(descricoes),
		Esquema: esquema.JSON(esquema.Objeto(map[string]any{
			"tela": esquema.Enum(telas.DoModelo...), "dados_json": esquema.Texto})),
		Executar: func(_ context.Context, _ int64, args json.RawMessage) (agente.Saida, error) {
			in, err := decodificar[struct {
				Tela  string `json:"tela"`
				Dados string `json:"dados_json"`
			}](args)
			if err != nil {
				return agente.Saida{}, err
			}
			var permitida bool
			for _, n := range telas.DoModelo {
				permitida = permitida || n == in.Tela
			}
			if !permitida {
				return agente.Saida{}, fmt.Errorf("%w: %q", telas.ErrTelaDesconhecida, in.Tela)
			}
			bruto := json.RawMessage(in.Dados)
			if !json.Valid(bruto) {
				return agente.Saida{}, errors.New("dados_json não é um JSON válido")
			}
			if err := telas.Validar(in.Tela, bruto); err != nil {
				return agente.Saida{}, err
			}
			return agente.Saida{Origem: proveniencia.Sistema, Texto: "Tela exibida ao usuário.",
				Quadro: &agente.Quadro{Tela: in.Tela, Dados: bruto}}, nil
		},
	}
}
