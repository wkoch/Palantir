package acervo

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Colecao é uma visão sobre documentos. Nada nos originais muda.
type Colecao struct {
	ID         int64  `json:"id"`
	Nome       string `json:"nome"`
	CriadaPor  string `json:"criada_por"` // usuario ou ia
	Documentos int    `json:"documentos"`
}

// Entidade é um item extraído de documentos.
type Entidade struct {
	Tipo       string `json:"tipo"`
	Valor      string `json:"valor"`
	Documentos int    `json:"documentos,omitempty"`
}

// ErrNomeInvalido indica um nome de coleção vazio ou longo demais.
var ErrNomeInvalido = errors.New("o nome da coleção deve ter de 1 a 60 caracteres")

// Colecoes lista as coleções em ordem alfabética.
func (a *Acervo) Colecoes(ctx context.Context) ([]Colecao, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT c.id, c.nome, c.criada_por, COUNT(cd.documento_id)
		FROM colecoes c LEFT JOIN colecao_documentos cd ON cd.colecao_id = c.id
		GROUP BY c.id HAVING c.criada_por = 'usuario' OR COUNT(cd.documento_id) > 0
		ORDER BY c.nome COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lista := []Colecao{}
	for rows.Next() {
		var c Colecao
		if err := rows.Scan(&c.ID, &c.Nome, &c.CriadaPor, &c.Documentos); err != nil {
			return nil, err
		}
		lista = append(lista, c)
	}
	return lista, rows.Err()
}

// CriarColecao cria uma coleção do usuário, assumindo uma homônima da IA.
func (a *Acervo) CriarColecao(ctx context.Context, nome string) (int64, error) {
	nome = strings.Join(strings.Fields(nome), " ")
	if nome == "" || len([]rune(nome)) > 60 {
		return 0, ErrNomeInvalido
	}
	var id int64
	err := a.DB.QueryRowContext(ctx, `INSERT INTO colecoes (nome, criada_por, criada_em) VALUES (?, 'usuario', ?)
		ON CONFLICT(nome) DO UPDATE SET criada_por = 'usuario' RETURNING id`, nome, time.Now().UnixMilli()).Scan(&id)
	if err == nil {
		a.avisar()
	}
	return id, err
}

// NaColecao inclui ou retira um documento de uma coleção.
func (a *Acervo) NaColecao(ctx context.Context, colecao, documento int64, incluir bool) error {
	q := `DELETE FROM colecao_documentos WHERE colecao_id = ? AND documento_id = ?`
	if incluir {
		q = `INSERT OR IGNORE INTO colecao_documentos (colecao_id, documento_id) VALUES (?, ?)`
	}
	_, err := a.DB.ExecContext(ctx, q, colecao, documento)
	if err != nil && strings.Contains(err.Error(), "FOREIGN KEY") {
		return ErrNaoEncontrado
	}
	if err == nil {
		a.avisar()
	}
	return err
}

// Entidades lista as entidades mais frequentes, com filtros opcionais.
func (a *Acervo) Entidades(ctx context.Context, tipo, contendo string, documento int64) ([]Entidade, error) {
	q := `SELECT tipo, valor, COUNT(*) FROM entidades WHERE 1 = 1`
	var args []any
	if tipo != "" {
		q += ` AND tipo = ?`
		args = append(args, tipo)
	}
	if contendo != "" {
		q += ` AND instr(lower(valor), lower(?)) > 0`
		args = append(args, contendo)
	}
	if documento > 0 {
		q += ` AND documento_id = ?`
		args = append(args, documento)
	}
	rows, err := a.DB.QueryContext(ctx, q+` GROUP BY tipo, valor ORDER BY 3 DESC, tipo, valor LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lista := []Entidade{}
	for rows.Next() {
		var e Entidade
		if err := rows.Scan(&e.Tipo, &e.Valor, &e.Documentos); err != nil {
			return nil, err
		}
		lista = append(lista, e)
	}
	return lista, rows.Err()
}

// Numeros resume o tamanho do acervo.
type Numeros struct {
	Documentos int   `json:"documentos"`
	Pendentes  int   `json:"pendentes"`
	ComErro    int   `json:"com_erro"`
	Trechos    int   `json:"trechos"`
	Vetores    int   `json:"vetores"`
	Bytes      int64 `json:"bytes"`
}

// Numeros conta o conteúdo do acervo.
func (a *Acervo) Numeros(ctx context.Context) (Numeros, error) {
	var n Numeros
	err := a.DB.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM documentos),
		(SELECT COUNT(*) FROM documentos WHERE estado = 'pendente'),
		(SELECT COUNT(*) FROM documentos WHERE estado = 'erro'),
		(SELECT COUNT(*) FROM trechos),
		(SELECT COUNT(*) FROM vetores),
		(SELECT COALESCE(SUM(tamanho), 0) FROM documentos)`).
		Scan(&n.Documentos, &n.Pendentes, &n.ComErro, &n.Trechos, &n.Vetores, &n.Bytes)
	return n, err
}
