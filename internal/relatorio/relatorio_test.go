package relatorio_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/relatorio"
	"github.com/wkoch/Palantir/internal/simulador"
	"github.com/wkoch/Palantir/internal/testes"
)

func preparar(t *testing.T) *testes.Sistema {
	t.Helper()
	s := testes.Novo(t, testes.Opcoes{})
	s.Importar("contrato.txt", "O aluguel é de R$ 900,00 e vence no dia 5. A multa por atraso é de dez por cento.")
	return s
}

func dados(t *testing.T, x atividades.Atividade) relatorio.Dados {
	return testes.Dados[relatorio.Dados](t, x)
}

func TestFluxoCompleto(t *testing.T) {
	s := preparar(t)
	x := s.Criar("relatorio", "Aluguel [perguntar]", `{"profundo":true}`)
	d := dados(t, x)
	if x.Estado != atividades.Esperando || len(d.Perguntas) != 1 || d.Planejado {
		t.Fatal(x.Estado, d)
	}
	// Retomar sem responder não chama o modelo.
	antes := len(s.Modelo.Pedidos())
	s.D.Gerente.Mudar(s.Contexto, x.ID, atividades.Rodando)
	if x = s.Esperar(x.ID); x.Estado != atividades.Esperando || len(s.Modelo.Pedidos()) != antes {
		t.Fatal(x.Estado)
	}
	for _, ruim := range []string{`{"respostas":[]}`, `{"respostas":[" "]}`, `{"x":1}`} {
		if _, err := s.D.Gerente.Agir(s.Contexto, x.ID, "responder", []byte(ruim)); !errors.Is(err, atividades.ErrPedidoInvalido) {
			t.Error(ruim, err)
		}
	}
	if _, err := s.D.Gerente.Agir(s.Contexto, x.ID, "aprovar", nil); !errors.Is(err, atividades.ErrPedidoInvalido) {
		t.Error("aprovar antes de escrever", err)
	}
	_, x = s.Agir(x.ID, "responder", `{"respostas":["2026"]}`)
	d = dados(t, x)
	if x.Estado != atividades.Pronta || len(d.Secoes) != 2 || len(d.Fontes) == 0 {
		t.Fatal(x.Estado, x.Aviso, d)
	}
	for _, sec := range d.Secoes {
		if len(sec.Paragrafos) == 0 || sec.Estado != relatorio.SecaoRascunho {
			t.Error(sec)
		}
		for _, p := range sec.Paragrafos {
			if len(p.Fontes) == 0 {
				t.Error("afirmação sem fonte")
			}
		}
	}
	// Corrigir uma seção não altera as outras.
	_, x = s.Agir(x.ID, "comentar", `{"secao":2,"texto":"detalhe a multa"}`)
	d2 := dados(t, x)
	if !slices.EqualFunc(d.Secoes[0].Paragrafos, d2.Secoes[0].Paragrafos, func(a, b relatorio.Paragrafo) bool { return a.Texto == b.Texto }) {
		t.Error("a outra seção mudou")
	}
	if !strings.Contains(d2.Secoes[1].Paragrafos[0].Texto, "detalhe a multa") || !d2.Secoes[1].Comentarios[0].Atendido {
		t.Errorf("%+v", d2.Secoes[1])
	}
	ultimo := s.Modelo.Pedidos()[len(s.Modelo.Pedidos())-1]
	if !strings.Contains(ultimo.Bruto, "Versão anterior") || ultimo.Modelo != "teste/forte" {
		t.Error(ultimo.Modelo)
	}
	for _, ruim := range [][2]string{{"comentar", `{"secao":9,"texto":"x"}`}, {"comentar", `{"secao":1,"texto":" "}`},
		{"comentar", `[]`}, {"aprovar_secao", `{"secao":9}`}, {"aprovar_secao", `[]`}, {"voar", `{}`}} {
		if _, err := s.D.Gerente.Agir(s.Contexto, x.ID, ruim[0], []byte(ruim[1])); !errors.Is(err, atividades.ErrPedidoInvalido) {
			t.Error(ruim, err)
		}
	}
	_, x = s.Agir(x.ID, "aprovar_secao", `{"secao":1,"aprovada":true}`)
	if dados(t, x).Secoes[0].Estado != relatorio.SecaoAprovada {
		t.Error("não aprovou")
	}
	r, x := s.Agir(x.ID, "aprovar", `{}`)
	d = dados(t, x)
	doc := r.(map[string]any)["documento"].(int64)
	if len(d.Versoes) != 1 || d.Versoes[0].Documento != doc || x.Aviso == "" {
		t.Fatal(d.Versoes, x.Aviso)
	}
	guardado, err := s.D.Acervo.Documento(s.Contexto, doc)
	if err != nil || guardado.Origem != "relatorio" || !strings.HasSuffix(guardado.Nome, "versão 1.md") {
		t.Error(guardado, err)
	}
}

func TestFontesInvalidasSaoRefeitasEDescartadas(t *testing.T) {
	s := preparar(t)
	tentativas := 0
	s.Modelo.Definir(func(p simulador.PedidoRecebido) (simulador.Resposta, bool) {
		if p.Esquema() != "secao_relatorio" {
			return simulador.Resposta{}, false
		}
		tentativas++
		return simulador.Resposta{JSON: map[string]any{"paragrafos": []any{
			map[string]any{"texto": "Com fonte.", "fontes": []string{"[F1]", "F1", "F42"}},
			map[string]any{"texto": "Sem fonte.", "fontes": []string{}},
			map[string]any{"texto": "  ", "fontes": []string{}},
		}}}, true
	})
	x := s.Criar("relatorio", "Aluguel", "")
	d := dados(t, x)
	if tentativas != 4 || len(d.Secoes[0].Paragrafos) != 1 || d.Secoes[0].Descartadas != 1 ||
		!slices.Equal(d.Secoes[0].Paragrafos[0].Fontes, []string{"F1"}) {
		t.Errorf("tentativas=%d %+v", tentativas, d.Secoes[0])
	}
}

