package conversa_test

import (
	"strings"
	"testing"

	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/conversa"
	"github.com/wkoch/Palantir/internal/modelos"
	"github.com/wkoch/Palantir/internal/simulador"
	"github.com/wkoch/Palantir/internal/testes"
)

func TestPesquisaNaWebELeituraDoAchado(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{Busca: true})
	passo := 0
	s.Modelo.Definir(func(p simulador.PedidoRecebido) (simulador.Resposta, bool) {
		if p.Esquema() != "" {
			return simulador.Resposta{}, false
		}
		if !p.Oferece("pesquisar_web") {
			t.Error("a pesquisa não foi oferecida")
		}
		passo++
		switch passo {
		case 1:
			return chamar("pesquisar_web", `{"consulta":"prazos de recurso"}`), true
		case 2:
			// Com conteúdo externo no contexto, o achado pode ser lido.
			return chamar("buscar_url", `{"url":"`+s.Web.URL+`/pagina"}`), true
		case 3:
			// Uma consulta com palavras novas pede confirmação.
			return chamar("pesquisar_web", `{"consulta":"salário do usuário 12345"}`), true
		}
		return simulador.Resposta{}, false
	})
	x := s.Criar("conversa", "Pesquise os prazos de recurso", "")
	d := testes.Dados[conversa.Dados](t, x)
	var ferramentas []string
	for _, it := range d.Itens {
		if it.Papel == modelos.PapelFerramenta {
			ferramentas = append(ferramentas, it.Conteudo)
		}
	}
	if len(ferramentas) != 3 || !strings.Contains(ferramentas[0], "prazos de recurso") ||
		!strings.Contains(ferramentas[1], "dez dias corridos") || !strings.Contains(ferramentas[2], "aguarda a confirmação") {
		t.Fatal(ferramentas)
	}
	if len(d.Itens[2].Achados) != 1 || len(d.Itens[2].Citacoes) != 1 {
		t.Errorf("%+v", d.Itens[2])
	}
	pendentes, err := s.D.Confirmacoes.Pendentes(s.Contexto)
	if err != nil || len(pendentes) != 1 || !strings.Contains(string(pendentes[0].Argumentos), "12345") {
		t.Fatal(pendentes, err)
	}
	// Aprovada na tela, a pesquisa acontece.
	if err := s.D.DecidirConfirmacao(s.Contexto, pendentes[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if x = s.Esperar(x.ID); x.Estado != atividades.Pronta {
		t.Fatal(x.Estado, x.Aviso)
	}
	achou := false
	for _, it := range testes.Dados[conversa.Dados](t, x).Itens {
		achou = achou || strings.Contains(it.Conteudo, "salário do usuário 12345") && len(it.Achados) == 1
	}
	if !achou {
		t.Error("a pesquisa aprovada não trouxe resultados")
	}
}

func TestSemMecanismoDeBuscaAFerramentaNaoAparece(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	s.Modelo.Definir(func(p simulador.PedidoRecebido) (simulador.Resposta, bool) {
		if p.Oferece("pesquisar_web") {
			t.Error("a pesquisa foi oferecida sem mecanismo")
		}
		if p.Ultima().Papel == "user" {
			return chamar("pesquisar_web", `{"consulta":"x"}`), true
		}
		return simulador.Resposta{}, false
	})
	x := s.Criar("conversa", "Pesquise x", "")
	var falhou bool
	for _, it := range testes.Dados[conversa.Dados](t, x).Itens {
		falhou = falhou || strings.Contains(it.Conteudo, "não está configurada")
	}
	if !falhou {
		t.Error(testes.Dados[conversa.Dados](t, x).Itens)
	}
	if err := (config.Config{Busca: config.Busca{URL: "ftp://x"}, Servidor: config.Servidor{Endereco: "127.0.0.1:1"}}).Validar(); err == nil {
		t.Error("aceitou um mecanismo inválido")
	}
}
