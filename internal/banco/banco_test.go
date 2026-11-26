package banco

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMigracoes(t *testing.T) {
	ctx := context.Background()
	caminho := filepath.Join(t.TempDir(), "x.db")
	db, err := abrir(ctx, caminho, []string{"CREATE TABLE a(x)"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO a VALUES (7)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if info, _ := os.Stat(caminho); info.Mode().Perm() != 0o600 {
		t.Error(info.Mode())
	}
	if _, err := os.Stat(caminho + ".v1.bak"); err == nil {
		t.Error("um banco novo não precisa de cópia")
	}

	// Uma migração que falha não deixa o banco pela metade.
	if _, err := abrir(ctx, caminho, []string{"CREATE TABLE a(x)", "CREATE TABLE b(y); ERRO"}); err == nil {
		t.Fatal("deveria falhar")
	}
	db, err = abrir(ctx, caminho, []string{"CREATE TABLE a(x)", "CREATE TABLE b(y)"})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	// A cópia anterior à migração é recuperável.
	copia, err := abrir(ctx, caminho+".v1.bak", []string{"CREATE TABLE a(x)"})
	if err != nil {
		t.Fatal(err)
	}
	var x int
	if err := copia.QueryRow("SELECT x FROM a").Scan(&x); err != nil || x != 7 {
		t.Fatal(x, err)
	}
	copia.Close()

	// Um programa antigo não abre um banco mais novo.
	if _, err := abrir(ctx, caminho, []string{"CREATE TABLE a(x)"}); !errors.Is(err, ErrBancoMaisNovo) {
		t.Fatal(err)
	}
	// Reabrir sem mudanças funciona.
	db, err = abrir(ctx, caminho, []string{"CREATE TABLE a(x)", "CREATE TABLE b(y)"})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
}

func TestEsquemaAtualEErros(t *testing.T) {
	ctx := context.Background()
	db, err := Abrir(ctx, filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil || v != Versao() {
		t.Fatal(v, err)
	}
	if _, err := db.Exec("INSERT INTO eventos (atividade_id, quando, origem, tipo, texto) VALUES (99,0,'','','')"); err == nil {
		t.Error("chaves estrangeiras deveriam estar ligadas")
	}
	if _, err := Abrir(ctx, "/proc/nao/existe.db"); err == nil {
		t.Error("deveria falhar")
	}
	lixo := filepath.Join(t.TempDir(), "lixo.db")
	if err := os.WriteFile(lixo, []byte("isto não é um banco, e é comprido o bastante para ser lido"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Abrir(ctx, lixo); err == nil {
		t.Error("deveria recusar um arquivo qualquer")
	}
}
