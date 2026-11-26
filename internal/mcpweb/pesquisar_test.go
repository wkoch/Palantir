package mcpweb_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wkoch/Palantir/internal/mcpweb"
	"github.com/wkoch/Palantir/internal/proxy"
	"github.com/wkoch/Palantir/internal/testes"
)

// socketLocal é um proxy de saída que só alcança uma porta desta máquina.
func socketLocal(t *testing.T, porta int) string {
	t.Helper()
	p := &proxy.Proxy{Ferramenta: "web", Fixas: func() []proxy.Regra {
		return []proxy.Regra{{Host: "127.0.0.1", Portas: []int{porta}, Local: true}}
	}}
	socket := filepath.Join(testes.Pasta(t), "s")
	if err := p.Escutar(socket); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Fechar() })
	return socket
}

func TestPesquisar(t *testing.T) {
	var resposta string
	var status int
	var pedido url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/base/search" {
			t.Error(r.URL.Path)
		}
		pedido = r.URL.Query()
		if status == http.StatusFound {
			w.Header().Set("Location", "http://atacante.example/")
		}
		w.WriteHeader(status)
		io.WriteString(w, resposta)
	}))
	defer srv.Close()
	porta := func() int {
		u, _ := url.Parse(srv.URL)
		var n int
		fmt.Sscan(u.Port(), &n)
		return n
	}()
	o := mcpweb.Opcoes{Busca: srv.URL + "/base/", Socket: socketLocal(t, porta)}
	ctx := context.Background()

	var itens []string
	for i := range 20 {
		itens = append(itens, fmt.Sprintf(`{"url":"https://exemplo.test/%d","title":" Título\n %d ","content":"%s"}`, i, i,
			strings.Repeat("palavra ", 100)))
	}
	itens = append([]string{`{"url":"javascript:alert(1)"}`, `{"url":"https://u:s@exemplo.test/"}`,
		`{"url":"https://exemplo.test/0","title":"t"}`}, itens...)
	status, resposta = 200, `{"results":[`+strings.Join(itens, ",")+`]}`
	r, err := mcpweb.Pesquisar(ctx, o, "  prazo & multa  ")
	if err != nil {
		t.Fatal(err)
	}
	if pedido.Get("q") != "prazo & multa" || pedido.Get("format") != "json" || pedido.Get("language") != "pt-BR" {
		t.Error(pedido)
	}
	if len(r.Itens) != 8 || r.Itens[0].Titulo != "t" || r.Itens[1].Titulo != "Título 1" || len([]rune(r.Itens[1].Trecho)) != 400 {
		t.Errorf("%d %+v", len(r.Itens), r.Itens[:2])
	}
	for _, ruim := range []struct {
		status int
		corpo  string
		quer   string
	}{{500, "", "HTTP 500"}, {302, "", "HTTP 302"}, {200, "<html>", "inesperada"}} {
		status, resposta = ruim.status, ruim.corpo
		if _, err := mcpweb.Pesquisar(ctx, o, "x"); err == nil || !strings.Contains(err.Error(), ruim.quer) {
			t.Error(ruim.quer, err)
		}
	}
	for _, c := range []struct {
		o        mcpweb.Opcoes
		consulta string
	}{{mcpweb.Opcoes{}, "x"}, {o, " "}, {mcpweb.Opcoes{Busca: "ftp://x"}, "x"},
		{mcpweb.Opcoes{Busca: "http://exemplo.test", Socket: "/nao/existe"}, "x"}} {
		if _, err := mcpweb.Pesquisar(ctx, c.o, c.consulta); err == nil {
			t.Error(c)
		}
	}
}
