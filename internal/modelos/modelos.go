// Package modelos é o roteador de modelos: escolhe o nível, chama a API
// compatível com o OpenRouter, controla o cache, o orçamento e o diário.
package modelos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/diario"
	"github.com/wkoch/Palantir/internal/orcamento"
	"github.com/wkoch/Palantir/internal/segredos"
)

// Nivel é a faixa de modelo escolhida para uma tarefa.
type Nivel string

// Níveis disponíveis.
const (
	Barato    Nivel = "barato"
	Principal Nivel = "principal"
	Forte     Nivel = "forte"
)

// Papéis das mensagens.
const (
	PapelSistema    = "system"
	PapelUsuario    = "user"
	PapelAssistente = "assistant"
	PapelFerramenta = "tool"
)

// Erros que a interface trata de forma própria.
var (
	ErrSemChave     = errors.New("a chave do OpenRouter não foi configurada")
	ErrSemRede      = errors.New("não foi possível alcançar o serviço de modelos")
	ErrSemModelo    = errors.New("nenhum modelo configurado para este nível")
	ErrRecusado     = errors.New("o serviço de modelos recusou a chamada")
	ErrRespostaRuim = errors.New("o modelo devolveu uma resposta fora do formato pedido")
)

// Indisponivel informa se o erro é uma condição externa, resolvida pelo
// usuário ou pelo tempo, e não uma falha do programa.
func Indisponivel(err error) bool {
	return errors.Is(err, ErrSemChave) || errors.Is(err, ErrSemRede) || errors.Is(err, ErrRecusado)
}

// Chamada é um pedido de uso de ferramenta feito pelo modelo.
type Chamada struct {
	ID         string `json:"id"`
	Nome       string `json:"nome"`
	Argumentos string `json:"argumentos"`
}

// Mensagem é um item da conversa enviada ao modelo.
type Mensagem struct {
	Papel     string    `json:"papel"`
	Conteudo  string    `json:"conteudo"`
	Chamadas  []Chamada `json:"chamadas,omitempty"`
	ChamadaID string    `json:"chamada_id,omitempty"`
}

// Ferramenta descreve uma ferramenta oferecida ao modelo.
type Ferramenta struct {
	Nome      string
	Descricao string
	Esquema   json.RawMessage
}

// Esquema pede uma resposta em JSON com formato definido.
type Esquema struct {
	Nome      string
	Definicao json.RawMessage
}

// Pedido é uma chamada de modelo.
type Pedido struct {
	Nivel       Nivel
	Atividade   int64
	Finalidade  string
	Mensagens   []Mensagem
	Ferramentas []Ferramenta
	Esquema     *Esquema
	// SemChamadas mantém as ferramentas declaradas, como o histórico exige, mas
	// obriga uma resposta em texto.
	SemChamadas bool
	Cache       bool
	MaxTokens   int
}

// Resposta é o que o modelo devolveu.
type Resposta struct {
	Conteudo string    `json:"conteudo"`
	Chamadas []Chamada `json:"chamadas,omitempty"`
	// Modelo e Provedor são os que atenderam, e não os pedidos: um roteador
	// como o jev-router escolhe a cada chamada.
	Modelo    string  `json:"modelo"`
	Provedor  string  `json:"provedor,omitempty"`
	TokensIn  int     `json:"tokens_in"`
	TokensOut int     `json:"tokens_out"`
	CustoUSD  float64 `json:"custo_usd"`
	DoCache   bool    `json:"-"`
}

// Completador é o que os módulos precisam do roteador.
type Completador interface {
	Completar(ctx context.Context, p Pedido) (Resposta, error)
}

// Estado resume a disponibilidade dos modelos.
type Estado string

// Estados possíveis.
const (
	EstadoSemChave Estado = "sem_chave"
	EstadoOK       Estado = "ok"
	EstadoSemRede  Estado = "sem_rede"
	EstadoRecusado Estado = "recusado"
)

// Roteador implementa Completador sobre HTTP.
type Roteador struct {
	Config  func() config.Modelos
	Cofre   segredos.Cofre
	Livro   *orcamento.Livro
	Diario  *diario.Diario
	DB      *sql.DB
	Cliente *http.Client
	// Espera é a pausa antes da primeira nova tentativa.
	Espera time.Duration

	ultimo atomic.Value // Estado
}

const (
	tentativas      = 3
	maxTokensPadrao = 8000
)

