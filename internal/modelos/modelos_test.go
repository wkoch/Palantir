package modelos_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wkoch/Palantir/internal/banco"
	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/diario"
	"github.com/wkoch/Palantir/internal/esquema"
	"github.com/wkoch/Palantir/internal/modelos"
	"github.com/wkoch/Palantir/internal/orcamento"
	"github.com/wkoch/Palantir/internal/segredos"
	"github.com/wkoch/Palantir/internal/testes"
)

type ambiente struct {
	r     *modelos.Roteador
	livro *orcamento.Livro
	cofre segredos.Cofre
	cfg   config.Modelos
	teto  float64
}

func montar(t *testing.T, h http.HandlerFunc) *ambiente {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	db, err := banco.Abrir(ctx, filepath.Join(dir, "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	d, err := diario.Abrir(filepath.Join(dir, "d.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	a := &ambiente{cfg: config.Modelos{URL: srv.URL + "/v1/", Barato: "m/barato", Principal: "m/principal",
		Embeddings: "m/emb"}, teto: 100, cofre: segredos.Novo("arquivo", filepath.Join(dir, "s.json"))}
	a.livro = orcamento.Novo(db, func() float64 { return a.teto })
	a.r = &modelos.Roteador{Config: func() config.Modelos { return a.cfg }, Cofre: a.cofre, Livro: a.livro,
		Diario: d, DB: db, Espera: time.Millisecond}
	if err := a.cofre.Definir(ctx, segredos.OpenRouter, "k"); err != nil {
		t.Fatal(err)
	}
	return a
}

const ok = `{"choices":[{"message":{"content":"oi"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"cost":0.5}}`

func pedido() modelos.Pedido {
	return modelos.Pedido{Nivel: modelos.Barato, Finalidade: "teste",
		Mensagens: []modelos.Mensagem{{Papel: modelos.PapelUsuario, Conteudo: "olá"}}}
}

func TestCompletarEnviaTudoEContabiliza(t *testing.T) {
	var corpo map[string]any
	a := montar(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" || r.URL.Path != "/v1/chat/completions" {
			t.Error(r.Header, r.URL)
		}
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &corpo); err != nil {
			t.Error(err)
		}
		io.WriteString(w, `{"model":"m/x","choices":[{"message":{"content":"","tool_calls":[
			{"type":"function","function":{"name":"f","arguments":"{}"}}]}}],"usage":{"prompt_tokens":1000000,"completion_tokens":1000000}}`)
	})
	ctx := context.Background()
	if a.r.Estado(ctx) != modelos.EstadoOK {
		t.Error(a.r.Estado(ctx))
	}
	p := pedido()
	p.MaxTokens = 50
	p.Mensagens = append(p.Mensagens, modelos.Mensagem{Papel: modelos.PapelAssistente,
		Chamadas: []modelos.Chamada{{ID: "1", Nome: "f", Argumentos: "{}"}}},
		modelos.Mensagem{Papel: modelos.PapelFerramenta, ChamadaID: "1", Conteudo: "r"})
	p.Ferramentas = []modelos.Ferramenta{{Nome: "f", Descricao: "d", Esquema: []byte(`{"type":"object"}`)}}
	p.Esquema = &modelos.Esquema{Nome: "s", Definicao: []byte(`{"type":"object"}`)}
	r, err := a.r.Completar(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chamadas) != 1 || r.Chamadas[0].ID != "chamada-1" || r.Modelo != "m/x" {
		t.Errorf("%+v", r)
	}
	// Sem custo informado, usa o preço conservador.
	if r.CustoUSD != 30 {
		t.Error(r.CustoUSD)
	}
	if corpo["model"] != "m/barato" || corpo["max_tokens"] != float64(50) || corpo["tools"] == nil ||
		corpo["response_format"] == nil || len(corpo["messages"].([]any)) != 3 || corpo["tool_choice"] != nil ||
		corpo["messages"].([]any)[1].(map[string]any)["content"] != nil {
		t.Errorf("%v", corpo)
	}
	if v, _ := a.livro.NoMes(ctx); v != 30 {
		t.Error(v)
	}
	resumo, err := a.livro.Resumo(ctx, "finalidade")
	if err != nil || len(resumo) != 1 || resumo[0].Chave != "teste" || resumo[0].Tokens != 2000000 {
		t.Error(resumo, err)
	}
}

