package diario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiario(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "d.jsonl")
	d, err := Abrir(caminho)
	if err != nil {
		t.Fatal(err)
	}
	d.Ocultar("chave", "segredo-especial")
	d.Ocultar("senha", "temporaria")
	d.Ocultar("senha", "")
	for i := 1; i <= 5; i++ {
		if err := d.Registrar(Entrada{Atividade: int64(i % 2), Tipo: "t", Nome: strings.Repeat("n", i),
			Motivo: "com segredo-especial e temporaria", Detalhe: "sk-abcdefghijklmnop123 Bearer abcdefghijklmnop CPF 123.456.789-09"}); err != nil {
			t.Fatal(err)
		}
	}
	bruto, _ := os.ReadFile(caminho)
	for _, proibido := range []string{"segredo-especial", "sk-abcdefgh", "abcdefghijklmnop", "123.456.789-09"} {
		if strings.Contains(string(bruto), proibido) {
			t.Errorf("%q apareceu no diário", proibido)
		}
	}
	if !strings.Contains(string(bruto), "temporaria") {
		t.Error("um segredo removido não precisa ser ocultado")
	}
	if info, _ := os.Stat(caminho); info.Mode().Perm() != 0o600 {
		t.Error(info.Mode())
	}
	ultimas, err := d.Ultimas(2, 0)
	if err != nil || len(ultimas) != 2 || ultimas[0].Nome != "nnnnn" || ultimas[1].Nome != "nnnn" || ultimas[0].Quando.IsZero() {
		t.Fatal(ultimas, err)
	}
	impares, _ := d.Ultimas(10, 1)
	if len(impares) != 3 {
		t.Error(impares)
	}
	// Linhas estragadas são puladas.
	f, _ := os.OpenFile(caminho, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString("não é json\n")
	f.Close()
	if todas, _ := d.Ultimas(100, 0); len(todas) != 5 {
		t.Error(len(todas))
	}
	if err := d.Registrar(Entrada{Nome: "longo", Detalhe: strings.Repeat("á", 5000)}); err != nil {
		t.Fatal(err)
	}
	if u, _ := d.Ultimas(1, 0); len([]rune(u[0].Detalhe)) != maxDetalhe+1 {
		t.Error("detalhe não foi cortado")
	}
	if v, _ := d.Ultimas(0, 0); v != nil {
		t.Error(v)
	}
	if err := os.Remove(caminho); err != nil {
		t.Fatal(err)
	}
	if v, err := d.Ultimas(3, 0); err != nil || v != nil {
		t.Error(v, err)
	}
}

func TestDiarioNuloEErros(t *testing.T) {
	var nulo *Diario
	nulo.Ocultar("a", "b")
	if nulo.Registrar(Entrada{}) != nil || nulo.Limpar("texto") != "texto" {
		t.Error("o diário nulo descarta")
	}
	if v, err := nulo.Ultimas(3, 0); v != nil || err != nil {
		t.Error(v, err)
	}
	if _, err := Abrir("/proc/nao/existe"); err == nil {
		t.Error("deveria falhar")
	}
	dir := t.TempDir()
	d, err := Abrir(filepath.Join(dir, "d"))
	if err != nil {
		t.Fatal(err)
	}
	d.caminho = dir
	if d.Registrar(Entrada{}) == nil {
		t.Error("gravar num diretório deveria falhar")
	}
	if _, err := d.Ultimas(1, 0); err == nil {
		t.Error("ler um diretório deveria falhar")
	}
}
