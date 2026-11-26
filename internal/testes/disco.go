package testes

import (
	"context"
	"sync"

	"github.com/wkoch/Palantir/internal/discos"
)

// Disco é um pendrive de mentira: uma pasta.
type Disco struct {
	Pasta string
	// FalhaAoMontar e FalhaAoDesmontar simulam defeitos.
	FalhaAoMontar    error
	FalhaAoDesmontar error
	// Segurar, se definido, atrasa a montagem até ser fechado.
	Segurar chan struct{}

	mu           sync.Mutex
	desmontagens int
}

// Dispositivo é o nome do disco de mentira.
const Dispositivo = "/dev/teste1"

// Listar implementa daemon.Discos.
func (d *Disco) Listar(context.Context) ([]discos.Disco, error) {
	return []discos.Disco{{Dispositivo: Dispositivo, Nome: "Pendrive de teste", Sistema: "ext4",
		Tamanho: 16 << 30, Removivel: true}}, nil
}

// Achar implementa daemon.Discos.
func (d *Disco) Achar(ctx context.Context, dispositivo string) (discos.Disco, error) {
	if dispositivo != Dispositivo {
		return discos.Disco{}, discos.ErrDesconhecido
	}
	l, err := d.Listar(ctx)
	return l[0], err
}

// Montar implementa daemon.Discos.
func (d *Disco) Montar(ctx context.Context, dispositivo string) (string, func(context.Context) error, error) {
	if d.Segurar != nil {
		select {
		case <-d.Segurar:
		case <-ctx.Done():
			return "", nil, ctx.Err()
		}
	}
	if d.FalhaAoMontar != nil {
		return "", nil, d.FalhaAoMontar
	}
	return d.Pasta, func(context.Context) error {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.desmontagens++
		return d.FalhaAoDesmontar
	}, nil
}

// Desmontagens conta quantas vezes o disco foi liberado.
func (d *Disco) Desmontagens() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.desmontagens
}
