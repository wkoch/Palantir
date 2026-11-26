package modelos_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wkoch/Palantir/internal/modelos"
)

var perguntas = map[string]modelos.Pergunta{
	"classe": {Tipo: modelos.Escolha, Instrucoes: "qual", Criterios: map[string]string{"a": "A", "b": "B"}},
	"sim":    {Tipo: modelos.SimOuNao, Instrucoes: "é", Criterios: map[string]string{"true": "s", "false": "n"}},
}

type corpoPrivado struct {
	Provider struct {
		ZDR            bool     `json:"zdr"`
		DataCollection string   `json:"data_collection"`
		Ignore         []string `json:"ignore"`
	} `json:"provider"`
}

func conferirPrivacidade(t *testing.T, corpo []byte) {
	t.Helper()
	var c corpoPrivado
	if err := json.Unmarshal(corpo, &c); err != nil {
		t.Fatal(err)
	}
	p := c.Provider
	if !p.ZDR || p.DataCollection != "deny" || len(p.Ignore) != 1 || p.Ignore[0] != "google-vertex" {
		t.Errorf("o pedido não exige retenção zero: %s", corpo)
	}
}

func TestTodoPedidoExigeRetencaoZero(t *testing.T) {
	var rotas []string
	a := montar(t, func(w http.ResponseWriter, r *http.Request) {
		rotas = append(rotas, r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		conferirPrivacidade(t, b)
		switch r.URL.Path {
		case "/v1/embeddings":
			io.WriteString(w, `{"data":[{"index":0,"embedding":[1]}],"usage":{"cost":0.1}}`)
		case "/alpha/decisions":
			io.WriteString(w, `{"model":"d/x","provider":"P","answers":{"classe":{"choice":"b","confidence":0.7},
				"sim":{"noul":0}},"usage":{"input_tokens":10,"output_tokens":2,"cost":0.25}}`)
		default:
			io.WriteString(w, `{"model":"escolhido/pelo-roteador","provider":"Azure",
				"choices":[{"message":{"content":"oi"}}],"usage":{"cost":0.5}}`)
		}
	})
	a.cfg.Decisoes = "m/decisoes"
	ctx := context.Background()
	r, err := a.r.Completar(ctx, pedido())
	if err != nil || r.Modelo != "escolhido/pelo-roteador" || r.Provedor != "Azure" {
		t.Fatal(r, err)
	}
	if _, _, err := a.r.Embeddings(ctx, 0, []string{"x"}); err != nil {
		t.Fatal(err)
	}
	j, err := a.r.Decidir(ctx, 0, "teste", map[string]any{"a": 1}, perguntas)
	if err != nil || j["classe"].Opcao != "b" || j["classe"].Confianca != 0.7 || j["sim"].Sim != 0 {
		t.Fatal(j, err)
	}
	if got := strings.Join(rotas, " "); got != "/v1/chat/completions /v1/embeddings /alpha/decisions" {
		t.Error(got)
	}
	if gasto, err := a.livro.NoMes(ctx); err != nil || gasto < 0.849 || gasto > 0.851 {
		t.Error(gasto, err)
	}
}

func TestDecisoesForaDoFormato(t *testing.T) {
	var resposta string
	a := montar(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, resposta) })
	ctx := context.Background()
	decidir := func() error {
		_, err := a.r.Decidir(ctx, 0, "teste", "x", perguntas)
		return err
	}
	if err := decidir(); !errors.Is(err, modelos.ErrSemModelo) || !modelos.SemDecisor(err) {
		t.Error(err)
	}
	a.cfg.Decisoes = "m/decisoes"
	for _, ruim := range []string{
		`{}`,
		`{"answers":{"classe":{"choice":"c"},"sim":{"noul":0.5}}}`,
		`{"answers":{"classe":{"choice":"a"},"sim":{}}}`,
		`{"answers":{"classe":{"choice":"a"},"sim":{"noul":2}}}`,
	} {
		resposta = ruim
		if err := decidir(); !errors.Is(err, modelos.ErrRespostaRuim) {
			t.Error(ruim, err)
		}
	}
	resposta = `{"error":{"message":"recusado"}}`
	if err := decidir(); !errors.Is(err, modelos.ErrRecusado) {
		t.Error(err)
	}
	a.teto = 0
	if err := decidir(); err == nil || modelos.SemDecisor(err) {
		t.Error("sem orçamento não se tenta outro caminho:", err)
	}
}
