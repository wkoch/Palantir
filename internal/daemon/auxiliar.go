package daemon

import (
	"context"
	"encoding/json"
	"os"

	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/confirmacoes"
)

func mkdirPrivado(dir string) error { return os.MkdirAll(dir, 0o700) }

func (d *Daemon) agirInterno(ctx context.Context, c confirmacoes.Confirmacao) error {
	corpo, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = d.Gerente.Agir(ctx, c.Atividade, atividades.ConfirmacaoDecidida, corpo)
	return err
}
