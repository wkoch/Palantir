package servidor_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/wkoch/Palantir/internal/servidor"
)

func subprotocolos(chave string) []string { return []string{servidor.Subprotocolo, "chave." + chave} }

func TestSemChaveNaoHaDados(t *testing.T) {
	c := montar(t)
	id := c.s.Importar("nota.html", "<p>segredo do acervo</p>")
	original := "/api/acervo/documentos/1/original"
	if id != 1 {
		t.Fatal(id)
	}
	sem := []string{"Authorization", ""}
	for _, x := range []struct{ metodo, rota string }{
		{"GET", "/api/estado"}, {"GET", "/api/atividades/1"}, {"GET", "/api/ajustes"}, {"GET", "/api/diario"},
		{"GET", "/api/acervo/documentos"}, {"GET", original}, {"GET", "/api/exportar"}, {"GET", "/api/eventos"},
		{"GET", "/api/nada"}, {"POST", "/api/atividades"}, {"POST", "/api/bilhetes"},
		{"PUT", "/api/segredos/openrouter"}, {"DELETE", "/api/segredos/openrouter"},
	} {
		for _, cab := range [][]string{sem, {"Authorization", "Bearer errada"}, {"Authorization", "Bearer "},
			{"Authorization", "", "Cookie", "chave=" + c.s.D.Chave}} {
			if res, b := c.pedir(x.metodo, x.rota, "{}", cab...); res.StatusCode != 401 {
				t.Error(x, cab, res.StatusCode, string(b))
			}
		}
	}
	// A interface e a saúde são públicas.
	for _, rota := range []string{"/", "/main.js", "/api/saude"} {
		if res, _ := c.pedir("GET", rota, "", sem...); res.StatusCode != 200 {
			t.Error(rota, res.StatusCode)
		}
	}
	// A chave na URL não vale.
	if res, _ := c.pedir("GET", "/api/estado?chave="+c.s.D.Chave+"&bilhete="+c.s.D.Chave, "", sem...); res.StatusCode != 401 {
		t.Error(res.StatusCode)
	}

	// O bilhete abre um único endereço, e só para leitura.
	v := c.quer(200, "POST", "/api/bilhetes", `{"caminho":"`+original+`"}`)
	endereco := v["endereco"].(string)
	if !strings.HasPrefix(endereco, original+"?bilhete=") || strings.Contains(endereco, c.s.D.Chave) {
		t.Fatal(endereco)
	}
	res, b := c.pedir("GET", endereco, "", sem...)
	if res.StatusCode != 200 || !strings.Contains(string(b), "segredo do acervo") {
		t.Fatal(res.StatusCode, string(b))
	}
	_, codigo, _ := strings.Cut(endereco, "?")
	for _, rota := range []string{"/api/exportar?" + codigo, "/api/estado?" + codigo, original + "?bilhete=outro"} {
		if res, _ := c.pedir("GET", rota, "", sem...); res.StatusCode != 401 {
			t.Error(rota, res.StatusCode)
		}
	}
	if res, _ := c.pedir("POST", endereco, "", sem...); res.StatusCode != 401 {
		t.Error("bilhete numa alteração", res.StatusCode)
	}
	for _, ruim := range []string{`{"caminho":"/api/estado"}`, `{"caminho":"/api/exportar/../ajustes"}`, `{}`, `x`} {
		c.quer(400, "POST", "/api/bilhetes", ruim)
	}
	// Muitos bilhetes não acumulam sem fim.
	for range 250 {
		c.quer(200, "POST", "/api/bilhetes", `{"caminho":"/api/exportar"}`)
	}

	// O WebSocket exige a chave no subprotocolo.
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	url := "ws" + strings.TrimPrefix(c.url, "http") + "/api/eventos"
	for _, ruim := range [][]string{nil, {servidor.Subprotocolo}, subprotocolos("errada")} {
		_, r, err := websocket.Dial(ctx, url, &websocket.DialOptions{Subprotocols: ruim})
		if err == nil || r == nil || r.StatusCode != http.StatusUnauthorized {
			t.Error(ruim, err)
		}
	}
	ws, r, err := websocket.Dial(ctx, url, &websocket.DialOptions{Subprotocols: subprotocolos(c.s.D.Chave)})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	if got := r.Header.Get("Sec-WebSocket-Protocol"); got != servidor.Subprotocolo {
		t.Error("o servidor devolveu o subprotocolo com a chave:", got)
	}
}
