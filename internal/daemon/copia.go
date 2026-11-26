package daemon

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/wkoch/Palantir/internal/copia"
	"github.com/wkoch/Palantir/internal/diario"
	"github.com/wkoch/Palantir/internal/discos"
	"github.com/wkoch/Palantir/internal/eventos"
)

// Discos lista e monta os destinos de uma cópia.
type Discos interface {
	Listar(ctx context.Context) ([]discos.Disco, error)
	Achar(ctx context.Context, dispositivo string) (discos.Disco, error)
	Montar(ctx context.Context, dispositivo string) (string, func(context.Context) error, error)
}

// EstadoDaCopia é a cópia em andamento ou a última feita desde o início.
type EstadoDaCopia struct {
	Rodando bool   `json:"rodando"`
	Disco   string `json:"disco"`
	// Arquivo é o nome gravado, dentro da pasta de cópias do disco.
	Arquivo string    `json:"arquivo,omitempty"`
	Erro    string    `json:"erro,omitempty"`
	Quando  time.Time `json:"quando"`
}

// ErrCopiaEmAndamento recusa duas cópias ao mesmo tempo.
var ErrCopiaEmAndamento = errors.New("já há uma cópia em andamento")

// Copia devolve o estado da cópia.
func (d *Daemon) Copia() EstadoDaCopia {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.copia
}

func (d *Daemon) definirCopia(e EstadoDaCopia) {
	d.mu.Lock()
	d.copia = e
	d.mu.Unlock()
	d.Eventos.Publicar(eventos.Evento{Tipo: eventos.Copia})
}

// tamanhoDaCopia estima o espaço necessário, sem contar com a compressão.
func (d *Daemon) tamanhoDaCopia(ctx context.Context) (int64, error) {
	n, err := d.Acervo.Numeros(ctx)
	if err != nil {
		return 0, err
	}
	total := n.Bytes
	for _, arquivo := range []string{d.Caminhos.Banco(), d.Caminhos.Banco() + "-wal", d.Caminhos.Diario()} {
		if info, err := os.Stat(arquivo); err == nil {
			total += info.Size()
		}
	}
	return total, nil
}

// Copiar inicia, em segundo plano, a cópia para o disco escolhido pelo usuário
// na tela. Nenhuma atividade nem o modelo têm acesso a isto.
func (d *Daemon) Copiar(ctx context.Context, dispositivo string) error {
	disco, err := d.Discos.Achar(ctx, dispositivo)
	if err != nil {
		return err
	}
	d.mu.Lock()
	if d.copia.Rodando {
		d.mu.Unlock()
		return ErrCopiaEmAndamento
	}
	estado := EstadoDaCopia{Rodando: true, Disco: disco.Nome, Quando: time.Now().UTC()}
	d.copia = estado
	d.copias.Add(1)
	vida := d.vida
	d.mu.Unlock()
	d.Eventos.Publicar(eventos.Evento{Tipo: eventos.Copia})
	go func() {
		defer d.copias.Done()
		arquivo, err := d.gravarCopia(vida, disco.Dispositivo, estado.Quando)
		estado.Rodando, estado.Arquivo, estado.Quando = false, arquivo, time.Now().UTC()
		decisao := "concluida"
		if err != nil {
			estado.Erro, decisao = d.Diario.Limpar(err.Error()), "erro"
		}
		if errDiario := d.Diario.Registrar(diario.Entrada{Tipo: "sistema", Nome: "copia", Decisao: decisao,
			Detalhe: disco.Dispositivo + " " + arquivo + estado.Erro}); errDiario != nil {
			d.Log.Warn("cópia", "erro", errDiario)
		}
		d.definirCopia(estado)
	}()
	return nil
}

func (d *Daemon) gravarCopia(ctx context.Context, dispositivo string, quando time.Time) (arquivo string, err error) {
	estimado, err := d.tamanhoDaCopia(ctx)
	if err != nil {
		return "", err
	}
	pasta, desmontar, err := d.Discos.Montar(ctx, dispositivo)
	if err != nil {
		return "", err
	}
	// Desmontar grava no disco o que falta; sem isso a cópia não está pronta.
	defer func() {
		prazo, cancelar := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancelar()
		if errDesmontar := desmontar(prazo); errDesmontar != nil {
			err = errors.Join(err, errDesmontar)
		}
	}()
	caminho, err := copia.Gravar(ctx, pasta, estimado, quando.Local(), func(ctx context.Context, w io.Writer) error {
		return d.Exportar(ctx, w)
	})
	if err != nil {
		return "", err
	}
	return filepath.Base(caminho), nil
}
