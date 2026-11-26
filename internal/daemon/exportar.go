package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/copia"
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

// Exportar grava um ZIP com tudo o que é preciso para refazer a instalação:
// originais, metadados, atividades, cartões, banco, configuração e diário. Os
// segredos e a chave de acesso ficam de fora.
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
	// Uma cópia consistente do banco, mesmo com atividades rodando.
	pasta, err := os.MkdirTemp(d.Caminhos.Execucao, "copia-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(pasta)
	banco := filepath.Join(pasta, "palantir.db")
	if _, err := d.DB.ExecContext(ctx, "VACUUM INTO ?", banco); err != nil {
		return fmt.Errorf("copiar o banco: %w", err)
	}
	return d.Acervo.Exportar(ctx, w, map[string]any{"atividades.json": exportadas, "cartoes.json": cartoes},
		map[string]string{
			copia.Banco:   banco,
			copia.Config:  d.Caminhos.Arquivo(),
			copia.Ajustes: d.Caminhos.Ajustes(),
			copia.Diario:  d.Caminhos.Diario(),
		})
}
