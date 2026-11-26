package estudo_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/estudo"
	"github.com/wkoch/Palantir/internal/segredos"
	"github.com/wkoch/Palantir/internal/simulador"
	"github.com/wkoch/Palantir/internal/testes"
)

const edital = "Direito Administrativo: Atos administrativos; Licitações\nDireito Constitucional: Direitos fundamentais"

func visao(t *testing.T, s *testes.Sistema, x atividades.Atividade) estudo.Visao {
	t.Helper()
	_, v, err := s.D.Gerente.Tela(s.Contexto, x)
	if err != nil {
		t.Fatal(err)
	}
	return v.(estudo.Visao)
}

func TestEstudoCompletoFuncionaSemRede(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	doc := s.Importar("edital.txt", edital)
	s.Importar("apostila.txt", "Atos administrativos produzem efeitos jurídicos. Licitações escolhem a proposta mais vantajosa. Direitos fundamentais protegem a pessoa.")
	x := s.Criar("estudo", "", `{"edital_documento":`+num(doc)+`}`)
	if x.Estado != atividades.Pronta || x.Titulo != "Estudo: edital.txt" {
		t.Fatal(x.Estado, x.Aviso, x.Titulo)
	}
	d := testes.Dados[estudo.Dados](t, x)
	if len(d.Assuntos) != 5 || d.Assuntos[1].Pai != 1 {
		t.Fatalf("%+v", d.Assuntos)
	}
	for _, a := range d.Assuntos {
		for _, id := range a.Trechos {
			tr, _ := s.D.Acervo.Trechos(s.Contexto, []int64{id})
			if tr[0].Documento == doc {
				t.Error("o edital não é material de estudo")
			}
		}
	}
	// Gerar um assunto principal gera os subtópicos.
	_, x = s.Agir(x.ID, "gerar", `{"assunto":1}`)
	v := visao(t, s, x)
	if v.Painel.Cartoes != 2 || v.Painel.Questoes != 2 || len(v.Cartoes) != 2 || v.Assuntos[0].Cartoes != 2 ||
		v.Assuntos[1].Cartoes != 1 || v.Cartoes[0].Fonte.Documento == 0 || !v.Cartoes[0].Novo {
		t.Fatalf("%+v", v)
	}
	// Gerar de novo não duplica.
	_, x = s.Agir(x.ID, "gerar", `{"assunto":2}`)
	if v := visao(t, s, x); v.Painel.Cartoes != 2 {
		t.Error(v.Painel.Cartoes)
	}

	// Daqui em diante, sem chave: nenhuma chamada de modelo.
	if err := s.D.DefinirSegredo(s.Contexto, segredos.OpenRouter, ""); err != nil {
		t.Fatal(err)
	}
	antes := len(s.Modelo.Pedidos())
	_, x = s.Agir(x.ID, "religar", `{}`)
	c1, c2 := v.Cartoes[0], v.Cartoes[1]
	r, x := s.Agir(x.ID, "revisar", `{"cartao":`+num(c1.ID)+`,"nota":1}`)
	if p := r.(map[string]any)["proxima_em"].(time.Time); time.Until(p) > 11*time.Minute {
		t.Error(p)
	}
	_, x = s.Agir(x.ID, "revisar", `{"cartao":`+num(c2.ID)+`,"nota":3}`)
	v = visao(t, s, x)
	if len(v.Cartoes) != 0 || v.Painel.Respostas != 2 || v.Painel.Acertos != 1 || v.Painel.Hoje != 2 || v.Painel.ProximaEm == nil {
		t.Errorf("%+v", v.Painel)
	}
	if v.Painel.MaisFracoID != c1.Assunto {
		t.Error("o mais fraco é o do cartão errado", v.Painel.MaisFracoID, c1.Assunto)
	}
	q1, q2 := v.Questoes[0], v.Questoes[1]
	r, x = s.Agir(x.ID, "responder", `{"questao":`+num(q1.ID)+`,"alternativa":1}`)
	if c := r.(estudo.Correcao); !c.Acertou || c.Explicacao == "" || c.Fonte.Texto == "" {
		t.Errorf("%+v", c)
	}
	r, x = s.Agir(x.ID, "responder", `{"questao":`+num(q2.ID)+`,"alternativa":0}`)
	if c := r.(estudo.Correcao); c.Acertou || c.Correta != 1 {
		t.Errorf("%+v", c)
	}
	v = visao(t, s, x)
	if v.Questoes[0].Situacao != "errada" || v.Questoes[1].Situacao != "certa" || v.Painel.Acertos != 2 {
		t.Errorf("%+v %+v", v.Questoes, v.Painel)
	}
	if len(s.Modelo.Pedidos()) != antes {
		t.Error("a sessão de estudo chamou o modelo")
	}
	for _, ruim := range [][2]string{{"gerar", `{"assunto":99}`}, {"gerar", `[]`}, {"revisar", `{"cartao":1,"nota":5}`},
		{"revisar", `{"cartao":999,"nota":3}`}, {"revisar", `[]`}, {"responder", `{"questao":999,"alternativa":0}`},
		{"responder", `{"questao":` + num(q1.ID) + `,"alternativa":7}`}, {"responder", `[]`}, {"voar", `{}`}} {
		if _, err := s.D.Gerente.Agir(s.Contexto, x.ID, ruim[0], []byte(ruim[1])); !errors.Is(err, atividades.ErrPedidoInvalido) {
			t.Error(ruim, err)
		}
	}
}

