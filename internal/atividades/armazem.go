// Package atividades cuida do ciclo de vida das atividades: estado persistente,
// execução em segundo plano e retomada após reinício.
package atividades

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/wkoch/Palantir/internal/proveniencia"
)

// Estado de uma atividade.
type Estado string

// Estados possíveis.
const (
	Rodando   Estado = "rodando"
	Esperando Estado = "esperando_voce"
	Pronta    Estado = "pronta"
	Pausada   Estado = "pausada"
	Arquivada Estado = "arquivada"
)

// ErrNaoEncontrada indica um identificador inexistente.
var ErrNaoEncontrada = errors.New("atividade não encontrada")

// Atividade é a unidade básica do sistema.
type Atividade struct {
	ID           int64           `json:"id"`
	Tipo         string          `json:"tipo"`
	Titulo       string          `json:"titulo"`
	Objetivo     string          `json:"objetivo"`
	Estado       Estado          `json:"estado"`
	Aviso        string          `json:"aviso"`
	OrcamentoUSD float64         `json:"orcamento_usd"`
	Dados        json.RawMessage `json:"-"`
	CriadaEm     time.Time       `json:"criada_em"`
	AtualizadaEm time.Time       `json:"atualizada_em"`
}

// Evento é um item do histórico de uma atividade.
type Evento struct {
	ID        int64               `json:"id"`
	Atividade int64               `json:"atividade"`
	Quando    time.Time           `json:"quando"`
	Origem    proveniencia.Origem `json:"origem"`
	Tipo      string              `json:"tipo"`
	Texto     string              `json:"texto"`
}

// Armazem persiste atividades e eventos.
type Armazem struct {
	DB *sql.DB
	// Notificar é chamado depois de cada mudança.
	Notificar func(atividade int64)
}

func (a *Armazem) avisar(id int64) {
	if a.Notificar != nil {
		a.Notificar(id)
	}
}

const colunas = `id, tipo, titulo, objetivo, estado, aviso, orcamento_usd, dados, criada_em, atualizada_em`

type linha interface{ Scan(...any) error }

func ler(l linha) (Atividade, error) {
	var x Atividade
	var dados string
	var criada, atualizada int64
	err := l.Scan(&x.ID, &x.Tipo, &x.Titulo, &x.Objetivo, &x.Estado, &x.Aviso, &x.OrcamentoUSD,
		&dados, &criada, &atualizada)
	if errors.Is(err, sql.ErrNoRows) {
		return x, ErrNaoEncontrada
	}
	x.Dados = json.RawMessage(dados)
	x.CriadaEm, x.AtualizadaEm = time.UnixMilli(criada).UTC(), time.UnixMilli(atualizada).UTC()
	return x, err
}

// Inserir grava uma atividade nova e preenche o identificador.
func (a *Armazem) Inserir(ctx context.Context, x *Atividade) error {
	if len(x.Dados) == 0 {
		x.Dados = json.RawMessage("{}")
	}
	agora := time.Now()
	res, err := a.DB.ExecContext(ctx, `INSERT INTO atividades
		(tipo, titulo, objetivo, estado, aviso, orcamento_usd, dados, criada_em, atualizada_em)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		x.Tipo, x.Titulo, x.Objetivo, x.Estado, x.Aviso, x.OrcamentoUSD, string(x.Dados),
		agora.UnixMilli(), agora.UnixMilli())
	if err != nil {
		return err
	}
	if x.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	x.CriadaEm, x.AtualizadaEm = agora.UTC(), agora.UTC()
	a.avisar(x.ID)
	return nil
}

// Obter lê uma atividade.
func (a *Armazem) Obter(ctx context.Context, id int64) (Atividade, error) {
	return ler(a.DB.QueryRowContext(ctx, `SELECT `+colunas+` FROM atividades WHERE id = ?`, id))
}

// Listar devolve as atividades, das mais recentes para as mais antigas.
func (a *Armazem) Listar(ctx context.Context, comArquivadas bool) ([]Atividade, error) {
	q := `SELECT ` + colunas + ` FROM atividades`
	if !comArquivadas {
		q += ` WHERE estado <> 'arquivada'`
	}
	rows, err := a.DB.QueryContext(ctx, q+` ORDER BY atualizada_em DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lista := []Atividade{}
	for rows.Next() {
		x, err := ler(rows)
		if err != nil {
			return nil, err
		}
		lista = append(lista, x)
	}
	return lista, rows.Err()
}

// Atualizar lê, modifica e grava uma atividade numa única transação.
func (a *Armazem) Atualizar(ctx context.Context, id int64, mudar func(*Atividade) error) (Atividade, error) {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return Atividade{}, err
	}
	defer tx.Rollback()
	x, err := ler(tx.QueryRowContext(ctx, `SELECT `+colunas+` FROM atividades WHERE id = ?`, id))
	if err != nil {
		return x, err
	}
	if err := mudar(&x); err != nil {
		return x, err
	}
	x.AtualizadaEm = time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE atividades SET titulo = ?, estado = ?, aviso = ?,
		orcamento_usd = ?, dados = ?, atualizada_em = ? WHERE id = ?`,
		x.Titulo, x.Estado, x.Aviso, x.OrcamentoUSD, string(x.Dados), x.AtualizadaEm.UnixMilli(), id); err != nil {
		return x, err
	}
	if err := tx.Commit(); err != nil {
		return x, err
	}
	a.avisar(id)
	return x, nil
}

// Dados decodifica o estado próprio do tipo de atividade.
func Dados[T any](x Atividade) (T, error) {
	var d T
	if err := json.Unmarshal(x.Dados, &d); err != nil {
		return d, fmt.Errorf("dados da atividade %d: %w", x.ID, err)
	}
	return d, nil
}

// Modificar altera o estado próprio do tipo numa transação e devolve o novo
// valor.
func Modificar[T any](ctx context.Context, a *Armazem, id int64, mudar func(*T) error) (T, error) {
	var final T
	_, err := a.Atualizar(ctx, id, func(x *Atividade) error {
		d, err := Dados[T](*x)
		if err != nil {
			return err
		}
		if err := mudar(&d); err != nil {
			return err
		}
		b, err := json.Marshal(d)
		if err != nil {
			return err
		}
		x.Dados, final = b, d
		return nil
	})
	return final, err
}

// Registrar acrescenta um evento ao histórico.
func (a *Armazem) Registrar(ctx context.Context, id int64, origem proveniencia.Origem, tipo, texto string) error {
	_, err := a.DB.ExecContext(ctx, `INSERT INTO eventos (atividade_id, quando, origem, tipo, texto)
		VALUES (?, ?, ?, ?, ?)`, id, time.Now().UnixMilli(), origem, tipo, texto)
	if err == nil {
		a.avisar(id)
	}
	return err
}

// Eventos devolve o histórico em ordem cronológica.
func (a *Armazem) Eventos(ctx context.Context, id int64) ([]Evento, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT id, atividade_id, quando, origem, tipo, texto
		FROM eventos WHERE atividade_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lista := []Evento{}
	for rows.Next() {
		var e Evento
		var quando int64
		if err := rows.Scan(&e.ID, &e.Atividade, &quando, &e.Origem, &e.Tipo, &e.Texto); err != nil {
			return nil, err
		}
		e.Quando = time.UnixMilli(quando).UTC()
		lista = append(lista, e)
	}
	return lista, rows.Err()
}
