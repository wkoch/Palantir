package mcpweb_test

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wkoch/Palantir/internal/extrator"
	"github.com/wkoch/Palantir/internal/ferramentas"
	"github.com/wkoch/Palantir/internal/mcpweb"
)

// montar liga a ferramenta a um proxy que libera o servidor de teste.
func montar(t *testing.T, srv *httptest.Server) mcpweb.Opcoes {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	porta, _ := strconv.Atoi(u.Port())
	socket := socketLocal(t, porta)
	pdftotext := filepath.Join(t.TempDir(), "pdftotext")
	if err := os.WriteFile(pdftotext, []byte("#!/bin/sh\nprintf 'texto da primeira pagina do edital\\fsegunda pagina com mais texto\\f'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	e := extrator.Padrao()
	e.PDFToText = pdftotext
	return mcpweb.Opcoes{Socket: socket, Extrator: e, TLS: &tls.Config{InsecureSkipVerify: true}}
}

func paginas(w http.ResponseWriter, r *http.Request) {
	tipo := func(t, corpo string) {
		w.Header().Set("Content-Type", t)
		w.Write([]byte(corpo))
	}
	switch r.URL.Path {
	case "/html":
		tipo("text/html; charset=utf-8", "<title> Um   título </title><p>Corpo</p>")
	case "/latin1":
		tipo("text/html; charset=iso-8859-1", "<p>licita\xe7\xe3o</p>")
	case "/texto":
		tipo("text/plain", "texto puro")
	case "/json":
		tipo("application/json", `{"a":1}`)
	case "/pdf":
		tipo("application/pdf", "%PDF-1.4")
	case "/binario":
		tipo("application/zip", "PK")
	case "/grande":
		tipo("text/plain", strings.Repeat("á", 50000))
	case "/sumiu":
		http.NotFound(w, r)
	case "/volta":
		http.Redirect(w, r, "/texto", http.StatusFound)
	case "/laco":
		http.Redirect(w, r, "/laco", http.StatusFound)
	case "/fora":
		http.Redirect(w, r, "http://atacante.invalid/x", http.StatusFound)
	case "/arquivo":
		w.Header().Set("Location", "file:///etc/passwd")
		w.WriteHeader(http.StatusFound)
	}
}

func TestBuscar(t *testing.T) {
	for _, tlsLigado := range []bool{false, true} {
		var srv *httptest.Server
		if tlsLigado {
			srv = httptest.NewTLSServer(http.HandlerFunc(paginas))
		} else {
			srv = httptest.NewServer(http.HandlerFunc(paginas))
		}
		defer srv.Close()
		o := montar(t, srv)
		ctx := context.Background()
		buscar := func(caminho string) (mcpweb.Pagina, error) { return mcpweb.Buscar(ctx, o, srv.URL+caminho) }

		p, err := buscar("/html")
		if err != nil || p.Titulo != "Um título" || p.Texto != "Corpo" || p.Status != 200 {
			t.Fatal(tlsLigado, p, err)
		}
		for caminho, quer := range map[string]string{"/latin1": "licitação", "/texto": "texto puro", "/json": `{"a":1}`,
			"/pdf": "[página 1]\ntexto da primeira pagina do edital\n\n[página 2]\nsegunda", "/volta": "texto puro"} {
			p, err := buscar(caminho)
			if err != nil || !strings.Contains(p.Texto, quer) {
				t.Error(caminho, p.Texto, err)
			}
		}
		g, err := buscar("/grande")
		if err != nil || !g.Truncado || len([]rune(g.Texto)) != 40000 {
			t.Error(g.Truncado, err)
		}
		for caminho, quer := range map[string]string{"/sumiu": "HTTP 404", "/binario": "não suportado",
			"/laco": "redirecionamentos", "/fora": "recusou", "/arquivo": "use http"} {
			if _, err := buscar(caminho); err == nil || !strings.Contains(err.Error(), quer) {
				t.Error(caminho, err)
			}
		}
		// Sem proxy não há rede, mesmo que o host seja alcançável.
		if _, err := mcpweb.Buscar(ctx, mcpweb.Opcoes{}, srv.URL+"/texto"); err == nil {
			t.Error("a ferramenta acessou a rede sem o proxy")
		}
	}
}

func TestValidarURL(t *testing.T) {
	for _, ruim := range []string{"", "ftp://a", "http://", "http://u@a.com", "http://[::1", "javascript:alert(1)"} {
		if _, err := mcpweb.ValidarURL(ruim); err == nil {
			t.Errorf("%q aceito", ruim)
		}
	}
	if _, err := mcpweb.ValidarURL(" https://a.com/x "); err != nil {
		t.Error(err)
	}
}

func TestServidorMCP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(paginas))
	defer srv.Close()
	o := montar(t, srv)
	s, err := ferramentas.Conectar(context.Background(), transporte(t, mcpweb.Novo(o)))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var p mcpweb.Pagina
	if err := s.Chamar(context.Background(), "buscar_url", mcpweb.Entrada{URL: srv.URL + "/texto"}, &p); err != nil || p.Texto != "texto puro" {
		t.Fatal(p, err)
	}
	if err := s.Chamar(context.Background(), "buscar_url", mcpweb.Entrada{URL: "x"}, &p); err == nil {
		t.Error("erro da ferramenta precisa aparecer")
	}
}