func TestCacheNaoCobraDuasVezes(t *testing.T) {
	var chamadas atomic.Int32
	a := montar(t, func(w http.ResponseWriter, _ *http.Request) { chamadas.Add(1); io.WriteString(w, ok) })
	p := pedido()
	p.Cache = true
	for i := range 3 {
		r, err := a.r.Completar(context.Background(), p)
		if err != nil || r.Conteudo != "oi" || r.DoCache != (i > 0) {
			t.Fatal(i, r, err)
		}
	}
	if chamadas.Load() != 1 {
		t.Error(chamadas.Load())
	}
}

func TestErros(t *testing.T) {
	ctx := context.Background()
	respostas := []struct {
		status int
		corpo  string
		quer   error
		vezes  int32
		estado modelos.Estado
	}{
		{401, `{"error":{"message":"chave ruim"}}`, modelos.ErrRecusado, 1, modelos.EstadoRecusado},
		{429, ``, modelos.ErrSemRede, 3, modelos.EstadoSemRede},
		{503, ``, modelos.ErrSemRede, 3, modelos.EstadoSemRede},
		{200, `{`, modelos.ErrRespostaRuim, 1, modelos.EstadoOK},
		{200, `{"choices":[]}`, modelos.ErrRespostaRuim, 1, modelos.EstadoOK},
		{200, `{"error":{"message":"modelo fora"}}`, modelos.ErrRecusado, 1, modelos.EstadoOK},
		{302, ``, modelos.ErrRecusado, 1, modelos.EstadoRecusado},
	}
	for _, c := range respostas {
		var vezes atomic.Int32
		a := montar(t, func(w http.ResponseWriter, _ *http.Request) {
			vezes.Add(1)
			// Um redirecionamento não pode levar a chave a outro lugar.
			w.Header().Set("Location", "http://atacante.invalid/")
			w.WriteHeader(c.status)
			io.WriteString(w, c.corpo)
		})
		_, err := a.r.Completar(ctx, pedido())
		if !errors.Is(err, c.quer) || vezes.Load() != c.vezes || a.r.Estado(ctx) != c.estado {
			t.Errorf("%d %q: err=%v vezes=%d estado=%s", c.status, c.corpo, err, vezes.Load(), a.r.Estado(ctx))
		}
	}
}

func TestCondicoesLocais(t *testing.T) {
	ctx := context.Background()
	a := montar(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, ok) })
	p := pedido()
	p.Nivel = modelos.Forte
	if _, err := a.r.Completar(ctx, p); !errors.Is(err, modelos.ErrSemModelo) {
		t.Error(err)
	}
	a.teto = 0
	if _, err := a.r.Completar(ctx, pedido()); !errors.Is(err, orcamento.ErrEsgotado) {
		t.Error(err)
	}
	a.teto = 1
	if _, err := a.r.Completar(ctx, pedido()); err != nil {
		t.Fatal(err)
	}
	p = pedido()
	p.Atividade = 42
	if _, err := a.r.Completar(ctx, p); err == nil {
		t.Error("atividade inexistente não tem orçamento")
	}
	if err := a.cofre.Remover(ctx, segredos.OpenRouter); err != nil {
		t.Fatal(err)
	}
	if _, err := a.r.Completar(ctx, pedido()); !errors.Is(err, modelos.ErrSemChave) || a.r.Estado(ctx) != modelos.EstadoSemChave {
		t.Error(err)
	}
	for _, e := range []error{modelos.ErrSemChave, modelos.ErrSemRede, modelos.ErrRecusado} {
		if !modelos.Indisponivel(e) {
			t.Error(e)
		}
	}
	if modelos.Indisponivel(modelos.ErrRespostaRuim) {
		t.Error("resposta ruim não é indisponibilidade")
	}
}

