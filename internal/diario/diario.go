// Package diario registra cada chamada de modelo e de ferramenta num arquivo
// JSONL legível só pelo usuário, sem segredos.
package diario

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Entrada é uma linha do diário.
type Entrada struct {
	Quando    time.Time `json:"quando"`
	Atividade int64     `json:"atividade,omitempty"`
	Tipo      string    `json:"tipo"` // modelo, ferramenta, rede, confirmacao, sistema
	Nome      string    `json:"nome"`
	Origem    string    `json:"origem,omitempty"`
	Decisao   string    `json:"decisao,omitempty"`
	Motivo    string    `json:"motivo,omitempty"`
	CustoUSD  float64   `json:"custo_usd,omitempty"`
	Tokens    int       `json:"tokens,omitempty"`
	Detalhe   string    `json:"detalhe,omitempty"`
}

// Diario grava entradas em um arquivo. Um *Diario nulo descarta tudo.
type Diario struct {
	caminho string
	mu      sync.Mutex
	segredo map[string]string
}

// Abrir prepara o diário no caminho indicado.
func Abrir(caminho string) (*Diario, error) {
	f, err := os.OpenFile(caminho, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return &Diario{caminho: caminho, segredo: map[string]string{}}, nil
}

// Ocultar registra um valor que nunca deve aparecer no diário.
func (d *Diario) Ocultar(nome, valor string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if valor == "" {
		delete(d.segredo, nome)
		return
	}
	d.segredo[nome] = valor
}

var padroesSensiveis = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]{12,}`),
	regexp.MustCompile(`\b\d{3}\.\d{3}\.\d{3}-\d{2}\b`), // CPF
}

// Limpar remove segredos conhecidos e padrões sensíveis de um texto.
func (d *Diario) Limpar(texto string) string {
	if d != nil {
		d.mu.Lock()
		for _, v := range d.segredo {
			texto = strings.ReplaceAll(texto, v, "[segredo]")
		}
		d.mu.Unlock()
	}
	for _, p := range padroesSensiveis {
		texto = p.ReplaceAllString(texto, "[removido]")
	}
	return texto
}

const maxDetalhe = 2000

// Registrar acrescenta uma entrada. Falhas de gravação são devolvidas para que
// o chamador possa recusar a ação que não conseguiu registrar.
func (d *Diario) Registrar(e Entrada) error {
	if d == nil {
		return nil
	}
	if e.Quando.IsZero() {
		e.Quando = time.Now().UTC()
	}
	e.Motivo = d.Limpar(e.Motivo)
	e.Detalhe = d.Limpar(e.Detalhe)
	if r := []rune(e.Detalhe); len(r) > maxDetalhe {
		e.Detalhe = string(r[:maxDetalhe]) + "…"
	}
	linha, err := json.Marshal(e)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	f, err := os.OpenFile(d.caminho, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(linha, '\n'))
	return errors.Join(err, f.Close())
}

// Ultimas devolve até n entradas, da mais recente para a mais antiga. Se
// atividade for maior que zero, filtra por ela.
func (d *Diario) Ultimas(n int, atividade int64) ([]Entrada, error) {
	if d == nil || n <= 0 {
		return nil, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	f, err := os.Open(d.caminho)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	anel := make([]Entrada, 0, n)
	r := bufio.NewReader(f)
	for {
		linha, err := r.ReadBytes('\n')
		if len(linha) > 0 {
			var e Entrada
			if json.Unmarshal(linha, &e) == nil && (atividade <= 0 || e.Atividade == atividade) {
				if len(anel) == n {
					anel = anel[1:]
				}
				anel = append(anel, e)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	for i, j := 0, len(anel)-1; i < j; i, j = i+1, j-1 {
		anel[i], anel[j] = anel[j], anel[i]
	}
	return anel, nil
}
