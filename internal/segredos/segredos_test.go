package segredos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func exercitar(t *testing.T, c Cofre) {
	t.Helper()
	ctx := context.Background()
	ler := func(quer string) {
		t.Helper()
		if v, err := c.Ler(ctx, OpenRouter); err != nil || v != quer {
			t.Fatalf("Ler = %q, %v; quero %q", v, err, quer)
		}
	}
	ler("")
	if err := c.Definir(ctx, OpenRouter, "abc"); err != nil {
		t.Fatal(err)
	}
	ler("abc")
	if err := c.Definir(ctx, SenhaIMAP, "outra"); err != nil {
		t.Fatal(err)
	}
	ler("abc")
	if err := c.Remover(ctx, OpenRouter); err != nil {
		t.Fatal(err)
	}
	ler("")
	if err := c.Definir(ctx, "qualquer", "x"); !errors.Is(err, ErrDesconhecido) {
		t.Error(err)
	}
	if err := c.Remover(ctx, "qualquer"); !errors.Is(err, ErrDesconhecido) {
		t.Error(err)
	}
}

func TestArquivo(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "s.json")
	c := Novo("arquivo", caminho)
	exercitar(t, c)
	info, err := os.Stat(caminho)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal(info, err)
	}
	if err := os.WriteFile(caminho, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := c.Ler(ctx, OpenRouter); err == nil {
		t.Error("arquivo corrompido deveria falhar")
	}
	if c.Definir(ctx, OpenRouter, "x") == nil || c.Remover(ctx, OpenRouter) == nil {
		t.Error("não pode sobrescrever um arquivo que não entende")
	}
	ilegivel := &Arquivo{Caminho: t.TempDir()}
	if _, err := ilegivel.Ler(ctx, OpenRouter); err == nil {
		t.Error("um diretório não é um arquivo de segredos")
	}
	sem := &Arquivo{Caminho: "/proc/nao/existe"}
	if sem.Definir(ctx, OpenRouter, "x") == nil {
		t.Error("deveria falhar ao gravar")
	}
}

// secretToolFalso imita o secret-tool guardando cada nome num arquivo.
const secretToolFalso = `#!/bin/sh
dir=$(dirname "$0")
comando=$1
shift
[ "$comando" = store ] && shift
nome=$4
case $comando in
store) cat > "$dir/$nome" ;;
lookup) [ -f "$dir/$nome" ] || exit 1; cat "$dir/$nome" ;;
clear) rm -f "$dir/$nome" ;;
esac
`

func TestSecretService(t *testing.T) {
	dir := t.TempDir()
	programa := filepath.Join(dir, "secret-tool")
	if err := os.WriteFile(programa, []byte(secretToolFalso), 0o700); err != nil {
		t.Fatal(err)
	}
	exercitar(t, &SecretService{Programa: programa})

	if c, ok := Novo("secret-service", "").(*SecretService); !ok || c.Programa != "secret-tool" {
		t.Error("tipo errado")
	}
	quebrado := filepath.Join(dir, "quebrado")
	if err := os.WriteFile(quebrado, []byte("#!/bin/sh\necho 'keyring travado' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	q := &SecretService{Programa: quebrado}
	ctx := context.Background()
	if _, err := q.Ler(ctx, OpenRouter); err == nil {
		t.Error("erro do keyring precisa aparecer")
	}
	if q.Definir(ctx, OpenRouter, "x") == nil || q.Remover(ctx, OpenRouter) == nil {
		t.Error("erro do keyring precisa aparecer")
	}
}
