package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// conferirAbrir é chamada com o daemon ligado.
func conferirAbrir(t *testing.T, i instalacao, endereco string) {
	t.Helper()
	chave, err := os.ReadFile(filepath.Join(i.dir, "config", "acesso"))
	if err != nil {
		t.Fatal(err)
	}
	segredo := strings.TrimSpace(string(chave))
	arquivo := strings.TrimSpace(i.quer(0, "entrar.html", "", "abrir", "--imprimir"))
	info, err := os.Stat(arquivo)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal(info, err)
	}
	pagina, err := os.ReadFile(arquivo)
	if err != nil || !strings.Contains(string(pagina), "http://"+endereco+"/#entrar="+segredo) {
		t.Fatal(string(pagina), err)
	}

	// O navegador recebe só o caminho do arquivo: a chave não passa pela linha
	// de comando, que os outros usuários enxergam.
	registro := filepath.Join(t.TempDir(), "argumentos")
	navegador := filepath.Join(t.TempDir(), "navegador")
	if err := os.WriteFile(navegador, []byte("#!/bin/sh\nprintf '%s' \"$*\" > "+registro+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	saida := i.quer(0, "", "", "abrir", "--navegador", navegador)
	var args []byte
	for prazo := time.Now().Add(5 * time.Second); len(args) == 0 && time.Now().Before(prazo); time.Sleep(20 * time.Millisecond) {
		args, _ = os.ReadFile(registro)
	}
	if string(args) != "file://"+arquivo || strings.Contains(saida, segredo) {
		t.Errorf("argumentos %q, saída %q", args, saida)
	}
	i.quer(1, "abrir o navegador", "", "abrir", "--navegador", "/nao/existe")
	t.Setenv("PALANTIR_NAVEGADOR", navegador)
	i.quer(0, "", "", "abrir")
	i.quer(2, "argumentos a mais", "", "abrir", "x")
	i.quer(2, "flag", "", "abrir", "--nada")
}

func TestAbrirSemDaemon(t *testing.T) {
	i := nova(t, "")
	i.quer(1, "o daemon não está respondendo", "", "abrir", "--espera", "300ms")
}
