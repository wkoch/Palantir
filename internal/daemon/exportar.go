package daemon

import (
	"context"
	"encoding/json"
	"io"

	"github.com/wkoch/Palantir/internal/atividades"
)

type atividadeExportada struct {
	atividades.Atividade
	Dados   json.RawMessage     `json:"dados"`
	Eventos []atividades.Evento `json:"eventos"`
}

type cartaoExportado struct {
	Atividade int64  `json:"atividade"`
	Frente    string `json:"frente"`
	Verso     string `json:"verso"`
	Fonte     string `json:"fonte"`
}

// Exportar grava um ZIP com os originais, os metadados, as atividades e o
// material de estudo. Segredos ficam de fora.
func (d *Daemon) Exportar(ctx context.Context, w io.Writer) error {
	lista, err := d.Armazem.Listar(ctx, true)
	if err != nil {
		return err
	}
	exportadas := []atividadeExportada{}
	for _, x := range lista {
		eventos, err := d.Armazem.Eventos(ctx, x.ID)
		if err != nil {
			return err
		}
		exportadas = append(exportadas, atividadeExportada{x, x.Dados, eventos})
	}
	cartoes := []cartaoExportado{}
	rows, err := d.DB.QueryContext(ctx, `SELECT atividade_id, frente, verso, fonte FROM cartoes ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c cartaoExportado
		if err := rows.Scan(&c.Atividade, &c.Frente, &c.Verso, &c.Fonte); err != nil {
			return err
		}
		cartoes = append(cartoes, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return d.Acervo.Exportar(ctx, w, map[string]any{"atividades.json": exportadas, "cartoes.json": cartoes})
}
