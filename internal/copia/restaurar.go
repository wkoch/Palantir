package copia

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wkoch/Palantir/internal/banco"
	"github.com/wkoch/Palantir/internal/config"
)

// ErrNaoVazia protege uma instalação com dados.
var ErrNaoVazia = errors.New("esta instalação já tem documentos ou atividades; a restauração só vale numa vazia")

var reOriginal = regexp.MustCompile(`^originais/([0-9a-f]{64})/`)

// vazia informa se não há nada que a restauração pudesse substituir.
func vazia(ctx context.Context, caminhos config.Caminhos) (bool, error) {
	if _, err := os.Stat(caminhos.Banco()); errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	db, err := banco.Abrir(ctx, caminhos.Banco())
	if err != nil {
		return false, err
	}
	defer db.Close()
	var n int
	err = db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM documentos) + (SELECT COUNT(*) FROM atividades)`).Scan(&n)
	return n == 0, err
}

// extrair grava um arquivo da cópia e devolve a soma do conteúdo. Nunca segue
// um link nem substitui um original.
func extrair(a *zip.File, destino string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(destino), 0o700); err != nil {
		return "", err
	}
	r, err := a.Open()
	if err != nil {
		return "", err
	}
	defer r.Close()
	tmp, err := os.CreateTemp(filepath.Dir(destino), ".restaurando-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	soma := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmp, soma), r)
	if err := errors.Join(err, tmp.Sync(), tmp.Close()); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), destino); err != nil {
		return "", err
	}
	return hex.EncodeToString(soma.Sum(nil)), nil
}

// Restaurar recompõe uma instalação vazia a partir de uma cópia. O daemon não
// pode estar em execução. Os segredos precisam ser definidos de novo.
func Restaurar(ctx context.Context, caminhos config.Caminhos, arquivo string) (originais int, err error) {
	if err := Conferir(ctx, arquivo); err != nil {
		return 0, err
	}
	ok, err := vazia(ctx, caminhos)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrNaoVazia
	}
	z, err := zip.OpenReader(arquivo)
	if err != nil {
		return 0, err
	}
	defer z.Close()
	var oBanco *zip.File
	for _, a := range z.File {
		if err := ctx.Err(); err != nil {
			return originais, err
		}
		// Só nomes conhecidos viram arquivos: nada do ZIP escolhe um caminho.
		switch {
		case a.Name == Banco:
			oBanco = a
		case a.Name == Config:
			_, err = extrair(a, caminhos.Arquivo())
		case a.Name == Ajustes:
			_, err = extrair(a, caminhos.Ajustes())
		case a.Name == Diario:
			_, err = extrair(a, caminhos.Diario())
		case strings.HasPrefix(a.Name, Originais):
			m := reOriginal.FindStringSubmatch(a.Name)
			if m == nil {
				return originais, fmt.Errorf("nome inesperado na cópia: %q", a.Name)
			}
			destino := filepath.Join(caminhos.Originais(), m[1])
			if _, errStat := os.Lstat(destino); errStat == nil {
				continue // um original já presente não é substituído
			}
			var soma string
			if soma, err = extrair(a, destino); err == nil && soma != m[1] {
				_ = os.Remove(destino)
				err = errors.New("o conteúdo não confere com a soma")
			}
			if err == nil {
				originais++
				err = os.Chmod(destino, 0o400)
			}
		}
		if err != nil {
			return originais, fmt.Errorf("restaurar %s: %w", a.Name, err)
		}
	}
	// O banco entra por último: até aqui a instalação continua vazia.
	for _, resto := range []string{"-wal", "-shm"} {
		if err := os.Remove(caminhos.Banco() + resto); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return originais, err
		}
	}
	if _, err := extrair(oBanco, caminhos.Banco()); err != nil {
		return originais, fmt.Errorf("restaurar o banco: %w", err)
	}
	return originais, nil
}