// Preço usado quando o serviço não informa o custo. É alto de propósito, para
// que o orçamento continue protegendo o usuário.
const (
	precoEntradaUSD = 5.0 / 1e6
	precoSaidaUSD   = 25.0 / 1e6
)

// Estado informa se há chave e como foi a última chamada.
func (r *Roteador) Estado(ctx context.Context) Estado {
	chave, err := r.Cofre.Ler(ctx, segredos.OpenRouter)
	if err != nil || chave == "" {
		return EstadoSemChave
	}
	if e, ok := r.ultimo.Load().(Estado); ok {
		return e
	}
	return EstadoOK
}

// Reavaliar esquece o resultado da última chamada, por exemplo após a troca da
// chave.
func (r *Roteador) Reavaliar() { r.ultimo.Store(EstadoOK) }

func (r *Roteador) modelo(n Nivel) (string, error) {
	c := r.Config()
	m := map[Nivel]string{Barato: c.Barato, Principal: c.Principal, Forte: c.Forte}[n]
	if m == "" {
		return "", fmt.Errorf("%w: %q", ErrSemModelo, n)
	}
	return m, nil
}

type fioMensagem struct {
	Papel string `json:"role"`
	// Conteudo é nulo quando a mensagem só carrega chamadas de ferramenta.
	Conteudo  *string       `json:"content"`
	Chamadas  []fioToolCall `json:"tool_calls,omitempty"`
	ChamadaID string        `json:"tool_call_id,omitempty"`
}

type fioToolCall struct {
	ID     string `json:"id"`
	Tipo   string `json:"type"`
	Funcao struct {
		Nome       string `json:"name"`
		Argumentos string `json:"arguments"`
	} `json:"function"`
}

// privacidade acompanha todo pedido e não é configurável: só provedores com
// retenção zero de dados (ZDR), que não guardam nem treinam com o conteúdo. O
// Vertex fica de fora porque o OpenRouter pode contá-lo como ZDR mesmo quando
// ele retém conteúdo para análise de abuso ("Safety Retention").
func privacidade() map[string]any {
	return map[string]any{"zdr": true, "data_collection": "deny", "ignore": []string{"google-vertex"}}
}

func corpoDaCompletacao(modelo string, p Pedido) ([]byte, error) {
	msgs := make([]fioMensagem, len(p.Mensagens))
	for i, m := range p.Mensagens {
		msgs[i] = fioMensagem{Papel: m.Papel, ChamadaID: m.ChamadaID}
		if m.Conteudo != "" || len(m.Chamadas) == 0 {
			msgs[i].Conteudo = &m.Conteudo
		}
		for _, c := range m.Chamadas {
			tc := fioToolCall{ID: c.ID, Tipo: "function"}
			tc.Funcao.Nome, tc.Funcao.Argumentos = c.Nome, c.Argumentos
			msgs[i].Chamadas = append(msgs[i].Chamadas, tc)
		}
	}
	corpo := map[string]any{
		"model":    modelo,
		"messages": msgs,
		"usage":    map[string]any{"include": true},
		"provider": privacidade(),
	}
	corpo["max_tokens"] = maxTokensPadrao
	if p.MaxTokens > 0 {
		corpo["max_tokens"] = p.MaxTokens
	}
	if len(p.Ferramentas) > 0 {
		fs := make([]map[string]any, len(p.Ferramentas))
		for i, f := range p.Ferramentas {
			fs[i] = map[string]any{"type": "function", "function": map[string]any{
				"name": f.Nome, "description": f.Descricao, "parameters": f.Esquema,
			}}
		}
		corpo["tools"] = fs
		if p.SemChamadas {
			corpo["tool_choice"] = "none"
		}
	}
	if p.Esquema != nil {
		corpo["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{
			"name": p.Esquema.Nome, "strict": true, "schema": p.Esquema.Definicao,
		}}
	}
	return json.Marshal(corpo)
}

type fioUso struct {
	Entrada int      `json:"prompt_tokens"`
	Saida   int      `json:"completion_tokens"`
	Custo   *float64 `json:"cost"`
}

func (u fioUso) custo() float64 {
	if u.Custo != nil {
		return *u.Custo
	}
	return float64(u.Entrada)*precoEntradaUSD + float64(u.Saida)*precoSaidaUSD
}

type fioErro struct {
	Mensagem string `json:"message"`
}

