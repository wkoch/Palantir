package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/wkoch/Palantir/internal/config"
)

func TestEnderecoPorUsuario(t *testing.T) {
	for uid, quer := range map[int]string{1000: "127.0.0.1:7700", 1001: "127.0.0.1:7701", 0: "127.0.0.1:7700",
		999: "127.0.0.1:8699", 2000: "127.0.0.1:7700"} {
		if got := config.EnderecoDoUsuario(uid); got != quer {
			t.Error(uid, got)
		}
	}
	c, err := config.CaminhosPadrao(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Carregar(c)
	if err != nil || cfg.Servidor.Endereco != config.EnderecoDoUsuario(os.Getuid()) || cfg.Segredos.Armazenamento != "secret-service" {
		t.Error(cfg.Servidor, cfg.Segredos, err)
	}
}

func TestChaveDeAcesso(t *testing.T) {
	c, err := config.CaminhosPadrao(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	primeira, err := c.LerChaveDeAcesso()
	if err != nil || len(primeira) < 40 {
		t.Fatal(primeira, err)
	}
	info, err := os.Stat(c.ChaveDeAcesso())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal(info.Mode(), err)
	}
	if segunda, err := c.LerChaveDeAcesso(); err != nil || segunda != primeira {
		t.Error("a chave mudou", err)
	}
	if err := os.WriteFile(c.ChaveDeAcesso(), []byte("curta\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.LerChaveDeAcesso(); err == nil || !strings.Contains(err.Error(), "curto") {
		t.Error(err)
	}
	os.Remove(c.ChaveDeAcesso())
	os.Mkdir(c.ChaveDeAcesso(), 0o700)
	if _, err := c.LerChaveDeAcesso(); err == nil {
		t.Error("uma pasta no lugar da chave")
	}
	ruim := config.Caminhos{Config: "/proc/nao"}
	if _, err := ruim.LerChaveDeAcesso(); err == nil {
		t.Error("pasta impossível")
	}
}
