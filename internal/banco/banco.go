// Package banco abre o SQLite do Palantír e aplica as migrações.
package banco

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"syscall"

	_ "modernc.org/sqlite" // driver "sqlite", sem CGO
)

// ErrBancoMaisNovo indica um banco criado por uma versão posterior.
var ErrBancoMaisNovo = errors.New("o banco foi criado por uma versão mais nova do Palantír")

// Abrir abre o banco e o leva à última versão do esquema. Antes de migrar um
// banco existente, uma cópia recuperável é gravada ao lado dele.
func Abrir(ctx context.Context, caminho string) (*sql.DB, error) {
	return abrir(ctx, caminho, migracoes)
}

// travar impede que dois processos criem ou migrem o banco ao mesmo tempo: o
// daemon e um comando podem abri-lo no mesmo instante, como na instalação.
func travar(caminho string) (func(), error) {
	f, err := os.OpenFile(caminho+".trava", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("travar o banco: %w", err)
	}
	return func() { f.Close() }, nil
}

func abrir(ctx context.Context, caminho string, passos []string) (*sql.DB, error) {
	destravar, err := travar(caminho)
	if err != nil {
		return nil, err
	}
	defer destravar()
	_, errStat := os.Stat(caminho)
	existia := !errors.Is(errStat, fs.ErrNotExist)

	q := url.Values{}
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	dsn := (&url.URL{Scheme: "file", Path: caminho, RawQuery: q.Encode()}).String()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := migrar(ctx, db, caminho, existia, passos); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(caminho, 0o600); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrar(ctx context.Context, db *sql.DB, caminho string, existia bool, passos []string) error {
	var versao int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&versao); err != nil {
		return fmt.Errorf("ler a versão do banco: %w", err)
	}
	if versao > len(passos) {
		return fmt.Errorf("%w (banco %d, programa %d); restaure %s.v%d.bak ou atualize o programa",
			ErrBancoMaisNovo, versao, len(passos), caminho, len(passos))
	}
	if versao == len(passos) {
		return nil
	}
	if existia && versao > 0 {
		copia := fmt.Sprintf("%s.v%d.bak", caminho, versao)
		if err := os.Remove(copia); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if _, err := db.ExecContext(ctx, "VACUUM INTO ?", copia); err != nil {
			return fmt.Errorf("copiar o banco antes de migrar: %w", err)
		}
	}
	for i := versao; i < len(passos); i++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, passos[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migração %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Versao é a versão atual do esquema.
func Versao() int { return len(migracoes) }