// Completar chama o modelo do nível pedido.
func (r *Roteador) Completar(ctx context.Context, p Pedido) (Resposta, error) {
	var resp Resposta
	modelo, err := r.modelo(p.Nivel)
	if err != nil {
		return resp, err
	}
	corpo, err := corpoDaCompletacao(modelo, p)
	if err != nil {
		return resp, err
	}
	var chaveCache string
	if p.Cache && r.DB != nil {
		soma := sha256.Sum256(corpo)
		chaveCache = hex.EncodeToString(soma[:])
		var guardada string
		err := r.DB.QueryRowContext(ctx, `SELECT resposta FROM cache_modelos WHERE chave = ?`, chaveCache).Scan(&guardada)
		if err == nil && json.Unmarshal([]byte(guardada), &resp) == nil {
			resp.CustoUSD, resp.TokensIn, resp.TokensOut, resp.DoCache = 0, 0, 0, true
			return resp, nil
		}
		resp = Resposta{}
	}

	var fio struct {
		Modelo   string `json:"model"`
		Provedor string `json:"provider"`
		Escolhas []struct {
			Mensagem struct {
				Conteudo string        `json:"content"`
				Chamadas []fioToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Uso  fioUso   `json:"usage"`
		Erro *fioErro `json:"error"`
	}
	if err := r.enviar(ctx, p.Atividade, p.Finalidade, r.endereco("/chat/completions"), corpo, &fio); err != nil {
		return resp, err
	}
	if fio.Modelo == "" {
		fio.Modelo = modelo
	}
	custo := fio.Uso.custo()
	if err := r.contabilizar(ctx, p.Atividade, p.Finalidade, fio.Modelo, fio.Provedor, fio.Uso.Entrada, fio.Uso.Saida, custo); err != nil {
		return resp, err
	}
	if fio.Erro != nil {
		return resp, fmt.Errorf("%w: %s", ErrRecusado, fio.Erro.Mensagem)
	}
	if len(fio.Escolhas) == 0 {
		return resp, fmt.Errorf("%w: sem escolhas", ErrRespostaRuim)
	}
	m := fio.Escolhas[0].Mensagem
	resp = Resposta{Conteudo: m.Conteudo, Modelo: fio.Modelo, Provedor: fio.Provedor,
		TokensIn: fio.Uso.Entrada, TokensOut: fio.Uso.Saida, CustoUSD: custo}
	for i, c := range m.Chamadas {
		if c.ID == "" {
			c.ID = fmt.Sprintf("chamada-%d", i+1)
		}
		resp.Chamadas = append(resp.Chamadas, Chamada{ID: c.ID, Nome: c.Funcao.Nome, Argumentos: c.Funcao.Argumentos})
	}
	if chaveCache != "" && len(resp.Chamadas) == 0 {
		if b, err := json.Marshal(resp); err == nil {
			if _, err := r.DB.ExecContext(ctx, `INSERT OR REPLACE INTO cache_modelos(chave, resposta, criada_em)
				VALUES (?, ?, ?)`, chaveCache, string(b), time.Now().UnixMilli()); err != nil {
				return resp, err
			}
		}
	}
	return resp, nil
}

// Embeddings devolve um vetor por texto, usando o modelo configurado.
func (r *Roteador) Embeddings(ctx context.Context, atividade int64, textos []string) ([][]float32, string, error) {
	modelo := r.Config().Embeddings
	if modelo == "" {
		return nil, "", ErrSemModelo
	}
	corpo, err := json.Marshal(map[string]any{"model": modelo, "input": textos, "provider": privacidade()})
	if err != nil {
		return nil, "", err
	}
	var fio struct {
		Provedor string `json:"provider"`
		Dados    []struct {
			Indice int       `json:"index"`
			Vetor  []float32 `json:"embedding"`
		} `json:"data"`
		Uso  fioUso   `json:"usage"`
		Erro *fioErro `json:"error"`
	}
	if err := r.enviar(ctx, atividade, "embeddings", r.endereco("/embeddings"), corpo, &fio); err != nil {
		return nil, "", err
	}
	if err := r.contabilizar(ctx, atividade, "embeddings", modelo, fio.Provedor, fio.Uso.Entrada, 0, fio.Uso.custo()); err != nil {
		return nil, "", err
	}
	if fio.Erro != nil {
		return nil, "", fmt.Errorf("%w: %s", ErrRecusado, fio.Erro.Mensagem)
	}
	vetores := make([][]float32, len(textos))
	for _, d := range fio.Dados {
		if d.Indice >= 0 && d.Indice < len(vetores) {
			vetores[d.Indice] = d.Vetor
		}
	}
	for _, v := range vetores {
		if len(v) == 0 {
			return nil, "", fmt.Errorf("%w: embeddings incompletos", ErrRespostaRuim)
		}
	}
	return vetores, modelo, nil
}

func (r *Roteador) contabilizar(ctx context.Context, atividade int64, finalidade, modelo, provedor string, in, out int, custo float64) error {
	// O gasto já aconteceu: é registrado mesmo que o pedido tenha sido cancelado.
	ctx = context.WithoutCancel(ctx)
	if err := r.Livro.Registrar(ctx, orcamento.Gasto{Atividade: atividade, Modelo: modelo,
		Finalidade: finalidade, TokensIn: in, TokensOut: out, CustoUSD: custo}); err != nil {
		return fmt.Errorf("registrar gasto: %w", err)
	}
	return r.Diario.Registrar(diario.Entrada{Atividade: atividade, Tipo: "modelo", Nome: modelo,
		Decisao: "ok", Motivo: finalidade, Detalhe: provedor, CustoUSD: custo, Tokens: in + out})
}

func (r *Roteador) endereco(rota string) string {
	return strings.TrimRight(r.Config().URL, "/") + rota
}

// enviar faz o POST com verificação de orçamento e novas tentativas.
func (r *Roteador) enviar(ctx context.Context, atividade int64, finalidade, destinoURL string, corpo []byte, destino any) (err error) {
	defer func() {
		// Falta de chave ou de crédito não é uma chamada: não enche o diário.
		if err != nil && !errors.Is(err, ErrSemChave) && !errors.Is(err, orcamento.ErrEsgotado) {
			_ = r.Diario.Registrar(diario.Entrada{Atividade: atividade, Tipo: "modelo", Nome: caminhoDe(destinoURL),
				Decisao: "erro", Motivo: finalidade, Detalhe: err.Error()})
		}
	}()
	if err := r.Livro.Verificar(ctx, atividade); err != nil {
		return err
	}
	chave, err := r.Cofre.Ler(ctx, segredos.OpenRouter)
	if err != nil {
		return err
	}
	if chave == "" {
		return ErrSemChave
	}
	r.Diario.Ocultar(segredos.OpenRouter, chave)

	espera := r.Espera
	if espera == 0 {
		espera = time.Second
	}
	cliente := r.Cliente
	if cliente == nil {
		cliente = &http.Client{Timeout: 5 * time.Minute}
	}
	// O redirecionamento é recusado para a chave não seguir para outro host.
	semRedirecionar := *cliente
	semRedirecionar.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	for tentativa := 1; ; tentativa++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, destinoURL, bytes.NewReader(corpo))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+chave)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Title", "Palantir")

		res, err := semRedirecionar.Do(req)
		var repetir bool
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			r.ultimo.Store(EstadoSemRede)
			err, repetir = fmt.Errorf("%w: %v", ErrSemRede, err), true
		} else {
			conteudo, errLer := io.ReadAll(io.LimitReader(res.Body, 64<<20))
			res.Body.Close()
			switch {
			case errLer != nil:
				r.ultimo.Store(EstadoSemRede)
				err, repetir = fmt.Errorf("%w: %v", ErrSemRede, errLer), true
			case res.StatusCode == http.StatusOK:
				if errJSON := json.Unmarshal(conteudo, destino); errJSON != nil {
					return fmt.Errorf("%w: %v", ErrRespostaRuim, errJSON)
				}
				r.ultimo.Store(EstadoOK)
				return nil
			case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
				r.ultimo.Store(EstadoSemRede)
				err, repetir = fmt.Errorf("%w: HTTP %d", ErrSemRede, res.StatusCode), true
			default:
				r.ultimo.Store(EstadoRecusado)
				var f struct {
					Erro fioErro `json:"error"`
				}
				_ = json.Unmarshal(conteudo, &f)
				err = fmt.Errorf("%w: HTTP %d %s", ErrRecusado, res.StatusCode, r.Diario.Limpar(f.Erro.Mensagem))
			}
		}
		if !repetir || tentativa >= tentativas {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(espera << (tentativa - 1)):
		}
	}
}

// caminhoDe devolve só o caminho do endereço, para o diário.
func caminhoDe(endereco string) string {
	if u, err := url.Parse(endereco); err == nil {
		return u.Path
	}
	return endereco
}
