// Package copia grava uma cópia de segurança num disco, confere o que gravou e
// a restaura numa instalação vazia.
package copia

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Nomes dos arquivos dentro da cópia.
const (
	Banco     = "banco/palantir.db"
	Config    = "config/config.toml"
	Ajustes   = "config/ajustes.toml"
	Diario    = "diario/diario.jsonl"
	Originais = "originais/"
)

// Pasta é onde as cópias ficam no disco de destino.
const Pasta = "Palantir-copias"

// folga cobre os metadados e o que crescer durante a cópia.
const folga = 50 << 20

// ErrSemEspaco indica um destino pequeno demais.
var ErrSemEspaco = errors.New("não há espaço suficiente no disco")

// livre é substituída nos testes.
var livre = func(pasta string) (int64, error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(pasta, &s); err != nil {
		return 0, err
	}
	return int64(s.Bavail) * s.Bsize, nil
}

// Gravar escreve a cópia em destino/Palantir-copias e devolve o caminho. O
// arquivo só ganha o nome final depois de relido e conferido.
func Gravar(ctx context.Context, destino string, estimado int64, quando time.Time,
	exportar func(context.Context, io.Writer) error) (string, error) {
	disponivel, err := livre(destino)
	if err != nil {
		return "", err
	}
	if disponivel < estimado+folga {
		return "", fmt.Errorf("%w: são precisos cerca de %d MB e há %d MB", ErrSemEspaco,
			(estimado+folga)>>20, disponivel>>20)
	}
	pasta := filepath.Join(destino, Pasta)
	if err := os.MkdirAll(pasta, 0o700); err != nil {
		return "", err
	}
	final := filepath.Join(pasta, "palantir-"+quando.Format("2006-01-02-150405")+".zip")
	if _, err := os.Stat(final); err == nil {
		return "", fmt.Errorf("já existe uma cópia com o nome %s", filepath.Base(final))
	}
	parcial := final + ".parcial"
	f, err := os.OpenFile(parcial, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	err = exportar(ctx, f)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err == nil {
		err = Conferir(ctx, parcial)
	}
	if err == nil {
		err = os.Rename(parcial, final)
	}
	if err != nil {
		_ = os.Remove(parcial)
		return "", err
	}
	return final, nil
}

// Conferir relê todos os arquivos da cópia, o que confere a soma de cada um, e
// exige o banco.
func Conferir(ctx context.Context, caminho string) error {
	z, err := zip.OpenReader(caminho)
	if err != nil {
		return fmt.Errorf("a cópia não pôde ser relida: %w", err)
	}
	defer z.Close()
	temBanco := false
	for _, a := range z.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		temBanco = temBanco || a.Name == Banco
		r, err := a.Open()
		if err != nil {
			return fmt.Errorf("a cópia está corrompida em %s: %w", a.Name, err)
		}
		_, err = io.Copy(io.Discard, r)
		if err := errors.Join(err, r.Close()); err != nil {
			return fmt.Errorf("a cópia está corrompida em %s: %w", a.Name, err)
		}
	}
	if !temBanco {
		return errors.New("o arquivo não é uma cópia do Palantír: falta o banco")
	}
	return nil
}
