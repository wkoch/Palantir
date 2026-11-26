// Package simulador imita o serviço de modelos, um servidor IMAP e uma página
// web. Serve à demonstração sem custo e aos testes de ponta a ponta.
package simulador

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Modelo imita a API do OpenRouter com respostas determinísticas.
type Modelo struct {
	// Atraso simula o tempo de resposta.
	Atraso time.Duration
	// Custo é cobrado a cada chamada.
	Custo float64
	// Responder, se definido, substitui a resposta padrão. Devolver false
	// mantém o comportamento normal.
	Responder func(p PedidoRecebido) (Resposta, bool)

	mu      sync.Mutex
	pedidos []PedidoRecebido
}

// MensagemRecebida é uma mensagem do pedido.
type MensagemRecebida struct {
	Papel    string `json:"role"`
	Conteudo string `json:"content"`
}

// PedidoRecebido é o corpo de uma chamada.
type PedidoRecebido struct {
	Modelo   string             `json:"model"`
	Mensagem []MensagemRecebida `json:"messages"`
	Ferram   []struct {
		Funcao struct {
			Nome string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
	Formato *struct {
		Esquema struct {
			Nome string `json:"name"`
		} `json:"json_schema"`
	} `json:"response_format"`
	// Bruto é o corpo completo.
	Bruto string `json:"-"`
}

// Esquema é o nome do formato pedido, ou vazio.
func (p PedidoRecebido) Esquema() string {
	if p.Formato == nil {
		return ""
	}
	return p.Formato.Esquema.Nome
}

// Oferece informa se a ferramenta foi oferecida.
func (p PedidoRecebido) Oferece(nome string) bool {
	return slices.ContainsFunc(p.Ferram, func(f struct {
		Funcao struct {
			Nome string `json:"name"`
		} `json:"function"`
	}) bool {
		return f.Funcao.Nome == nome
	})
}

// Ultima é a última mensagem.
func (p PedidoRecebido) Ultima() MensagemRecebida {
	if len(p.Mensagem) == 0 {
		return MensagemRecebida{}
	}
	return p.Mensagem[len(p.Mensagem)-1]
}

// Uso são as mensagens do usuário concatenadas.
func (p PedidoRecebido) usuario() string {
	var b strings.Builder
	for _, m := range p.Mensagem {
		if m.Papel == "user" {
			b.WriteString(m.Conteudo + "\n")
		}
	}
	return b.String()
}

// Chamada é um uso de ferramenta pedido pelo modelo simulado.
type Chamada struct {
	Nome       string
	Argumentos any
}

// Resposta é o que o modelo simulado devolve.
type Resposta struct {
	Texto    string
	JSON     any
	Chamadas []Chamada
	Status   int
}

// Pedidos devolve o que já foi recebido.
func (m *Modelo) Pedidos() []PedidoRecebido {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.pedidos)
}

// ServeHTTP implementa as rotas de completação e de embeddings.
func (m *Modelo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	chave := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if chave == "" || chave == "recusada" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "chave inválida"}})
		return
	}
	corpo, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if m.Atraso > 0 {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(m.Atraso):
		}
	}
	uso := map[string]any{"prompt_tokens": len(corpo) / 4, "completion_tokens": 100, "cost": m.Custo}
	switch {
	case strings.HasSuffix(r.URL.Path, "/embeddings"):
		var p struct {
			Entrada []string `json:"input"`
		}
		if json.Unmarshal(corpo, &p) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		dados := make([]map[string]any, len(p.Entrada))
		for i, t := range p.Entrada {
			dados[i] = map[string]any{"index": i, "embedding": Vetor(t)}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": dados, "usage": uso})
	case strings.HasSuffix(r.URL.Path, "/chat/completions"):
		var p PedidoRecebido
		if json.Unmarshal(corpo, &p) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		p.Bruto = string(corpo)
		m.mu.Lock()
		m.pedidos = append(m.pedidos, p)
		numero := len(m.pedidos)
		m.mu.Unlock()
		var resp Resposta
		usada := false
		if m.Responder != nil {
			resp, usada = m.Responder(p)
		}
		if !usada {
			resp = Padrao(p)
		}
		if resp.Status != 0 {
			w.WriteHeader(resp.Status)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": resp.Texto}})
			return
		}
		texto := resp.Texto
		if resp.JSON != nil {
			b, err := json.Marshal(resp.JSON)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			texto = string(b)
		}
		chamadas := []map[string]any{}
		for i, c := range resp.Chamadas {
			args, ok := c.Argumentos.(string)
			if !ok {
				b, err := json.Marshal(c.Argumentos)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				args = string(b)
			}
			chamadas = append(chamadas, map[string]any{"id": fmt.Sprintf("sim-%d-%d", numero, i), "type": "function",
				"function": map[string]string{"name": c.Nome, "arguments": args}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "simulado/" + p.Modelo, "usage": uso,
			"choices": []any{map[string]any{"message": map[string]any{"content": texto, "tool_calls": chamadas}}}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// EnderecoSugerido é a página que o modelo simulado "inventa". O domínio
// .invalid nunca resolve.
const EnderecoSugerido = "https://sugestao.exemplo.invalid/pagina"

const dimensoes = 128

func palavras(t string) []string {
	return strings.FieldsFunc(strings.ToLower(t), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

// Vetor é um embedding de saco de palavras: textos com palavras em comum ficam
// próximos.
func Vetor(texto string) []float32 {
	v := make([]float32, dimensoes)
	for _, p := range palavras(texto) {
		if len([]rune(p)) < 4 {
			continue
		}
		h := fnv.New32a()
		h.Write([]byte(p))
		v[h.Sum32()%dimensoes]++
	}
	var n float64
	for _, x := range v {
		n += float64(x * x)
	}
	if n == 0 {
		v[0] = 1
		return v
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / math.Sqrt(n))
	}
	return v
}

var (
	reBloco = regexp.MustCompile(`(?s)<dados_externos origem="[^"]*" fonte="([^"]*)">\n(.*?)\n</dados_externos>`)
	reMarca = regexp.MustCompile(`\[([TFW][0-9a-f]+)\]`)
	reURL   = regexp.MustCompile(`https?://[^\s<>"']+`)
	reData  = regexp.MustCompile(`\b\d{2}/\d{2}/\d{4}\b`)
	reValor = regexp.MustCompile(`R\$ ?[\d.]+,\d{2}`)
)

type bloco struct{ fonte, texto string }

func blocos(t string) []bloco {
	var b []bloco
	for _, m := range reBloco.FindAllStringSubmatch(t, -1) {
		b = append(b, bloco{m[1], m[2]})
	}
	return b
}

func frase(t string) string {
	t = reMarca.ReplaceAllString(t, "")
	t = strings.Join(strings.Fields(t), " ")
	if i := strings.Index(t, ". "); i > 20 {
		t = t[:i+1]
	}
	if r := []rune(t); len(r) > 180 {
		t = string(r[:180]) + "…"
	}
	return t
}

// Padrao é o comportamento do modelo simulado.
func Padrao(p PedidoRecebido) Resposta {
	switch p.Esquema() {
	case "plano_relatorio":
		return plano(p)
	case "secao_relatorio":
		return secao(p)
	case "classificacao_email":
		return classificar(p)
	case "arvore_assuntos":
		return arvore(p)
	case "material_estudo":
		return material(p)
	case "enriquecimento":
		return enriquecer(p)
	case "juiz":
		return Resposta{JSON: map[string]any{"bloquear": false, "motivo": "compatível com o pedido (simulado)"}}
	}
	return conversar(p)
}

func plano(p PedidoRecebido) Resposta {
	pedido, _, _ := strings.Cut(strings.TrimPrefix(p.usuario(), "Pedido: "), "\n")
	if strings.Contains(strings.ToLower(pedido), "[perguntar]") && !strings.Contains(p.usuario(), "Resposta do usuário:") {
		return Resposta{JSON: map[string]any{"perguntas": []string{"Qual período o relatório deve cobrir?"},
			"secoes": []any{}}}
	}
	urls := reURL.FindAllString(p.usuario(), -1)
	if urls == nil {
		urls = []string{}
	}
	secoes := []any{
		map[string]any{"titulo": "Contexto", "orientacao": "O que se sabe sobre o tema.",
			"buscas": []string{pedido}, "urls": urls},
		map[string]any{"titulo": "Pontos de atenção", "orientacao": "Prazos, valores e riscos.",
			"buscas": []string{"prazo valor multa " + pedido}, "urls": []string{}},
	}
	return Resposta{JSON: map[string]any{"perguntas": []string{}, "secoes": secoes}}
}

func secao(p PedidoRecebido) Resposta {
	texto := p.usuario()
	var correcao string
	if _, resto, ok := strings.Cut(texto, "Correção pedida pelo usuário: "); ok {
		linhas := strings.Split(texto, "Correção pedida pelo usuário: ")
		resto = linhas[len(linhas)-1]
		correcao, _, _ = strings.Cut(resto, "\n")
	}
	_, titulo, _ := strings.Cut(texto, "Seção: ")
	titulo, _, _ = strings.Cut(titulo, "\n")
	_, fontes, _ := strings.Cut(texto, "\nFontes:\n")
	paragrafos := []any{}
	for _, b := range blocos(fontes) {
		id, _, _ := strings.Cut(b.fonte, ":")
		prefixo := "Segundo a fonte: "
		if correcao != "" {
			prefixo = "Revisado (" + correcao + "): "
		}
		paragrafos = append(paragrafos, map[string]any{"texto": prefixo + frase(b.texto), "fontes": []string{id}})
		if len(paragrafos) == 2 || (titulo != "Contexto" && len(paragrafos) == 1) {
			break
		}
	}
	return Resposta{JSON: map[string]any{"paragrafos": paragrafos}}
}

func classificar(p PedidoRecebido) Resposta {
	itens := []any{}
	for _, b := range blocos(p.usuario()) {
		campos := map[string]string{}
		for _, l := range strings.Split(b.texto, "\n") {
			if k, v, ok := strings.Cut(l, ": "); ok {
				campos[k] = v
			}
		}
		uid, err := strconv.Atoi(campos["uid"])
		if err != nil {
			continue
		}
		t := strings.ToLower(campos["assunto"] + " " + campos["início"])
		tem := func(s ...string) bool {
			return slices.ContainsFunc(s, func(x string) bool { return strings.Contains(t, x) })
		}
		classe := "ler_depois"
		switch {
		case tem("ignore", "instruç", "assistente", "promoç", "desconto", "ganhe", "oferta"):
			classe = "lixo"
		case tem("urgente", "vence hoje", "último dia", "prazo"):
			classe = "urgente"
		case tem("pode", "poderia", "confirma", "reunião", "?"):
			classe = "responder"
		}
		itens = append(itens, map[string]any{"uid": uid, "classe": classe, "resumo": "Resumo: " + campos["assunto"]})
	}
	return Resposta{JSON: map[string]any{"itens": itens}}
}

func arvore(p PedidoRecebido) Resposta {
	texto := p.usuario()
	for _, b := range blocos(texto) {
		texto += "\n" + b.texto
	}
	assuntos := []any{}
	vistos := map[string]bool{}
	for _, l := range strings.Split(texto, "\n") {
		titulo, resto, ok := strings.Cut(strings.TrimSpace(l), ":")
		titulo = strings.TrimSpace(titulo)
		if !ok || titulo == "" || len(titulo) > 60 || strings.Contains(titulo, "<") || strings.TrimSpace(resto) == "" || vistos[titulo] {
			continue
		}
		var subs []string
		for _, s := range strings.Split(resto, ";") {
			if s = strings.Trim(strings.TrimSpace(s), "."); s != "" {
				subs = append(subs, s)
			}
		}
		vistos[titulo] = true
		assuntos = append(assuntos, map[string]any{"titulo": titulo, "subtopicos": subs})
	}
	if len(assuntos) == 0 {
		assuntos = append(assuntos, map[string]any{"titulo": "Conhecimentos gerais", "subtopicos": []string{"Tema principal"}})
	}
	return Resposta{JSON: map[string]any{"assuntos": assuntos}}
}

func material(p PedidoRecebido) Resposta {
	cartoes, questoes := []any{}, []any{}
	_, assunto, _ := strings.Cut(p.usuario(), "Assunto: ")
	assunto, _, _ = strings.Cut(assunto, "\n")
	for i, b := range blocos(p.usuario()) {
		if i == 4 {
			break
		}
		marca, _, _ := strings.Cut(b.fonte, ":")
		f := frase(b.texto)
		cartoes = append(cartoes, map[string]any{"frente": fmt.Sprintf("%s: o que diz o trecho %s?", assunto, marca),
			"verso": f, "fonte": marca})
		questoes = append(questoes, map[string]any{
			"enunciado": fmt.Sprintf("Sobre %s (%s), assinale a alternativa de acordo com o material.", assunto, marca),
			"alternativas": []string{"Nada consta no material.", f, "O oposto do que o material afirma.",
				"A matéria é de outra disciplina.", "Nenhuma das anteriores."},
			"correta": 1, "explicacao": "O trecho afirma: " + f, "fonte": marca})
	}
	return Resposta{JSON: map[string]any{"cartoes": cartoes, "questoes": questoes}}
}

func enriquecer(p PedidoRecebido) Resposta {
	b := blocos(p.usuario())
	if len(b) == 0 {
		return Resposta{JSON: map[string]any{"resumo": "", "entidades": []any{}, "colecoes": []string{}}}
	}
	t := b[0].texto
	entidades := []any{}
	vistas := map[string]bool{}
	acrescentar := func(tipo string, valores []string) {
		for _, v := range valores {
			if !vistas[v] && len(entidades) < 12 {
				vistas[v] = true
				entidades = append(entidades, map[string]any{"tipo": tipo, "valor": v})
			}
		}
	}
	acrescentar("data", reData.FindAllString(t, -1))
	acrescentar("valor", reValor.FindAllString(t, -1))
	colecoes := []string{}
	baixo := strings.ToLower(t + " " + b[0].fonte)
	for _, c := range [][2]string{{"contrato", "Contratos"}, {"edital", "Concursos"}, {"apostila", "Estudo"}, {"fatura", "Finanças"}} {
		if strings.Contains(baixo, c[0]) && len(colecoes) < 2 {
			colecoes = append(colecoes, c[1])
		}
	}
	return Resposta{JSON: map[string]any{"resumo": frase(t), "entidades": entidades, "colecoes": colecoes}}
}

func conversar(p PedidoRecebido) Resposta {
	ultima := p.Ultima()
	texto := ultima.Conteudo
	if ultima.Papel == "tool" || strings.Contains(texto, "<dados_externos") {
		b := blocos(texto)
		if len(b) == 0 {
			if strings.Contains(texto, "bloqueada") {
				return Resposta{Texto: "Não consegui fazer isso: a chamada foi bloqueada pelas regras de segurança."}
			}
			return Resposta{Texto: "Pronto. " + frase(texto)}
		}
		var s strings.Builder
		s.WriteString("Encontrei o seguinte:\n")
		vistas := map[string]bool{}
		for _, trecho := range strings.Split(b[0].texto, "\n\n") {
			m := reMarca.FindStringSubmatch(trecho)
			if m == nil || vistas[m[1]] || len(vistas) == 3 {
				continue
			}
			vistas[m[1]] = true
			_, corpo, ok := strings.Cut(trecho, "\n")
			if !ok {
				corpo = trecho
			}
			if strings.HasPrefix(corpo, "http") {
				corpo = b[0].texto[strings.Index(b[0].texto, corpo)+len(corpo):]
			}
			s.WriteString("- " + frase(corpo) + " [" + m[1] + "]\n")
		}
		return Resposta{Texto: s.String()}
	}
	baixo := strings.ToLower(texto)
	// Um endereço que o usuário não citou: exercita a fila de confirmação.
	if strings.Contains(baixo, "página sugerida") && p.Oferece("buscar_url") {
		return Resposta{Chamadas: []Chamada{{"buscar_url", map[string]any{"url": EnderecoSugerido}}}}
	}
	if u := reURL.FindString(texto); u != "" && p.Oferece("buscar_url") {
		return Resposta{Chamadas: []Chamada{{"buscar_url", map[string]any{"url": strings.TrimRight(u, ".,)?!")}}}}
	}
	if (strings.Contains(baixo, "indicadores") || strings.Contains(baixo, "tabela")) && p.Oferece("mostrar_tela") {
		tela, dados := "indicadores", `{"titulo":"Indicadores de exemplo","indicadores":[{"rotulo":"Documentos","valor":"3"},{"rotulo":"Pendências","valor":"1"}]}`
		if strings.Contains(baixo, "tabela") {
			tela, dados = "tabela", `{"titulo":"Tabela de exemplo","colunas":["Item","Situação"],"linhas":[["Contrato","vigente"],["Fatura","a pagar"]]}`
		}
		return Resposta{Chamadas: []Chamada{{"mostrar_tela", map[string]any{"tela": tela, "dados_json": dados}}}}
	}
	if (strings.Contains(texto, "?") || strings.Contains(baixo, "acervo") || strings.Contains(baixo, "sobre")) && p.Oferece("buscar_acervo") {
		return Resposta{Chamadas: []Chamada{{"buscar_acervo", map[string]any{"consulta": texto}}}}
	}
	return Resposta{Texto: "Estou no **modo de simulação**: nenhuma chamada paga é feita. Pergunte sobre os " +
		"documentos do acervo, cole um endereço da página de exemplo ou peça uma tabela."}
}
