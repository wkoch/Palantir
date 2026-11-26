package copia

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wkoch/Palantir/internal/config"
)

// pacote monta um ZIP com os nomes e conteúdos dados.
func pacote(arquivos ...string) []byte {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for i := 0; i+1 < len(arquivos); i += 2 {
		w, _ := z.CreateHeader(&zip.FileHeader{Name: arquivos[i], Method: zip.Store})
		io.WriteString(w, arquivos[i+1])
	}
	z.Close()
	return b.Bytes()
}

func exportar(b []byte, falha error) func(context.Context, io.Writer) error {
	return func(_ context.Context, w io.Writer) error {
		w.Write(b)
		return falha
	}
}

func TestGravarConfereAntesDeDarONome(t *testing.T) {
	ctx := context.Background()
	destino := t.TempDir()
	quando := time.Date(2026, 11, 26, 10, 30, 0, 0, time.UTC)
	bom := pacote(Banco, "banco", "originais/x", "conteudo")
	caminho, err := Gravar(ctx, destino, 10, quando, exportar(bom, nil))
	if err != nil || caminho != filepath.Join(destino, Pasta, "palantir-2026-11-26-103000.zip") {
		t.Fatal(caminho, err)
	}
	// Uma cópia existente nunca é substituída.
	if _, err := Gravar(ctx, destino, 10, quando, exportar(bom, nil)); err == nil || !strings.Contains(err.Error(), "já existe") {
		t.Error(err)
	}
	corrompido := bytes.Replace(pacote(Banco, "banco", "originais/x", "conteudo"), []byte("conteudo"), []byte("trocado!"), 1)
	for i, c := range []struct {
		corpo []byte
		falha error
		quer  string
	}{
		{bom, errors.New("disco retirado"), "disco retirado"},
		{[]byte("lixo"), nil, "não pôde ser relida"},
		{corrompido, nil, "corrompida"},
		{pacote("outra", "coisa"), nil, "falta o banco"},
	} {
		_, err := Gravar(ctx, destino, 10, quando.Add(time.Duration(i+1)*time.Second), exportar(c.corpo, c.falha))
		if err == nil || !strings.Contains(err.Error(), c.quer) {
			t.Error(c.quer, err)
		}
	}
	sobras, _ := os.ReadDir(filepath.Join(destino, Pasta))
	if len(sobras) != 1 {
		t.Error("ficaram arquivos incompletos", sobras)
	}
	cancelado, cancelar := context.WithCancel(ctx)
	cancelar()
	if err := Conferir(cancelado, caminho); !errors.Is(err, context.Canceled) {
		t.Error(err)
	}
}

func TestGravarSemEspacoESemDestino(t *testing.T) {
	ctx := context.Background()
	if _, err := Gravar(ctx, "/nao/existe", 1, time.Now(), nil); err == nil {
		t.Error("destino inexistente")
	}
	if _, err := Gravar(ctx, t.TempDir(), 1<<62, time.Now(), nil); !errors.Is(err, ErrSemEspaco) {
		t.Error(err)
	}
	arquivo := filepath.Join(t.TempDir(), "f")
	os.WriteFile(arquivo, nil, 0o600)
	if _, err := Gravar(ctx, arquivo, 0, time.Now(), nil); err == nil {
		t.Error("destino que é um arquivo")
	}
}

func TestRestaurarSoNomesConhecidos(t *testing.T) {
	ctx := context.Background()
	gravar := func(b []byte) string {
		p := filepath.Join(t.TempDir(), "c.zip")
		os.WriteFile(p, b, 0o600)
		return p
	}
	nova := func() config.Caminhos {
		c, err := config.CaminhosPadrao(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	soma := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" // "hello"
	c := nova()
	n, err := Restaurar(ctx, c, gravar(pacote(Banco, "b", "originais/"+soma+"/../../../fora.txt", "hello",
		"../escapou", "x", "config/../acesso", "x", "segredos.json", "x")))
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	var achados []string
	filepath.Walk(filepath.Dir(c.Config), func(p string, i os.FileInfo, _ error) error {
		if !i.IsDir() {
			achados = append(achados, i.Name())
		}
		return nil
	})
	if strings.Join(achados, " ") != "palantir.db "+soma && strings.Join(achados, " ") != soma+" palantir.db" {
		t.Error(achados)
	}
	for nome, b := range map[string][]byte{
		"soma errada":   pacote(Banco, "b", "originais/"+soma+"/a", "outra coisa"),
		"nome estranho": pacote(Banco, "b", "originais/../x", "x"),
		"sem banco":     pacote("a", "b"),
	} {
		d := nova()
		if _, err := Restaurar(ctx, d, gravar(b)); err == nil {
			t.Error(nome)
		}
		if _, err := os.Stat(d.Banco()); err == nil {
			t.Error(nome, "o banco não podia ter entrado")
		}
	}
	cancelado, cancelar := context.WithCancel(ctx)
	cancelar()
	if _, err := Restaurar(cancelado, nova(), gravar(pacote(Banco, "b"))); err == nil {
		t.Error("cancelado")
	}
}
