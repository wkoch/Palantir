package servidor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/confirmacoes"
	"github.com/wkoch/Palantir/internal/segredos"
	"github.com/wkoch/Palantir/internal/telas"
)

func (s *Servidor) saude(http.ResponseWriter, *http.Request) (any, error) {
	return map[string]any{"ok": true, "versao": s.Versao}, nil
}

type itemDaFila struct {
	atividades.Atividade
	CustoUSD float64 `json:"custo_usd"`
}

type estadoGeral struct {
	Versao       string                     `json:"versao"`
	Modelos      string                     `json:"modelos"`
	Atividades   []itemDaFila               `json:"atividades"`
	Confirmacoes []confirmacoes.Confirmacao `json:"confirmacoes"`
	GastoMesUSD  float64                    `json:"gasto_mes_usd"`
	TetoMesUSD   float64                    `json:"teto_mes_usd"`
	Hora         time.Time                  `json:"hora"`
}

// estado alimenta a fila de atenção.
func (s *Servidor) estado(_ http.ResponseWriter, r *http.Request) (any, error) {
	ctx := r.Context()
	lista, err := s.D.Armazem.Listar(ctx, r.URL.Query().Get("arquivadas") == "1")
	if err != nil {
		return nil, err
	}
	e := estadoGeral{Versao: s.Versao, Modelos: string(s.D.Roteador.Estado(ctx)), Atividades: []itemDaFila{},
		TetoMesUSD: s.D.Config().Orcamento.MensalUSD, Hora: time.Now().UTC()}
	for _, x := range lista {
		custo, err := s.D.Livro.DaAtividade(ctx, x.ID)
		if err != nil {
			return nil, err
		}
		x.Objetivo = primeiros(x.Objetivo, 300)
		e.Atividades = append(e.Atividades, itemDaFila{x, custo})
	}
	if e.Confirmacoes, err = s.D.Confirmacoes.Pendentes(ctx); err != nil {
		return nil, err
	}
	e.GastoMesUSD, err = s.D.Livro.NoMes(ctx)
	return e, err
}

func primeiros(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func (s *Servidor) criar(w http.ResponseWriter, r *http.Request) (any, error) {
	in, err := corpo[struct {
		Tipo     string          `json:"tipo"`
		Titulo   string          `json:"titulo"`
		Objetivo string          `json:"objetivo"`
		Opcoes   json.RawMessage `json:"opcoes"`
	}](w, r)
	if err != nil {
		return nil, err
	}
	return s.D.Gerente.Criar(r.Context(), in.Tipo, in.Titulo, in.Objetivo, in.Opcoes)
}

func (s *Servidor) atividade(_ http.ResponseWriter, r *http.Request) (any, error) {
	ctx := r.Context()
	id, err := idDe(r, "id")
	if err != nil {
		return nil, err
	}
	x, err := s.D.Armazem.Obter(ctx, id)
	if err != nil {
		return nil, err
	}
	nome, dados, err := s.D.Gerente.Tela(ctx, x)
	if err != nil {
		return nil, err
	}
	// Nenhum dado chega à interface fora do esquema da tela.
	if err := telas.Validar(nome, dados); err != nil {
		return nil, fmt.Errorf("a tela %s recebeu dados inválidos: %w", nome, err)
	}
	eventos, err := s.D.Armazem.Eventos(ctx, id)
	if err != nil {
		return nil, err
	}
	custo, err := s.D.Livro.DaAtividade(ctx, id)
	if err != nil {
		return nil, err
	}
	todas, err := s.D.Confirmacoes.Pendentes(ctx)
	if err != nil {
		return nil, err
	}
	pendentes := []confirmacoes.Confirmacao{}
	for _, c := range todas {
		if c.Atividade == id {
			pendentes = append(pendentes, c)
		}
	}
	return map[string]any{"atividade": itemDaFila{x, custo}, "tela": nome, "dados": dados,
		"eventos": eventos, "confirmacoes": pendentes}, nil
}

func (s *Servidor) mudarEstado(w http.ResponseWriter, r *http.Request) (any, error) {
	id, err := idDe(r, "id")
	if err != nil {
		return nil, err
	}
	in, err := corpo[struct {
		Estado atividades.Estado `json:"estado"`
	}](w, r)
	if err != nil {
		return nil, err
	}
	return s.D.Gerente.Mudar(r.Context(), id, in.Estado)
}

func (s *Servidor) mudarOrcamento(w http.ResponseWriter, r *http.Request) (any, error) {
	id, err := idDe(r, "id")
	if err != nil {
		return nil, err
	}
	in, err := corpo[struct {
		USD float64 `json:"usd"`
	}](w, r)
	if err != nil {
		return nil, err
	}
	return s.D.Gerente.DefinirOrcamento(r.Context(), id, in.USD)
}

func (s *Servidor) agir(w http.ResponseWriter, r *http.Request) (any, error) {
	id, err := idDe(r, "id")
	if err != nil {
		return nil, err
	}
	acao := r.PathValue("acao")
	if acao == atividades.ConfirmacaoDecidida {
		return nil, erroHTTP{http.StatusForbidden, "ação reservada ao daemon"}
	}
	bruto, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if len(bruto) > 0 && !json.Valid(bruto) {
		return nil, erroHTTP{http.StatusBadRequest, "corpo inválido"}
	}
	resposta, err := s.D.Gerente.Agir(r.Context(), id, acao, bruto)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "resposta": resposta}, nil
}

func (s *Servidor) decidir(w http.ResponseWriter, r *http.Request) (any, error) {
	id, err := idDe(r, "id")
	if err != nil {
		return nil, err
	}
	in, err := corpo[struct {
		Aprovar *bool `json:"aprovar"`
	}](w, r)
	if err != nil {
		return nil, err
	}
	if in.Aprovar == nil {
		return nil, erroHTTP{http.StatusBadRequest, "informe aprovar: true ou false"}
	}
	return nil, s.D.DecidirConfirmacao(r.Context(), id, *in.Aprovar)
}

func (s *Servidor) definirSegredo(w http.ResponseWriter, r *http.Request) (any, error) {
	nome := r.PathValue("nome")
	in, err := corpo[struct {
		Valor string `json:"valor"`
	}](w, r)
	if err != nil {
		return nil, err
	}
	if err := segredos.Validar(nome); err != nil {
		return nil, err
	}
	if in.Valor == "" || len(in.Valor) > 4096 {
		return nil, erroHTTP{http.StatusBadRequest, "valor vazio ou longo demais"}
	}
	return nil, s.D.DefinirSegredo(r.Context(), nome, in.Valor)
}

func (s *Servidor) removerSegredo(_ http.ResponseWriter, r *http.Request) (any, error) {
	return nil, s.D.DefinirSegredo(r.Context(), r.PathValue("nome"), "")
}
