package daemon

import (
	"context"

	"github.com/wkoch/Palantir/internal/diario"
	"github.com/wkoch/Palantir/internal/eventos"
	"github.com/wkoch/Palantir/internal/segredos"
)

// DefinirSegredo grava ou, com valor vazio, remove um segredo.
func (d *Daemon) DefinirSegredo(ctx context.Context, nome, valor string) error {
	if err := segredos.Validar(nome); err != nil {
		return err
	}
	var err error
	decisao := "definido"
	if valor == "" {
		decisao, err = "removido", d.Cofre.Remover(ctx, nome)
	} else {
		d.Diario.Ocultar(nome, valor)
		err = d.Cofre.Definir(ctx, nome, valor)
	}
	if err != nil {
		return err
	}
	if nome == segredos.OpenRouter {
		d.Roteador.Reavaliar()
		d.Acervo.Acordar()
	}
	d.Eventos.Publicar(eventos.Evento{Tipo: eventos.Ajustes})
	return d.Diario.Registrar(diario.Entrada{Tipo: "sistema", Nome: "segredo " + nome, Origem: "usuario", Decisao: decisao})
}