func num(n int64) string {
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func TestEditalColadoNaoViraMaterial(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	doc := s.Importar("edital.txt", "EDITAL\n\n"+edital+"\n")
	apostila := s.Importar("apostila.txt", "Atos administrativos produzem efeitos. Licitações e direitos fundamentais.")
	x := s.Criar("estudo", edital, "")
	for _, a := range testes.Dados[estudo.Dados](t, x).Assuntos {
		for _, id := range a.Trechos {
			tr, err := s.D.Acervo.Trechos(s.Contexto, []int64{id})
			if err != nil || tr[0].Documento == doc || tr[0].Documento != apostila {
				t.Error(a.Titulo, tr, err)
			}
		}
	}
}

func TestMaterialInvalidoEDescartado(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	s.Importar("a.txt", "Redes de computadores ligam máquinas.")
	validos := false
	s.Modelo.Definir(func(p simulador.PedidoRecebido) (simulador.Resposta, bool) {
		if p.Esquema() != "material_estudo" {
			return simulador.Resposta{}, false
		}
		q := func(fonte string, correta int, alt ...string) map[string]any {
			return map[string]any{"enunciado": "E " + strings.Join(alt, ""), "alternativas": alt, "correta": correta, "explicacao": "x", "fonte": fonte}
		}
		questoes := []any{q("T999", 0, "a", "b"), q("T1", 5, "a", "b"), q("T1", 0, "a"), q("T1", 0, "a", "A "), q("T1", 0, "a", ""),
			q("T1", 0, "a", "b", "c", "d", "e", "f"), q("T1", -1, "a", "b")}
		cartoes := []any{map[string]any{"frente": "", "verso": "v", "fonte": "T1"}, map[string]any{"frente": "f", "verso": " ", "fonte": "T1"}}
		if validos {
			questoes = append(questoes, q("[T1]", 0, "sim", "não"))
		}
		return simulador.Resposta{JSON: map[string]any{"cartoes": cartoes, "questoes": questoes}}, true
	})
	x := s.Criar("estudo", "Informática: Redes", "")
	_, x = s.Agir(x.ID, "gerar", `{"assunto":2}`)
	v := visao(t, s, x)
	if v.Painel.Questoes != 0 || v.Painel.Cartoes != 0 || !strings.Contains(v.Assuntos[1].Aviso, "descartado") {
		t.Errorf("%+v", v.Assuntos[1])
	}
	validos = true
	_, x = s.Agir(x.ID, "gerar", `{"assunto":2}`)
	v = visao(t, s, x)
	if v.Painel.Questoes != 1 || !strings.Contains(v.Assuntos[1].Aviso, "9 item(ns)") {
		t.Errorf("%+v %+v", v.Painel, v.Assuntos[1])
	}
}

func TestEntradasEEdital(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	for _, c := range [][2]string{{"", ""}, {"", `{"edital_documento":99}`}, {"x", `{"y":1}`}} {
		var o []byte
		if c[1] != "" {
			o = []byte(c[1])
		}
		if _, err := s.D.Gerente.Criar(s.Contexto, "estudo", "", c[0], o); !errors.Is(err, atividades.ErrPedidoInvalido) {
			t.Error(c, err)
		}
	}
	vazio := s.Importar("vazio.txt", "   \n ")
	x := s.Criar("estudo", "", `{"edital_documento":`+num(vazio)+`}`)
	if x.Estado != atividades.Esperando || !strings.Contains(x.Aviso, "não tem texto") {
		t.Error(x.Estado, x.Aviso)
	}
	if v := visao(t, s, x); v.Extraido || len(v.Assuntos) != 0 {
		t.Error(v)
	}
	s.Modelo.Definir(func(p simulador.PedidoRecebido) (simulador.Resposta, bool) {
		return simulador.Resposta{JSON: map[string]any{"assuntos": []any{map[string]any{"titulo": " ", "subtopicos": []string{}}}}}, p.Esquema() == "arvore_assuntos"
	})
	x = s.Criar("estudo", "nada", "")
	if x.Estado != atividades.Esperando || !strings.Contains(x.Aviso, "nenhum assunto") {
		t.Error(x.Aviso)
	}
	// Edital muito grande é cortado, e um assunto sem subtópicos é uma folha.
	s.Modelo.Definir(func(p simulador.PedidoRecebido) (simulador.Resposta, bool) {
		if len(p.Bruto) > 120_000 {
			t.Error("edital enviado sem limite", len(p.Bruto))
		}
		return simulador.Resposta{JSON: map[string]any{"assuntos": []any{map[string]any{"titulo": "Português", "subtopicos": []string{}}}}}, p.Esquema() == "arvore_assuntos"
	})
	longo := s.Importar("longo.txt", strings.Repeat("Conteúdo programático extenso do edital. ", 4000))
	x = s.Criar("estudo", "", `{"edital_documento":`+num(longo)+`}`)
	_, x = s.Agir(x.ID, "gerar", `{"assunto":1}`)
	if v := visao(t, s, x); !v.Assuntos[0].Folha || x.Estado != atividades.Pronta {
		t.Error(v.Assuntos, x.Aviso)
	}
}