func TestSemRedeECancelamento(t *testing.T) {
	a := montar(t, func(http.ResponseWriter, *http.Request) {})
	a.cfg.URL = "http://" + testes.PortaFechada(t)
	if _, err := a.r.Completar(context.Background(), pedido()); !errors.Is(err, modelos.ErrSemRede) {
		t.Error(err)
	}
	a.r.Reavaliar()
	if a.r.Estado(context.Background()) != modelos.EstadoOK {
		t.Error("reavaliar limpa o estado")
	}
	ctx, cancelar := context.WithCancel(context.Background())
	a.r.Espera = time.Hour
	go func() { time.Sleep(50 * time.Millisecond); cancelar() }()
	if _, err := a.r.Completar(ctx, pedido()); !errors.Is(err, context.Canceled) {
		t.Error(err)
	}
}

func TestEmbeddings(t *testing.T) {
	corpo := `{"data":[{"index":1,"embedding":[0,1]},{"index":0,"embedding":[1,0]}],"usage":{"prompt_tokens":2,"cost":0.1}}`
	status := 200
	a := montar(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status); io.WriteString(w, corpo) })
	ctx := context.Background()
	v, modelo, err := a.r.Embeddings(ctx, 0, []string{"a", "b"})
	if err != nil || modelo != "m/emb" || v[0][0] != 1 || v[1][1] != 1 {
		t.Fatal(v, modelo, err)
	}
	if _, _, err := a.r.Embeddings(ctx, 0, []string{"a", "b", "c"}); !errors.Is(err, modelos.ErrRespostaRuim) {
		t.Error(err)
	}
	corpo = `{"error":{"message":"x"}}`
	if _, _, err := a.r.Embeddings(ctx, 0, []string{"a"}); !errors.Is(err, modelos.ErrRecusado) {
		t.Error(err)
	}
	status = 400
	if _, _, err := a.r.Embeddings(ctx, 0, []string{"a"}); !errors.Is(err, modelos.ErrRecusado) {
		t.Error(err)
	}
	a.cfg.Embeddings = ""
	if _, _, err := a.r.Embeddings(ctx, 0, []string{"a"}); !errors.Is(err, modelos.ErrSemModelo) {
		t.Error(err)
	}
}

type respostas struct {
	lista   []string
	pedidos []modelos.Pedido
	err     error
}

func (r *respostas) Completar(_ context.Context, p modelos.Pedido) (modelos.Resposta, error) {
	r.pedidos = append(r.pedidos, p)
	if r.err != nil {
		return modelos.Resposta{}, r.err
	}
	t := r.lista[0]
	r.lista = r.lista[1:]
	return modelos.Resposta{Conteudo: t}, nil
}

func TestCompletarJSON(t *testing.T) {
	ctx := context.Background()
	def := esquema.JSON(esquema.Objeto(map[string]any{"n": esquema.Inteiro, "b": esquema.Booleano,
		"x": esquema.Numero, "l": esquema.Lista(esquema.Enum("a"))}))
	p := modelos.Pedido{Cache: true, Esquema: &modelos.Esquema{Nome: "e", Definicao: def}}
	var v struct {
		N int `json:"n"`
	}
	r := &respostas{lista: []string{`{"n":"um"}`, "```json\n{\"n\":2,\"b\":true,\"x\":1.5,\"l\":[\"a\"]}\n```"}}
	if err := modelos.CompletarJSON(ctx, r, p, &v); err != nil || v.N != 2 {
		t.Fatal(v, err)
	}
	segundo := r.pedidos[1]
	if segundo.Cache || len(segundo.Mensagens) != 2 || !strings.Contains(segundo.Mensagens[1].Conteudo, "esquema") {
		t.Errorf("%+v", segundo)
	}
	r = &respostas{lista: []string{"nada", "{}"}}
	if err := modelos.CompletarJSON(ctx, r, p, &v); !errors.Is(err, modelos.ErrRespostaRuim) {
		t.Error(err)
	}
	r = &respostas{err: io.ErrUnexpectedEOF}
	if err := modelos.CompletarJSON(ctx, r, p, &v); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Error(err)
	}
	if modelos.CompletarJSON(ctx, r, modelos.Pedido{}, &v) == nil {
		t.Error("sem esquema")
	}
	p.Esquema.Definicao = []byte(`{"type":"nada"}`)
	if modelos.CompletarJSON(ctx, r, p, &v) == nil {
		t.Error("esquema inválido")
	}
}
