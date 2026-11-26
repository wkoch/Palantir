package simulador

import (
	"encoding/json"
	"net/http"
	"slices"
)

// PedidoDeDecisao é o corpo de uma chamada ao modelo de decisões.
type PedidoDeDecisao struct {
	Modelo    string         `json:"model"`
	Estado    map[string]any `json:"state"`
	Perguntas map[string]struct {
		Tipo      string            `json:"type"`
		Criterios map[string]string `json:"criteria"`
	} `json:"questions"`
	// Bruto é o corpo completo.
	Bruto string `json:"-"`
}

// DefinirDecisao troca o comportamento do decisor. Devolver false mantém o
// normal. Em Resposta valem JSON (as respostas por pergunta) e Status.
func (m *Modelo) DefinirDecisao(responder func(p PedidoDeDecisao) (Resposta, bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decidir = responder
}

// Decisoes devolve os pedidos de decisão já recebidos.
func (m *Modelo) Decisoes() []PedidoDeDecisao {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.decisoes)
}

// Sim monta a resposta de uma pergunta de sim ou não.
func Sim(p float64) map[string]any { return map[string]any{"type": "noul", "noul": p} }

// Opcao monta a resposta de uma pergunta de escolha.
func Opcao(opcao string, confianca float64) map[string]any {
	return map[string]any{"type": "choice", "choice": opcao, "confidence": confianca}
}

// DecisaoPadrao é o comportamento do decisor simulado.
func DecisaoPadrao(p PedidoDeDecisao) Resposta {
	email, _ := p.Estado["email"].(map[string]any)
	assunto, _ := email["assunto"].(string)
	inicio, _ := email["inicio"].(string)
	texto := assunto + " " + inicio
	respostas := map[string]any{}
	for nome, q := range p.Perguntas {
		switch {
		case nome == "classe":
			respostas[nome] = Opcao(classeDe(texto), 0.9)
		case nome == "ordem" && tentaMandar(texto):
			respostas[nome] = Sim(0.97)
		case q.Tipo == "noul":
			respostas[nome] = Sim(0.05)
		}
	}
	return Resposta{JSON: respostas}
}

func (m *Modelo) decisao(w http.ResponseWriter, corpo []byte) {
	var p PedidoDeDecisao
	// O estado pode não ser um objeto, como no teste de conexão.
	var solto struct {
		Modelo    string          `json:"model"`
		Estado    json.RawMessage `json:"state"`
		Perguntas json.RawMessage `json:"questions"`
	}
	if json.Unmarshal(corpo, &solto) != nil || json.Unmarshal(solto.Perguntas, &p.Perguntas) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_ = json.Unmarshal(solto.Estado, &p.Estado)
	p.Modelo, p.Bruto = solto.Modelo, string(corpo)
	m.mu.Lock()
	m.decisoes = append(m.decisoes, p)
	responder := m.decidir
	m.mu.Unlock()
	var resp Resposta
	usada := false
	if responder != nil {
		resp, usada = responder(p)
	}
	if !usada {
		resp = DecisaoPadrao(p)
	}
	if resp.Status != 0 {
		w.WriteHeader(resp.Status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": resp.Texto}})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"model": "simulado/" + p.Modelo, "provider": "Simulador",
		"answers": resp.JSON, "usage": map[string]any{"input_tokens": len(corpo) / 4, "output_tokens": 10, "cost": m.Custo}})
}
