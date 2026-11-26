// Package confirmacoes é a fila de ações que esperam o clique do usuário.
package confirmacoes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/wkoch/Palantir/internal/diario"
)

// Estados de uma confirmação.
const (
	Pendente = "pendente"
	Aprovada = "aprovada"
	Recusada = "recusada"
)

// ErrJaDecidida indica uma confirmação inexistente ou já respondida.
var ErrJaDecidida = errors.New("a confirmação não existe ou já foi respondida")

// Confirmacao é uma ação proposta.
type Confirmacao struct {
	ID         int64           `json:"id"`
	Atividade  int64           `json:"atividade"`
	Ferramenta string          `json:"ferramenta"`
	Argumentos json.RawMessage `json:"argumentos"`
	Motivos    []string        `json:"motivos"`
	Estado     string          `json:"estado"`
	CriadaEm   time.Time       `json:"criada_em"`
}

// Fila guarda as confirmações.
type Fila struct {
	DB     *sql.DB
	Diario *diario.Diario
}

// Criar enfileira uma ação. Um pedido idêntico ainda pendente é reaproveitado.
func (f *Fila) Criar(ctx context.Context, c Confirmacao) (int64, error) {
	motivos, err := json.Marshal(c.Motivos)
	if err != nil {
		return 0, err
	}
	var id int64
	err = f.DB.QueryRowContext(ctx, `SELECT id FROM confirmacoes
		WHERE atividade_id = ? AND ferramenta = ? AND argumentos = ? AND estado = ?`,
		c.Atividade, c.Ferramenta, string(c.Argumentos), Pendente).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := f.DB.ExecContext(ctx, `INSERT INTO confirmacoes
		(atividade_id, ferramenta, argumentos, motivos, estado, criada_em) VALUES (?, ?, ?, ?, ?, ?)`,
		c.Atividade, c.Ferramenta, string(c.Argumentos), string(motivos), Pendente, time.Now().UnixMilli())
	if err != nil {
		return 0, err
	}
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}
	return id, f.Diario.Registrar(diario.Entrada{Atividade: c.Atividade, Tipo: "confirmacao",
		Nome: c.Ferramenta, Decisao: Pendente, Detalhe: string(c.Argumentos)})
}

const colunas = `id, atividade_id, ferramenta, argumentos, motivos, estado, criada_em`

func ler(s interface{ Scan(...any) error }) (Confirmacao, error) {
	var c Confirmacao
	var args, motivos string
	var criada int64
	if err := s.Scan(&c.ID, &c.Atividade, &c.Ferramenta, &args, &motivos, &c.Estado, &criada); err != nil {
		return c, err
	}
	c.Argumentos = json.RawMessage(args)
	c.CriadaEm = time.UnixMilli(criada).UTC()
	return c, json.Unmarshal([]byte(motivos), &c.Motivos)
}

// Pendentes lista o que espera resposta, de atividades não arquivadas.
func (f *Fila) Pendentes(ctx context.Context) ([]Confirmacao, error) {
	rows, err := f.DB.QueryContext(ctx, `SELECT `+colunas+` FROM confirmacoes
		WHERE estado = ? AND atividade_id IN (SELECT id FROM atividades WHERE estado <> 'arquivada')
		ORDER BY id`, Pendente)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lista := []Confirmacao{}
	for rows.Next() {
		c, err := ler(rows)
		if err != nil {
			return nil, err
		}
		lista = append(lista, c)
	}
	return lista, rows.Err()
}

// Decidir grava a resposta do usuário. Só a interface chama este método, e uma
// confirmação é decidida uma única vez.
func (f *Fila) Decidir(ctx context.Context, id int64, aprovar bool) (Confirmacao, error) {
	estado := Recusada
	if aprovar {
		estado = Aprovada
	}
	c, err := ler(f.DB.QueryRowContext(ctx, `UPDATE confirmacoes SET estado = ?, decidida_em = ?
		WHERE id = ? AND estado = ? RETURNING `+colunas, estado, time.Now().UnixMilli(), id, Pendente))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrJaDecidida
	}
	if err != nil {
		return c, err
	}
	return c, f.Diario.Registrar(diario.Entrada{Atividade: c.Atividade, Tipo: "confirmacao",
		Nome: c.Ferramenta, Origem: "usuario", Decisao: estado, Detalhe: string(c.Argumentos)})
}
