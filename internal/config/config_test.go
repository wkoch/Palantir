package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaminhos(t *testing.T) {
	t.Setenv("HOME", "/casa")
	t.Setenv("XDG_CONFIG_HOME", "/x/config")
	t.Setenv("XDG_DATA_HOME", "relativo-ignorado")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/7")
	c, err := CaminhosPadrao("")
	if err != nil {
		t.Fatal(err)
	}
	quer := Caminhos{Config: "/x/config/palantir", Dados: "/casa/.local/share/palantir", Execucao: "/run/user/7/palantir"}
	if c != quer {
		t.Errorf("%+v", c)
	}
	for teve, fim := range map[string]string{c.Arquivo(): "config.toml", c.Ajustes(): "ajustes.toml",
		c.Segredos(): "segredos.json", c.Banco(): "palantir.db", c.Originais(): "originais", c.Diario(): "diario.jsonl"} {
		if filepath.Base(teve) != fim {
			t.Errorf("%s", teve)
		}
	}
	curto, err := CaminhosPadrao("/tmp/p")
	if err != nil || curto.Execucao != "/tmp/p/run" {
		t.Fatal(curto, err)
	}
	// Uma raiz comprida demais para um socket usa o diretório de execução.
	longa := "/tmp/" + strings.Repeat("x", 120)
	l, err := CaminhosPadrao(longa)
	if err != nil || !strings.HasPrefix(l.Execucao, "/run/user/7/palantir-") {
		t.Fatal(l.Execucao, err)
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	if l, _ = CaminhosPadrao(longa); !strings.HasPrefix(l.Execucao, os.TempDir()) {
		t.Error(l.Execucao)
	}
	d := t.TempDir()
	c, _ = CaminhosPadrao(d)
	if err := c.Criar(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(c.Originais())
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatal(info, err)
	}
	ruim := Caminhos{Config: "/proc/proibido"}
	if ruim.Criar() == nil {
		t.Error("deveria falhar")
	}
}

func preparar(t *testing.T, conteudo string) Caminhos {
	t.Helper()
	c, err := CaminhosPadrao(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if conteudo != "" {
		if err := c.Criar(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(c.Arquivo(), []byte(conteudo), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func TestCarregarCriaOPadrao(t *testing.T) {
	c := preparar(t, "")
	cfg, err := Carregar(c)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Servidor.Endereco != EnderecoDoUsuario(os.Getuid()) || cfg.Ferramentas.Isolamento != "podman" ||
		!cfg.IMAP.Etiquetar || cfg.Orcamento.MensalUSD != 20 || cfg.Modelos.Barato == "" {
		t.Errorf("%+v", cfg)
	}
	b, err := os.ReadFile(c.Arquivo())
	if err != nil || string(b) != string(Padrao()) {
		t.Fatal("o arquivo inicial deveria ser o padrão", err)
	}
	// O padrão devolvido é uma cópia.
	Padrao()[0] = 'x'
	if Padrao()[0] != '#' {
		t.Error("padrão alterado")
	}
}

func TestValidacao(t *testing.T) {
	casos := map[string]string{
		`[servidor]` + "\n" + `endereco="0.0.0.0:7700"`:                 "não é um endereço local",
		`[servidor]` + "\n" + `endereco="sem-porta"`:                    "servidor.endereco",
		`[orcamento]` + "\n" + `mensal_usd=-1.0`:                        "negativos",
		`[ferramentas]` + "\n" + `isolamento="chroot"`:                  "isolamento",
		`[ferramentas.web]` + "\n" + `hosts=["http://x"]`:               "nome de host",
		`[imap]` + "\n" + `servidor="a b"`:                              "imap.servidor",
		`[imap]` + "\n" + `servidor="a.com"` + "\nporta=70000":          "imap.porta",
		`[imap]` + "\n" + `servidor="a.com"` + "\nseguranca=\"nenhum\"": "sem TLS",
		`[imap]` + "\n" + `seguranca="ssl"`:                             "imap.seguranca",
		`[imap]` + "\n" + `limite=0`:                                    "imap.limite",
		`[segredos]` + "\n" + `armazenamento="nuvem"`:                   "armazenamento",
		`[acervo]` + "\n" + `intervalo_segundos=0`:                      "intervalo",
		`[modelos]` + "\n" + `barata="x"`:                               "chave desconhecida",
		`isto não é toml`:                                               "ler",
	}
	for conteudo, quer := range casos {
		_, err := Carregar(preparar(t, conteudo))
		if err == nil || !strings.Contains(err.Error(), quer) {
			t.Errorf("%q: erro = %v, quero %q", conteudo, err, quer)
		}
	}
	if _, err := Carregar(preparar(t, "[imap]\nservidor=\"localhost\"\nseguranca=\"nenhum\"")); err != nil {
		t.Error(err)
	}
}

func TestHostsEEnderecos(t *testing.T) {
	for h, quer := range map[string]bool{"a.com": true, "x-y_z.org": true, "10.0.0.1": true, "::1": true, "": false,
		"-a.com": false, "a-.com": false, "a..com": false, "a/b": false, "a.com:80": false,
		strings.Repeat("a", 64) + ".com": false, strings.Repeat("a.", 130): false} {
		if HostValido(h) != quer {
			t.Errorf("HostValido(%q)", h)
		}
	}
	for h, quer := range map[string]bool{"localhost": true, "127.0.0.5": true, "::1": true, "192.168.0.1": false, "x.com": false} {
		if EnderecoLocal(h) != quer {
			t.Errorf("EnderecoLocal(%q)", h)
		}
	}
	t.Setenv("HOME", "/casa")
	for entrada, quer := range map[string]string{"~": "/casa", "~/Docs": "/casa/Docs", "/abs": "/abs", "~outro": "~outro"} {
		if teve := Expandir(entrada); teve != quer {
			t.Errorf("Expandir(%q) = %q", entrada, teve)
		}
	}
}

func TestGravarAjustes(t *testing.T) {
	c := preparar(t, "# meu comentário\n[orcamento]\nmensal_usd = 5.0\n")
	imap := IMAP{Servidor: "imap.exemplo.org", Porta: 993, Usuario: "eu", Caixa: "INBOX", Limite: 10, Seguranca: "tls"}
	novo, err := GravarAjustes(c, Ajustes{IMAP: &imap})
	if err != nil || novo.IMAP != imap || novo.Orcamento.MensalUSD != 5 {
		t.Fatal(novo, err)
	}
	hosts := []string{"a.org"}
	if _, err = GravarAjustes(c, Ajustes{Orcamento: &Orcamento{MensalUSD: 9, PorAtividadeUSD: 2}, WebHosts: &hosts}); err != nil {
		t.Fatal(err)
	}
	// Ajustes anteriores continuam e os novos valem na próxima leitura.
	lido, err := Carregar(c)
	if err != nil || lido.IMAP != imap || lido.Orcamento.MensalUSD != 9 || lido.Ferramentas.Web.Hosts[0] != "a.org" ||
		lido.Ferramentas.Isolamento != "podman" {
		t.Fatal(lido, err)
	}
	original, _ := os.ReadFile(c.Arquivo())
	if !strings.HasPrefix(string(original), "# meu comentário") {
		t.Error("o config.toml do usuário não pode mudar")
	}
	antes, _ := os.ReadFile(c.Ajustes())
	ruim := []string{"http://x"}
	if _, err := GravarAjustes(c, Ajustes{WebHosts: &ruim}); err == nil {
		t.Fatal("deveria recusar")
	}
	depois, _ := os.ReadFile(c.Ajustes())
	if string(antes) != string(depois) {
		t.Error("um ajuste inválido foi gravado")
	}
	if _, err := GravarAjustes(preparar(t, "lixo ="), Ajustes{}); err == nil {
		t.Error("configuração ilegível deveria falhar")
	}
}
