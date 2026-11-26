package cli_test

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wkoch/Palantir/internal/cli"
	"github.com/wkoch/Palantir/internal/testes"
)

type instalacao struct {
	t        *testing.T
	dir      string
	endereco string
}

func ferramenta(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "palantir-tool")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/palantir-tool").CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	return bin
}

func nova(t *testing.T, extra string) instalacao {
	t.Helper()
	i := instalacao{t: t, dir: testes.Pasta(t), endereco: testes.PortaFechada(t)}
	cfg := "[servidor]\nendereco = \"" + i.endereco + "\"\n[segredos]\narmazenamento = \"arquivo\"\n" +
		"[acervo]\npasta_entrada = \"" + i.dir + "/Entrada\"\n" + extra
	os.MkdirAll(filepath.Join(i.dir, "config"), 0o700)
	if err := os.WriteFile(filepath.Join(i.dir, "config", "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return i
}

func rodar(ctx context.Context, entrada string, args ...string) (int, string, string) {
	var saida, erros bytes.Buffer
	codigo := cli.Executar(ctx, args, cli.Ambiente{Versao: "9.9.9", Entrada: strings.NewReader(entrada), Saida: &saida, Erros: &erros})
	return codigo, saida.String(), erros.String()
}

func (i instalacao) quer(codigo int, contem, entrada string, args ...string) string {
	i.t.Helper()
	c, s, e := rodar(context.Background(), entrada, append([]string{"--dir", i.dir}, args...)...)
	if c != codigo || !strings.Contains(s+e, contem) {
		i.t.Errorf("%v: código %d (quero %d)\nsaída: %s\nerros: %s", args, c, codigo, s, e)
	}
	return s
}

func TestComandosBasicos(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		args   []string
		codigo int
		contem string
	}{
		{[]string{"versao"}, 0, "9.9.9"},
		{[]string{"ajuda"}, 0, "diagnostico"},
		{[]string{"-h"}, 0, "uso:"},
		{[]string{"--nada"}, 2, "uso:"},
		{[]string{"voar"}, 2, "comando desconhecido"},
		{[]string{"--dir", "/proc/nao", "verificar"}, 1, "palantird:"},
	} {
		codigo, s, e := rodar(ctx, "", c.args...)
		if codigo != c.codigo || !strings.Contains(s+e, c.contem) {
			t.Error(c.args, codigo, s, e)
		}
	}
	ruim := nova(t, "[imap]\nlimite = 0\n")
	ruim.quer(1, "imap.limite", "", "verificar")
}

func TestSegredos(t *testing.T) {
	i := nova(t, "")
	i.quer(0, "não definido", "", "segredo", "listar")
	i.quer(0, "ok", "minha-chave-123\n", "segredo", "definir", "openrouter")
	s := i.quer(0, "openrouter   definido", "", "segredo", "listar")
	if strings.Contains(s, "minha-chave") {
		t.Error("o valor apareceu")
	}
	diario := i.quer(0, "segredo openrouter", "", "diario")
	if strings.Contains(diario, "minha-chave") {
		t.Error("o valor está no diário")
	}
	i.quer(0, "ok", "sem-quebra-de-linha", "segredo", "definir", "imap")
	i.quer(0, "ok", "", "segredo", "remover", "openrouter")
	i.quer(1, "valor vazio", "\n", "segredo", "definir", "openrouter")
	i.quer(2, "desconhecido", "x", "segredo", "definir", "outro")
	i.quer(2, "use:", "", "segredo")
	i.quer(2, "use:", "", "segredo", "ver", "openrouter")
}

func TestAcervoPelaLinhaDeComando(t *testing.T) {
	i := nova(t, "[ferramentas]\nisolamento = \"nenhum\"\nbinario = \""+ferramenta(t)+"\"\n")
	pasta := t.TempDir()
	os.WriteFile(filepath.Join(pasta, "a.txt"), []byte("documento um"), 0o600)
	i.quer(0, "1 importado(s)", "", "importar", pasta)
	i.quer(0, "1 já no acervo", "", "importar", filepath.Join(pasta, "a.txt"))
	i.quer(2, "ao menos um", "", "importar")
	i.quer(1, "no such file", "", "importar", "/nao/existe")
	os.WriteFile(filepath.Join(pasta, "vazio.txt"), nil, 0o600)
	i.quer(1, "1 arquivo(s) não importado(s)", "", "importar", pasta)

	i.quer(0, "1 original(is) íntegro(s), 0 com problema", "", "verificar")
	i.quer(0, "1 documento(s), 1 trecho(s), 0 com erro", "", "regenerar")
	destino := filepath.Join(t.TempDir(), "tudo.zip")
	i.quer(0, "Exportado para", "", "exportar", destino)
	z, err := zip.OpenReader(destino)
	if err != nil {
		t.Fatal(err)
	}
	z.Close()
	i.quer(1, "file exists", "", "exportar", destino)
	i.quer(2, "destino", "", "exportar")
	for _, c := range []string{"verificar", "regenerar", "diagnostico", "servir"} {
		i.quer(2, "argumentos a mais", "", c, "sobra")
	}
	i.quer(0, "importar", "", "diario", "-n", "200")
	i.quer(2, "flag", "", "diario", "--nada")

	s := i.quer(0, "ferramentas", "", "diagnostico")
	for _, quer := range []string{"SEM isolamento", "não definida", "não configurado", "não está respondendo", "1 íntegro(s)"} {
		if !strings.Contains(s, quer) {
			t.Errorf("falta %q em %s", quer, s)
		}
	}

	originais, _ := filepath.Glob(filepath.Join(i.dir, "dados", "originais", "*"))
	os.Chmod(originais[0], 0o600)
	os.WriteFile(originais[0], []byte("x"), 0o600)
	i.quer(1, "PROBLEMA a.txt", "", "verificar")
	i.quer(1, "falharam", "", "diagnostico")
	falha := filepath.Join(t.TempDir(), "f.zip")
	os.Remove(originais[0])
	i.quer(1, "exportar a.txt", "", "exportar", falha)
	if _, err := os.Stat(falha); err == nil {
		t.Error("um ZIP incompleto ficou no disco")
	}
	i.quer(1, "regeneração", "", "regenerar")
}

func TestDiagnosticoDosModelos(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	modelos := "[modelos]\nurl = \"" + s.D.Config().Modelos.URL + "\"\n"
	i := nova(t, modelos+"[ferramentas]\nisolamento = \"nenhum\"\nbinario = \""+ferramenta(t)+"\"\n")
	i.quer(1, "a chave do OpenRouter não foi configurada", "", "diagnostico", "--modelos")
	i.quer(0, "ok", "chave\n", "segredo", "definir", "openrouter")
	saida := i.quer(0, "modelo forte", "", "diagnostico", "--modelos")
	for _, quer := range []string{"ok    modelo barato", "ok    modelo principal", "ok    ferramentas do modelo", "ok    busca semântica"} {
		if !strings.Contains(saida, quer) {
			t.Errorf("falta %q em %s", quer, saida)
		}
	}
	i.quer(2, "argumentos a mais", "", "diagnostico", "--modelos", "x")
	if s := i.quer(0, "ferramentas", "", "diagnostico", "--instalacao"); strings.Contains(s, "daemon") || strings.Contains(s, "chave") {
		t.Error("a conferência da instalação olhou o que depende de uma sessão:", s)
	}
}

func TestDiagnosticoDeFerramentasQuebradas(t *testing.T) {
	i := nova(t, "[ferramentas]\nisolamento = \"nenhum\"\nbinario = \"/nao/existe\"\n")
	i.quer(1, "FALHA ferramentas", "", "diagnostico")
	eco := filepath.Join(t.TempDir(), "x")
	os.WriteFile(eco, []byte("#!/bin/sh\nexit 1\n"), 0o700)
	nova(t, "[ferramentas]\nisolamento = \"nenhum\"\nbinario = \""+eco+"\"\n").quer(1, "FALHA ferramentas", "", "diagnostico")
}

func TestServir(t *testing.T) {
	i := nova(t, "[ferramentas]\nisolamento = \"nenhum\"\nbinario = \""+ferramenta(t)+"\"\n")
	ctx, cancelar := context.WithCancel(context.Background())
	pronto := make(chan string, 1)
	fim := make(chan int, 1)
	var erros bytes.Buffer
	go func() {
		fim <- cli.Executar(ctx, []string{"--dir", i.dir}, cli.Ambiente{Versao: "1", Saida: &bytes.Buffer{}, Erros: &erros,
			Pronto: func(e string) { pronto <- e }})
	}()
	var endereco string
	select {
	case endereco = <-pronto:
	case c := <-fim:
		t.Fatal(c, erros.String())
	case <-time.After(20 * time.Second):
		t.Fatal("não ficou pronto")
	}
	if codigo, corpo := testes.Obter(t, "http://"+endereco+"/api/saude"); codigo != 200 {
		t.Error(codigo, corpo)
	}
	// Um segundo daemon sobre os mesmos dados é recusado.
	i.quer(1, "já existe um palantird", "", "servir")
	conferirAbrir(t, i, endereco)
	// Os outros comandos continuam funcionando com o daemon ligado.
	i.quer(0, "respondendo, versão 1", "", "diagnostico")
	arq := filepath.Join(t.TempDir(), "b.txt")
	os.WriteFile(arq, []byte("com o daemon ligado"), 0o600)
	s := i.quer(0, "1 importado(s)", "", "importar", arq)
	if strings.Contains(s, "Extraindo") {
		t.Error("com o daemon ligado a extração é dele")
	}
	cancelar()
	select {
	case c := <-fim:
		if c != 0 {
			t.Error(c, erros.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatal("não encerrou")
	}
}

func TestServirComPortaOcupada(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	_ = s
	i := nova(t, "")
	l := strings.TrimPrefix(s.Web.URL, "http://")
	os.WriteFile(filepath.Join(i.dir, "config", "config.toml"), []byte("[servidor]\nendereco = \""+l+"\"\n[segredos]\narmazenamento = \"arquivo\"\n"), 0o600)
	i.quer(1, "address already in use", "", "servir")
}

func TestRestaurar(t *testing.T) {
	origem := nova(t, "")
	arquivo := filepath.Join(t.TempDir(), "n.txt")
	os.WriteFile(arquivo, []byte("nota guardada"), 0o600)
	ferramentas := "[ferramentas]\nisolamento = \"nenhum\"\nbinario = \"" + ferramenta(t) + "\"\n"
	origem = nova(t, ferramentas)
	origem.quer(0, "1 importado(s)", "", "importar", arquivo)
	copia := filepath.Join(t.TempDir(), "c.zip")
	origem.quer(0, "Exportado", "", "exportar", copia)
	origem.quer(1, "já tem documentos", "", "restaurar", copia)
	destino := nova(t, "")
	destino.quer(0, "Restaurado, com 1 original(is)", "", "restaurar", copia)
	destino.quer(0, "1 original(is) íntegro(s)", "", "verificar")
	destino.quer(2, "informe o arquivo", "", "restaurar")
	nova(t, "").quer(1, "não pôde ser relida", "", "restaurar", arquivo)
}
