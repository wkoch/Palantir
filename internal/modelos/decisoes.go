package modelos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Tipos de pergunta de um modelo de decisões.
const (
	// SimOuNao devolve a probabilidade de a resposta ser sim.
	SimOuNao = "noul"
	// Escolha devolve uma das opções dos critérios.
	Escolha = "choice"
)

// Pergunta é uma decisão fechada pedida sobre um estado.
type Pergunta struct {
	Tipo       string            `json:"type"`
	Instrucoes string            `json:"instructions"`
	Criterios  map[string]string `json:"criteria"`
}

// Julgamento é a resposta de uma pergunta.
type Julgamento struct {
	// Opcao e Confianca respondem a uma pergunta de Escolha.
	Opcao     string
	Confianca float64
	// Sim é a probabilidade de uma pergunta SimOuNao.
	Sim float64
}

// Decisor responde perguntas fechadas, sem gerar texto.
type Decisor interface {
	Decidir(ctx context.Context, atividade int64, finalidade string, estado any,
		perguntas map[string]Pergunta) (map[string]Julgamento, error)
}

// enderecoDeDecisoes deriva a rota de decisões da URL de conversa: no
// OpenRouter ela fica ao lado de /v1, em /alpha.
func (r *Roteador) enderecoDeDecisoes() string {
	base := strings.TrimSuffix(strings.TrimRight(r.Config().URL, "/"), "/v1")
	return base + "/alpha/decisions"
}

// Decidir consulta o modelo de decisões configurado. A resposta só é aceita se
// trouxer, para cada pergunta, um valor do tipo e das opções pedidas.
func (r *Roteador) Decidir(ctx context.Context, atividade int64, finalidade string, estado any,
	perguntas map[string]Pergunta) (map[string]Julgamento, error) {
	modelo := r.Config().Decisoes
	if modelo == "" {
		return nil, fmt.Errorf("%w: decisões", ErrSemModelo)
	}
	corpo, err := json.Marshal(map[string]any{"model": modelo, "state": estado, "questions": perguntas,
		"provider": privacidade()})
	if err != nil {
		return nil, err
	}
	var fio struct {
		Modelo    string `json:"model"`
		Provedor  string `json:"provider"`
		Respostas map[string]struct {
			Tipo      string   `json:"type"`
			Opcao     string   `json:"choice"`
			Confianca float64  `json:"confidence"`
			Sim       *float64 `json:"noul"`
		} `json:"answers"`
		Uso struct {
			Entrada int      `json:"input_tokens"`
			Saida   int      `json:"output_tokens"`
			Custo   *float64 `json:"cost"`
		} `json:"usage"`
		Erro *fioErro `json:"error"`
	}
	if err := r.enviar(ctx, atividade, finalidade, r.enderecoDeDecisoes(), corpo, &fio); err != nil {
		return nil, err
	}
	if fio.Modelo == "" {
		fio.Modelo = modelo
	}
	uso := fioUso{Entrada: fio.Uso.Entrada, Saida: fio.Uso.Saida, Custo: fio.Uso.Custo}
	if err := r.contabilizar(ctx, atividade, finalidade, fio.Modelo, fio.Provedor, uso.Entrada, uso.Saida, uso.custo()); err != nil {
		return nil, err
	}
	if fio.Erro != nil {
		return nil, fmt.Errorf("%w: %s", ErrRecusado, fio.Erro.Mensagem)
	}
	julgamentos := make(map[string]Julgamento, len(perguntas))
	for nome, p := range perguntas {
		a, ok := fio.Respostas[nome]
		switch p.Tipo {
		case SimOuNao:
			ok = ok && a.Sim != nil && *a.Sim >= 0 && *a.Sim <= 1
		case Escolha:
			_, conhecida := p.Criterios[a.Opcao]
			ok = ok && conhecida
		}
		if !ok {
			return nil, fmt.Errorf("%w: decisão %q ausente ou fora das opções", ErrRespostaRuim, nome)
		}
		j := Julgamento{Opcao: a.Opcao, Confianca: a.Confianca}
		if a.Sim != nil {
			j.Sim = *a.Sim
		}
		julgamentos[nome] = j
	}
	return julgamentos, nil
}

// SemDecisor informa se o erro permite usar um modelo de conversa no lugar: o
// serviço de decisões é experimental e pode faltar ou mudar de formato.
func SemDecisor(err error) bool {
	return errors.Is(err, ErrSemModelo) || errors.Is(err, ErrRecusado) || errors.Is(err, ErrRespostaRuim)
}
