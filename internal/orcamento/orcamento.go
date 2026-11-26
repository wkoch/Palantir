// Package orcamento contabiliza o custo das chamadas de modelo e interrompe o
// trabalho quando um limite é atingido.
package orcamento

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrEsgotado indica que o limite mensal ou da atividade foi atingido.
var ErrEsgotado = errors.New("orçamento esgotado")

// Gasto descreve uma chamada de modelo.
type Gasto struct {
	Atividade  int64
	Modelo     string
	Finalidade string
	TokensIn   int
	TokensOut  int
	CustoUSD   float64
}

// Livro é o registro de gastos.
type Livro struct {
	db     *sql.DB
	mensal func() float64
	agora  func() time.Time
}

// Novo cria o livro. mensal devolve o teto mensal vigente; zero bloqueia tudo.
func Novo(db *sql.DB, mensal func() float64) *Livro {
	return &Livro{db: db, mensal: mensal, agora: time.Now}
}

// Registrar grava um gasto.
func (l *Livro) Registrar(ctx context.Context, g Gasto) error {
	_, err := l.db.ExecContext(ctx, `INSERT INTO gastos
		(quando, atividade_id, modelo, finalidade, tokens_in, tokens_out, custo_usd)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		l.agora().UnixMilli(), g.Atividade, g.Modelo, g.Finalidade, g.TokensIn, g.TokensOut, g.CustoUSD)
	return err
}

func (l *Livro) inicioDoMes() int64 {
	a := l.agora()
	return time.Date(a.Year(), a.Month(), 1, 0, 0, 0, 0, a.Location()).UnixMilli()
}

// NoMes soma o gasto do mês corrente.
func (l *Livro) NoMes(ctx context.Context) (float64, error) {
	var v float64
	err := l.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(custo_usd), 0) FROM gastos WHERE quando >= ?`, l.inicioDoMes()).Scan(&v)
	return v, err
}

// DaAtividade soma todo o gasto de uma atividade.
func (l *Livro) DaAtividade(ctx context.Context, id int64) (float64, error) {
	var v float64
	err := l.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(custo_usd), 0) FROM gastos WHERE atividade_id = ?`, id).Scan(&v)
	return v, err
}

// Verificar devolve ErrEsgotado se uma nova chamada não puder ser feita. A
// atividade zero representa trabalho do acervo, limitado só pelo teto mensal.
func (l *Livro) Verificar(ctx context.Context, atividade int64) error {
	mes, err := l.NoMes(ctx)
	if err != nil {
		return err
	}
	if teto := l.mensal(); mes >= teto {
		return fmt.Errorf("%w: teto mensal de US$ %.2f atingido (gasto US$ %.4f)", ErrEsgotado, teto, mes)
	}
	if atividade <= 0 {
		return nil
	}
	var limite float64
	err = l.db.QueryRowContext(ctx, `SELECT orcamento_usd FROM atividades WHERE id = ?`, atividade).Scan(&limite)
	if err != nil {
		return fmt.Errorf("orçamento da atividade %d: %w", atividade, err)
	}
	gasto, err := l.DaAtividade(ctx, atividade)
	if err != nil {
		return err
	}
	if gasto >= limite {
		return fmt.Errorf("%w: limite da atividade de US$ %.2f atingido (gasto US$ %.4f)", ErrEsgotado, limite, gasto)
	}
	return nil
}

// Linha é um agrupamento de gastos do mês.
type Linha struct {
	Chave    string  `json:"chave"`
	Chamadas int     `json:"chamadas"`
	Tokens   int     `json:"tokens"`
	CustoUSD float64 `json:"custo_usd"`
}

// Resumo agrupa o gasto do mês por "modelo" ou "finalidade".
func (l *Livro) Resumo(ctx context.Context, por string) ([]Linha, error) {
	coluna := "modelo"
	if por == "finalidade" {
		coluna = "finalidade"
	}
	rows, err := l.db.QueryContext(ctx, `SELECT `+coluna+`, COUNT(*),
		SUM(tokens_in + tokens_out), SUM(custo_usd)
		FROM gastos WHERE quando >= ? GROUP BY 1 ORDER BY 4 DESC, 1`, l.inicioDoMes())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	linhas := []Linha{}
	for rows.Next() {
		var x Linha
		if err := rows.Scan(&x.Chave, &x.Chamadas, &x.Tokens, &x.CustoUSD); err != nil {
			return nil, err
		}
		linhas = append(linhas, x)
	}
	return linhas, rows.Err()
}
