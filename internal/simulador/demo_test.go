package simulador_test

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/segredos"
	"github.com/wkoch/Palantir/internal/simulador"
	"github.com/wkoch/Palantir/internal/testes"
)

func args(dir string, extra ...string) []string {
	return append([]string{"--dir", dir, "--modelo", "127.0.0.1:0", "--imap", "127.0.0.1:0", "--web", "127.0.0.1:0"}, extra...)
}

func TestDemonstracao(t *testing.T) {
	dir := testes.Pasta(t)
	var saida bytes.Buffer
	ctx, cancelar := context.WithCancel(context.Background())
	pronto := make(chan struct{})
	fim := make(chan error, 1)
	go func() {
		fim <- simulador.Executar(ctx, args(dir, "--ferramentas", "./bin/palantir-tool"), &saida, func() { close(pronto) })
	}()
	select {
	case <-pronto:
	case err := <-fim:
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("não ficou pronto")
	}
	c, err := config.CaminhosPadrao(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Carregar(c)
	if err != nil || cfg.IMAP.Usuario != simulador.Usuario || !strings.HasPrefix(cfg.Modelos.URL, "http://127.0.0.1:") ||
		!filepath.IsAbs(cfg.Ferramentas.Binario) {
		t.Fatal(cfg, err)
	}
	if v, _ := segredos.Novo("arquivo", c.Segredos()).Ler(ctx, segredos.SenhaIMAP); v != simulador.Senha {
		t.Error(v)
	}
	if _, err := os.Stat(filepath.Join(dir, "exemplos", "edital-exemplo.txt")); err != nil {
		t.Error(err)
	}
	web := regexp.MustCompile(`Página de exemplo:\s+(\S+)/pagina`).FindStringSubmatch(saida.String())[1]
	if codigo, corpo := testes.Obter(t, web+"/pagina"); codigo != 200 || !strings.Contains(corpo, "dez dias") {
		t.Error(codigo)
	}
	if codigo, _ := testes.Obter(t, web+"/outra"); codigo != 404 {
		t.Error(codigo)
	}
	res, err := http.PostForm(web+"/entregar", url.Values{"de": {"a@b.test\r\nBcc: x@y.test"}, "assunto": {"oi"}})
	if err != nil || res.StatusCode != http.StatusNoContent {
		t.Fatal(res, err)
	}
	res.Body.Close()
	cancelar()
	if err := <-fim; err != nil {
		t.Fatal(err)
	}
	// A mesma pasta pode ser usada de novo.
	ctx2, cancelar2 := context.WithCancel(context.Background())
	if err := simulador.Executar(ctx2, args(dir), &saida, cancelar2); err != nil {
		t.Fatal(err)
	}
}

func TestDemonstracaoRecusas(t *testing.T) {
	ctx := context.Background()
	var saida bytes.Buffer
	if err := simulador.Executar(ctx, nil, &saida, nil); err == nil {
		t.Error("sem --dir")
	}
	if err := simulador.Executar(ctx, []string{"--nada"}, &saida, nil); err == nil {
		t.Error("opção desconhecida")
	}
	// Uma instalação com dados reais não é sobrescrita.
	real := testes.Pasta(t)
	c, _ := config.CaminhosPadrao(real)
	if _, err := config.Carregar(c); err != nil {
		t.Fatal(err)
	}
	antes, _ := os.ReadFile(c.Arquivo())
	if err := simulador.Executar(ctx, args(real), &saida, nil); err == nil || !strings.Contains(err.Error(), "não é de demonstração") {
		t.Error(err)
	}
	if depois, _ := os.ReadFile(c.Arquivo()); !bytes.Equal(antes, depois) {
		t.Error("a configuração real mudou")
	}
	ocupada := testes.PortaFechada(t)
	for _, extra := range [][]string{{"--imap", "256.0.0.1:1"}, {"--modelo", "256.0.0.1:1"}, {"--web", "256.0.0.1:1"},
		{"--endereco", "0.0.0.0:80"}, {"--isolamento", "outro"}} {
		if err := simulador.Executar(ctx, append(args(testes.Pasta(t)), extra...), &saida, nil); err == nil {
			t.Error(extra, ocupada)
		}
	}
	if err := simulador.Executar(ctx, args("/proc/nao"), &saida, nil); err == nil {
		t.Error("pasta impossível")
	}
}

func TestModeloSimulado(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	url := s.D.Config().Modelos.URL
	for _, c := range []struct {
		chave, rota, corpo string
		quer               int
	}{
		{"", "/chat/completions", "{}", 401},
		{"k", "/chat/completions", "x", 400},
		{"k", "/embeddings", "x", 400},
		{"k", "/nada", "{}", 404},
		{"k", "/chat/completions", "{}", 200},
	} {
		req, _ := http.NewRequest("POST", url+c.rota, strings.NewReader(c.corpo))
		req.Header.Set("Authorization", "Bearer "+c.chave)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != c.quer {
			t.Error(c, res.StatusCode)
		}
	}
	if v := simulador.Vetor(""); v[0] != 1 {
		t.Error("vetor vazio")
	}
	// Respostas do modelo padrão em casos de borda.
	p := func(esquema, usuario string) simulador.Resposta {
		var x simulador.PedidoRecebido
		x.Mensagem = []simulador.MensagemRecebida{{Papel: "user", Conteudo: usuario}}
		if esquema != "" {
			x.Formato = &struct {
				Esquema struct {
					Nome string `json:"name"`
				} `json:"json_schema"`
			}{}
			x.Formato.Esquema.Nome = esquema
		}
		return simulador.Padrao(x)
	}
	for _, e := range []string{"enriquecimento", "arvore_assuntos", "classificacao_email", "juiz", "material_estudo", "secao_relatorio"} {
		if p(e, "nada").JSON == nil {
			t.Error(e)
		}
	}
	if r := p("", "<dados_externos origem=\"x\" fonte=\"y\">\nsem marcas\n</dados_externos>"); !strings.Contains(r.Texto, "Encontrei") {
		t.Error(r.Texto)
	}
	if r := p("", "a chamada foi bloqueada <dados_externos"); !strings.Contains(r.Texto, "bloqueada") {
		t.Error(r.Texto)
	}
	if (simulador.PedidoRecebido{}).Ultima().Papel != "" {
		t.Error("sem mensagens")
	}
}