func TestConfirmacaoDeFonteDaWeb(t *testing.T) {
	s := preparar(t)
	s.Modelo.Definir(func(p simulador.PedidoRecebido) (simulador.Resposta, bool) {
		if p.Esquema() != "plano_relatorio" {
			return simulador.Resposta{}, false
		}
		return simulador.Resposta{JSON: map[string]any{"perguntas": []string{}, "secoes": []any{
			map[string]any{"titulo": "Única", "orientacao": "", "buscas": []string{}, "urls": []string{
				"https://sugerido.test/lei", "http://10.0.0.1/", "http://liberado.test:1/"}},
			map[string]any{"titulo": " ", "orientacao": "", "buscas": []string{}, "urls": []string{}},
		}}}, true
	})
	x := s.Criar("relatorio", "Aluguel", "")
	d := dados(t, x)
	if x.Estado != atividades.Esperando || len(d.Secoes) != 1 || !strings.Contains(d.Secoes[0].Aviso, "aguardando confirmação") ||
		!strings.Contains(d.Secoes[0].Aviso, "bloqueado: http://10.0.0.1/") {
		t.Fatal(x.Estado, d.Secoes)
	}
	if len(s.Resolver.Consultas()) != 0 {
		t.Error("nada sai antes do clique")
	}
	p, _ := s.D.Confirmacoes.Pendentes(s.Contexto)
	if len(p) != 1 {
		t.Fatal(p)
	}
	if err := s.D.DecidirConfirmacao(s.Contexto, p[0].ID, true); err != nil {
		t.Fatal(err)
	}
	x = s.Esperar(x.ID)
	d = dados(t, x)
	if x.Estado != atividades.Pronta || !slices.Contains(d.Liberadas, "https://sugerido.test/lei") ||
		!slices.Contains(s.Resolver.Consultas(), "sugerido.test") {
		t.Error(x.Estado, d.Liberadas, s.Resolver.Consultas())
	}
}

func TestRecusaESemFontes(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	s.Modelo.Definir(func(p simulador.PedidoRecebido) (simulador.Resposta, bool) {
		if p.Esquema() != "plano_relatorio" {
			return simulador.Resposta{}, false
		}
		return simulador.Resposta{JSON: map[string]any{"perguntas": []string{}, "secoes": []any{
			map[string]any{"titulo": "Única", "orientacao": "", "buscas": []string{"nada"}, "urls": []string{"https://sugerido.test/x"}}}}}, true
	})
	x := s.Criar("relatorio", "Tema sem material", "")
	d := dados(t, x)
	if len(d.Secoes[0].Paragrafos) != 0 || !strings.Contains(d.Secoes[0].Aviso, "nenhuma fonte") {
		t.Fatal(d.Secoes[0])
	}
	md := relatorio.Markdown("T", d)
	if !strings.Contains(md, "nenhuma fonte sustentou") {
		t.Error(md)
	}
	p, _ := s.D.Confirmacoes.Pendentes(s.Contexto)
	if err := s.D.DecidirConfirmacao(s.Contexto, p[0].ID, false); err != nil {
		t.Fatal(err)
	}
	if x = s.Esperar(x.ID); x.Estado != atividades.Pronta || len(s.Resolver.Consultas()) != 0 {
		t.Error(x.Estado)
	}
}

func TestEntradasEPlanoRuim(t *testing.T) {
	s := preparar(t)
	for _, c := range [][2]string{{"", ""}, {"x", `{"y":true}`}} {
		var o []byte
		if c[1] != "" {
			o = []byte(c[1])
		}
		if _, err := s.D.Gerente.Criar(s.Contexto, "relatorio", "t", c[0], o); !errors.Is(err, atividades.ErrPedidoInvalido) {
			t.Error(c, err)
		}
	}
	s.Modelo.Definir(func(p simulador.PedidoRecebido) (simulador.Resposta, bool) {
		return simulador.Resposta{JSON: map[string]any{"perguntas": []string{" "}, "secoes": []any{}}}, p.Esquema() == "plano_relatorio"
	})
	x := s.Criar("relatorio", "Aluguel", "")
	if x.Estado != atividades.Esperando || !strings.Contains(x.Aviso, "sem seções") {
		t.Error(x.Estado, x.Aviso)
	}
}

func TestMarkdown(t *testing.T) {
	d := relatorio.Dados{
		Secoes: []relatorio.Secao{{Titulo: "A", Paragrafos: []relatorio.Paragrafo{{Texto: "um", Fontes: []string{"F2", "F1"}}, {Texto: "dois", Fontes: []string{"F2"}}}}},
		Fontes: []relatorio.Fonte{{ID: "F1", Tipo: "documento", Titulo: "doc", Pagina: 3}, {ID: "F2", Tipo: "url", Titulo: "pág", URL: "http://a"}},
	}
	quer := "# T\n\n## A\n\num [1] [2]\n\ndois [1]\n\n## Fontes\n\n1. pág: <http://a>\n2. doc, página 3 (acervo)\n"
	if got := relatorio.Markdown("T", d); got != quer {
		t.Errorf("%q", got)
	}
}
