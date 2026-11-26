package servidor_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/wkoch/Palantir/internal/eventos"
	"github.com/wkoch/Palantir/internal/servidor"
	"github.com/wkoch/Palantir/internal/testes"
)

type cliente struct {
	t   *testing.T
	url string
	s   *testes.Sistema
}

func montar(t *testing.T) cliente {
	t.Helper()
	s := testes.Novo(t, testes.Opcoes{})
	srv := &servidor.Servidor{D: s.D, Versao: "v-teste", Interface: fstest.MapFS{
		"index.html": {Data: []byte("<pl-app>")}, "main.js": {Data: []byte("x()")}, ".gitignore": {Data: []byte("*")}}}
	h := httptest.NewServer(srv.Handler())
	t.Cleanup(h.Close)
	return cliente{t, h.URL, s}
}

func (c cliente) pedir(metodo, rota, corpo string, cab ...string) (*http.Response, []byte) {
	c.t.Helper()
	req, err := http.NewRequest(metodo, c.url+rota, strings.NewReader(corpo))
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set(servidor.Cabecalho, "1")
	req.Header.Set("Authorization", "Bearer "+c.s.D.Chave)
	for i := 0; i+1 < len(cab); i += 2 {
		if cab[i] == "Host" {
			req.Host = cab[i+1]
			continue
		}
		req.Header.Set(cab[i], cab[i+1])
		if cab[i+1] == "" {
			req.Header.Del(cab[i])
		}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	return res, b
}

// ok faz o pedido, exige o código e devolve o JSON.
func (c cliente) quer(codigo int, metodo, rota, corpo string) map[string]any {
	c.t.Helper()
	res, b := c.pedir(metodo, rota, corpo)
	if res.StatusCode != codigo {
		c.t.Fatalf("%s %s: %d (quero %d): %s", metodo, rota, res.StatusCode, codigo, b)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		c.t.Fatalf("%s %s: %v %s", metodo, rota, err, b)
	}
	return v
}

func (c cliente) enviarArquivos(arquivos map[string]string) (int, map[string]any) {
	c.t.Helper()
	var b bytes.Buffer
	m := multipart.NewWriter(&b)
	m.WriteField("campo", "ignorado")
	for nome, conteudo := range arquivos {
		w, err := m.CreateFormFile("arquivo", nome)
		if err != nil {
			c.t.Fatal(err)
		}
		w.Write([]byte(conteudo))
	}
	m.Close()
	res, corpo := c.pedir("POST", "/api/acervo/importar", b.String(), "Content-Type", m.FormDataContentType())
	var v map[string]any
	json.Unmarshal(corpo, &v)
	return res.StatusCode, v
}

func TestProtecoes(t *testing.T) {
	c := montar(t)
	casos := []struct {
		metodo, rota string
		cab          []string
		quer         int
	}{
		{"GET", "/api/saude", nil, 200},
		{"GET", "/api/saude", []string{"Host", "atacante.example"}, 403},
		{"GET", "/api/saude", []string{"Host", "192.168.0.10:7700"}, 403},
		{"GET", "/api/saude", []string{"Host", "localhost:7700"}, 200},
		{"GET", "/api/saude", []string{"Host", "[::1]"}, 200},
		{"GET", "/api/saude", []string{"Origin", "http://atacante.example"}, 403},
		{"GET", "/api/saude", []string{"Sec-Fetch-Site", "cross-site"}, 403},
		{"GET", "/api/saude", []string{"Sec-Fetch-Site", "same-site"}, 403},
		{"GET", "/api/saude", []string{"Sec-Fetch-Site", "same-origin"}, 200},
		{"POST", "/api/atividades", []string{servidor.Cabecalho, ""}, 403},
		{"DELETE", "/api/segredos/openrouter", []string{servidor.Cabecalho, ""}, 403},
		{"GET", "/api/eventos", []string{"Origin", "http://atacante.example"}, 403},
		{"OPTIONS", "/api/atividades", []string{"Origin", "http://atacante.example", "Access-Control-Request-Method", "POST"}, 403},
	}
	for _, x := range casos {
		res, b := c.pedir(x.metodo, x.rota, "", x.cab...)
		if res.StatusCode != x.quer {
			t.Errorf("%s %s %v: %d %s", x.metodo, x.rota, x.cab, res.StatusCode, b)
		}
		if res.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Error("nenhuma outra origem é autorizada")
		}
	}
	res, _ := c.pedir("GET", "/", "")
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") || res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error(csp)
	}
	origemCerta := strings.TrimPrefix(c.url, "")
	if res, _ := c.pedir("GET", "/api/saude", "", "Origin", origemCerta); res.StatusCode != 200 {
		t.Error("a própria origem", res.StatusCode)
	}
}

func TestInterface(t *testing.T) {
	c := montar(t)
	for rota, quer := range map[string]int{"/": 200, "/main.js": 200, "/nao-existe": 404, "/.gitignore": 404, "/api/nada": 404} {
		if res, _ := c.pedir("GET", rota, ""); res.StatusCode != quer {
			t.Error(rota, res.StatusCode)
		}
	}
	if res, _ := c.pedir("POST", "/", ""); res.StatusCode != 405 {
		t.Error(res.StatusCode)
	}
	for _, s := range []*servidor.Servidor{{D: c.s.D}, {D: c.s.D, Interface: fstest.MapFS{}}} {
		h := httptest.NewServer(s.Handler())
		codigo, corpo := testes.Obter(t, h.URL)
		if codigo != 404 || !strings.Contains(corpo, "não foi compilada") {
			t.Error(codigo, corpo)
		}
		h.Close()
	}
}

func TestAtividades(t *testing.T) {
	c := montar(t)
	v := c.quer(200, "GET", "/api/estado", "")
	if v["modelos"] != "ok" || v["versao"] != "v-teste" || len(v["atividades"].([]any)) != 0 {
		t.Fatal(v)
	}
	x := c.quer(200, "POST", "/api/atividades", `{"tipo":"conversa","objetivo":"`+strings.Repeat("longo ", 100)+`Abra a página sugerida"}`)
	id := strconv.Itoa(int(x["id"].(float64)))
	c.s.D.Gerente.Esperar()
	v = c.quer(200, "GET", "/api/estado", "")
	item := v["atividades"].([]any)[0].(map[string]any)
	if item["estado"] != "esperando_voce" || item["custo_usd"].(float64) <= 0 || len([]rune(item["objetivo"].(string))) != 301 {
		t.Error(item)
	}
	conf := v["confirmacoes"].([]any)[0].(map[string]any)
	d := c.quer(200, "GET", "/api/atividades/"+id, "")
	if d["tela"] != "conversa" || len(d["confirmacoes"].([]any)) != 1 || len(d["eventos"].([]any)) == 0 {
		t.Error(d)
	}
	// A ação interna não pode ser chamada de fora: seria aprovar sem clicar.
	c.quer(403, "POST", "/api/atividades/"+id+"/acoes/confirmacao_decidida", `{"id":1,"estado":"aprovada"}`)
	cid := strconv.Itoa(int(conf["id"].(float64)))
	c.quer(400, "POST", "/api/confirmacoes/"+cid, `{}`)
	c.quer(400, "POST", "/api/confirmacoes/"+cid, `x`)
	c.quer(400, "POST", "/api/confirmacoes/abc", `{"aprovar":true}`)
	c.quer(200, "POST", "/api/confirmacoes/"+cid, `{"aprovar":false}`)
	c.quer(409, "POST", "/api/confirmacoes/"+cid, `{"aprovar":true}`)
	c.s.D.Gerente.Esperar()
	r := c.quer(200, "POST", "/api/atividades/"+id+"/acoes/mensagem", `{"texto":"oi"}`)
	if r["ok"] != true {
		t.Error(r)
	}
	c.s.D.Gerente.Esperar()
	c.quer(200, "POST", "/api/atividades/"+id+"/orcamento", `{"usd":5}`)
	c.quer(400, "POST", "/api/atividades/"+id+"/orcamento", `{"usd":-1}`)
	c.quer(200, "POST", "/api/atividades/"+id+"/estado", `{"estado":"pausada"}`)
	c.quer(200, "POST", "/api/atividades/"+id+"/estado", `{"estado":"arquivada"}`)
	c.quer(409, "POST", "/api/atividades/"+id+"/estado", `{"estado":"pausada"}`)
	c.quer(409, "POST", "/api/atividades/"+id+"/acoes/mensagem", `{"texto":"oi"}`)
	if v := c.quer(200, "GET", "/api/estado?arquivadas=1", ""); len(v["atividades"].([]any)) != 1 {
		t.Error(v)
	}
	erros := []struct {
		codigo              int
		metodo, rota, corpo string
	}{
		{400, "POST", "/api/atividades", `{"tipo":"x"}`},
		{400, "POST", "/api/atividades", `{"inesperado":1}`},
		{413, "POST", "/api/atividades", `{"objetivo":"` + strings.Repeat("a", 3<<20) + `"}`},
		{404, "GET", "/api/atividades/999", ""},
		{400, "GET", "/api/atividades/-1", ""},
		{404, "POST", "/api/atividades/999/estado", `{"estado":"pausada"}`},
		{400, "POST", "/api/atividades/x/estado", `{}`},
		{400, "POST", "/api/atividades/1/estado", `[`},
		{400, "POST", "/api/atividades/x/orcamento", `{}`},
		{400, "POST", "/api/atividades/1/orcamento", `[`},
		{400, "POST", "/api/atividades/x/acoes/a", `{}`},
		{400, "POST", "/api/atividades/1/acoes/a", `{`},
		{413, "POST", "/api/atividades/1/acoes/a", strings.Repeat(" ", 3<<20)},
		{404, "POST", "/api/atividades/999/acoes/a", `{}`},
	}
	for _, e := range erros {
		c.quer(e.codigo, e.metodo, e.rota, e.corpo)
	}
	// Dados que não cabem no esquema da tela não chegam à interface.
	if _, err := c.s.D.DB.Exec(`UPDATE atividades SET dados = '{"forte":false,"itens":[{"id":1,"origem":"mentira"}]}' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if v := c.quer(500, "GET", "/api/atividades/"+id, ""); !strings.Contains(v["erro"].(string), "dados inválidos") {
		t.Error(v)
	}
}

func TestAcervo(t *testing.T) {
	c := montar(t)
	codigo, v := c.enviarArquivos(map[string]string{"contrato.txt": "Contrato de aluguel com multa.", "vazio.txt": ""})
	rel := v["relatorio"].(map[string]any)
	if codigo != 200 || rel["importados"] != 1.0 || len(rel["falhas"].([]any)) != 1 {
		t.Fatal(codigo, v)
	}
	if _, v := c.enviarArquivos(map[string]string{"outra.txt": "Contrato de aluguel com multa."}); v["relatorio"].(map[string]any)["repetidos"] != 1.0 {
		t.Error(v)
	}
	if codigo, _ := c.enviarArquivos(nil); codigo != 400 {
		t.Error(codigo)
	}
	c.quer(400, "POST", "/api/acervo/importar", `{}`)
	if res, _ := c.pedir("POST", "/api/acervo/importar", "--x\r\nlixo", "Content-Type", "multipart/form-data; boundary=x"); res.StatusCode != 400 {
		t.Error(res.StatusCode)
	}
	c.enviarArquivos(map[string]string{"pagina.html": "<script>alert(1)</script>", "desenho.svg": "<svg onload='x()'/>"})
	if err := c.s.D.Acervo.Processar(c.s.Contexto); err != nil {
		t.Fatal(err)
	}
	l := c.quer(200, "GET", "/api/acervo/documentos", "")
	if len(l["documentos"].([]any)) != 3 || l["pasta_entrada"] == "" {
		t.Fatal(l)
	}
	d := c.quer(200, "GET", "/api/acervo/documentos/1", "")
	if len(d["trechos"].([]any)) != 1 {
		t.Error(d)
	}
	for nome, quer := range map[string][3]string{
		"contrato.txt": {"text/plain; charset=utf-8", "inline", "sandbox"},
		"pagina.html":  {"text/plain; charset=utf-8", "inline", "sandbox"},
		"desenho.svg":  {"application/octet-stream", "attachment", "sandbox"},
	} {
		lista := c.quer(200, "GET", "/api/acervo/documentos?nome="+nome, "")
		id := strconv.Itoa(int(lista["documentos"].([]any)[0].(map[string]any)["id"].(float64)))
		res, _ := c.pedir("GET", "/api/acervo/documentos/"+id+"/original", "")
		h := res.Header
		if res.StatusCode != 200 || h.Get("Content-Type") != quer[0] || !strings.HasPrefix(h.Get("Content-Disposition"), quer[1]) ||
			!strings.Contains(h.Get("Content-Security-Policy"), quer[2]) {
			t.Error(nome, res.StatusCode, h)
		}
	}
	c.enviarArquivos(map[string]string{"doc.pdf": "%PDF-1.4"})
	lista := c.quer(200, "GET", "/api/acervo/documentos?nome=doc.pdf", "")
	id := strconv.Itoa(int(lista["documentos"].([]any)[0].(map[string]any)["id"].(float64)))
	if res, _ := c.pedir("GET", "/api/acervo/documentos/"+id+"/original", ""); res.Header.Get("Content-Type") != "application/pdf" ||
		strings.Contains(res.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Error(res.Header)
	}

	if v := c.quer(200, "GET", "/api/acervo/busca?q=aluguel", ""); len(v["achados"].([]any)) != 1 {
		t.Error(v)
	}
	col := c.quer(200, "POST", "/api/acervo/colecoes", `{"nome":"Casa"}`)
	cid := strconv.Itoa(int(col["id"].(float64)))
	c.quer(200, "POST", "/api/acervo/colecoes/"+cid, `{"documento":1,"incluir":true}`)
	c.quer(200, "POST", "/api/acervo/documentos/1/reextrair", "")
	c.quer(200, "GET", "/api/acervo/visoes?tipo=data", "")
	if v := c.quer(200, "POST", "/api/acervo/verificar", ""); len(v["problemas"].([]any)) != 0 {
		t.Error(v)
	}

	pasta := t.TempDir()
	os.WriteFile(filepath.Join(pasta, "n.txt"), []byte("novo"), 0o600)
	if v := c.quer(200, "POST", "/api/acervo/importar-pasta", `{"caminho":"`+pasta+`"}`); v["relatorio"].(map[string]any)["importados"] != 1.0 {
		t.Error(v)
	}
	link := filepath.Join(pasta, "atalho")
	os.Symlink(c.s.Caminhos.Config, link)
	erros := []struct {
		codigo              int
		metodo, rota, corpo string
	}{
		{400, "POST", "/api/acervo/importar-pasta", `{"caminho":"relativo"}`},
		{400, "POST", "/api/acervo/importar-pasta", `{"caminho":"` + c.s.Caminhos.Config + `"}`},
		{400, "POST", "/api/acervo/importar-pasta", `{"caminho":"` + link + `"}`},
		{400, "POST", "/api/acervo/importar-pasta", `{"caminho":"` + c.s.Caminhos.Originais() + `"}`},
		{404, "POST", "/api/acervo/importar-pasta", `{"caminho":"/nao/existe"}`},
		{400, "POST", "/api/acervo/importar-pasta", `x`},
		{400, "GET", "/api/acervo/busca?q=", ""},
		{404, "GET", "/api/acervo/documentos/999", ""},
		{400, "GET", "/api/acervo/documentos/x", ""},
		{400, "GET", "/api/acervo/documentos/x/original", ""},
		{404, "GET", "/api/acervo/documentos/999/original", ""},
		{400, "POST", "/api/acervo/documentos/x/reextrair", ""},
		{404, "POST", "/api/acervo/documentos/999/reextrair", ""},
		{400, "POST", "/api/acervo/colecoes", `{"nome":""}`},
		{400, "POST", "/api/acervo/colecoes", `x`},
		{400, "POST", "/api/acervo/colecoes/x", `{}`},
		{400, "POST", "/api/acervo/colecoes/1", `x`},
		{404, "POST", "/api/acervo/colecoes/999", `{"documento":1,"incluir":true}`},
	}
	for _, e := range erros {
		c.quer(e.codigo, e.metodo, e.rota, e.corpo)
	}

	res, b := c.pedir("GET", "/api/exportar", "")
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/zip" {
		t.Fatal(res.StatusCode, string(b))
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	nomes := map[string]bool{}
	for _, f := range z.File {
		nomes[f.Name] = true
		r, _ := f.Open()
		conteudo, _ := io.ReadAll(r)
		r.Close()
		if bytes.Contains(conteudo, []byte(testes.Chave)) {
			t.Error("a chave foi exportada em", f.Name)
		}
	}
	for _, n := range []string{"metadados.json", "atividades.json", "cartoes.json"} {
		if !nomes[n] {
			t.Error("falta", n)
		}
	}
	if sobras, _ := filepath.Glob(filepath.Join(c.s.Caminhos.Execucao, "exportacao-*")); len(sobras) != 0 {
		t.Error(sobras)
	}
	c.quer(200, "POST", "/api/acervo/regenerar", "")

	// Um original ausente impede exportar e regenerar, com a causa.
	originais, _ := filepath.Glob(filepath.Join(c.s.Caminhos.Originais(), "*"))
	for _, o := range originais {
		os.Remove(o)
	}
	c.quer(404, "GET", "/api/exportar", "")
	c.quer(409, "POST", "/api/acervo/regenerar", "")
	c.quer(404, "GET", "/api/acervo/documentos/1/original", "")
}

func TestAjustesSegredosDiarioEPainel(t *testing.T) {
	c := montar(t)
	res, b := c.pedir("GET", "/api/ajustes", "")
	if res.StatusCode != 200 || bytes.Contains(b, []byte(testes.Chave)) || !bytes.Contains(b, []byte(`"openrouter":true`)) {
		t.Fatal(res.StatusCode, string(b))
	}
	c.quer(200, "PUT", "/api/ajustes", `{"orcamento":{"mensal_usd":3,"por_atividade_usd":1},"web_hosts":["a.org"]}`)
	v := c.quer(200, "GET", "/api/ajustes", "")
	if v["web_hosts"].([]any)[0] != "a.org" {
		t.Error(v)
	}
	c.quer(400, "PUT", "/api/ajustes", `{"web_hosts":["x y"]}`)
	// Nada além dos três ajustes pode ser alterado pela interface.
	c.quer(400, "PUT", "/api/ajustes", `{"ferramentas":{"isolamento":"nenhum"}}`)
	c.quer(400, "PUT", "/api/ajustes", `{"modelos":{"url":"http://atacante.example"}}`)

	c.quer(200, "PUT", "/api/segredos/openrouter", `{"valor":"nova-chave-secreta-xyz"}`)
	c.quer(400, "PUT", "/api/segredos/outro", `{"valor":"x"}`)
	c.quer(400, "PUT", "/api/segredos/openrouter", `{"valor":""}`)
	c.quer(400, "PUT", "/api/segredos/openrouter", `{"valor":"`+strings.Repeat("a", 5000)+`"}`)
	c.quer(400, "PUT", "/api/segredos/openrouter", `x`)
	c.quer(400, "DELETE", "/api/segredos/outro", "")
	c.quer(200, "DELETE", "/api/segredos/openrouter", "")
	if res, _ := c.pedir("GET", "/api/segredos/openrouter", ""); res.StatusCode != 404 {
		t.Error("não há como ler um segredo pela API", res.StatusCode)
	}
	if v := c.quer(200, "GET", "/api/estado", ""); v["modelos"] != "sem_chave" {
		t.Error(v)
	}
	d := c.quer(200, "GET", "/api/diario?n=3", "")
	if len(d["entradas"].([]any)) != 3 {
		t.Error(d)
	}
	c.quer(200, "GET", "/api/diario?n=99999&atividade=1", "")
	c.quer(400, "GET", "/api/diario?%zz", "")
	_, b = c.pedir("GET", "/api/diario?n=1000", "")
	if bytes.Contains(b, []byte("nova-chave-secreta")) {
		t.Error("segredo no diário")
	}
	c.s.Criar("conversa", "oi", "")
	p := c.quer(200, "GET", "/api/painel", "")
	if p["teto_mes_usd"] != 3.0 || p["atividades"].(map[string]any)["esperando_voce"] != 1.0 {
		t.Error(p)
	}
	// Erro interno é informado sem derrubar o servidor.
	c.s.D.DB.Close()
	for _, rota := range []string{"/api/estado", "/api/painel", "/api/acervo/documentos", "/api/acervo/visoes", "/api/exportar", "/api/atividades/1"} {
		c.quer(500, "GET", rota, "")
	}
}

func TestEventosPorWebSocket(t *testing.T) {
	c := montar(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(c.url, "http")+"/api/eventos",
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {c.url}}, Subprotocols: subprotocolos(c.s.D.Chave)})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	// Publica até o assinante receber, pois a assinatura é assíncrona.
	recebido := make(chan eventos.Evento, 1)
	go func() {
		var e eventos.Evento
		if wsjson.Read(ctx, ws, &e) == nil {
			recebido <- e
		}
	}()
	for {
		c.s.D.Eventos.Publicar(eventos.Evento{Tipo: eventos.Atividade, Atividade: 7})
		select {
		case e := <-recebido:
			if e.Tipo != eventos.Atividade || e.Atividade != 7 {
				t.Error(e)
			}
			ws.Close(websocket.StatusNormalClosure, "")
			// Um GET comum não é um WebSocket.
			if res, _ := c.pedir("GET", "/api/eventos", ""); res.StatusCode == 200 {
				t.Error(res.StatusCode)
			}
			return
		case <-time.After(20 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal("nenhum evento chegou")
		}
	}
}

func TestEscutarEServir(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	srv := &servidor.Servidor{D: s.D, Versao: "v"}
	l, err := srv.Escutar()
	if err != nil {
		t.Fatal(err)
	}
	if h := l.Addr().(*net.TCPAddr).IP; !h.IsLoopback() {
		t.Error("o daemon só atende a própria máquina", h)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	fim := make(chan error)
	go func() { fim <- srv.Servir(ctx, l) }()
	if codigo, corpo := testes.Obter(t, "http://"+l.Addr().String()+"/api/saude"); codigo != 200 || !strings.Contains(corpo, `"v"`) {
		t.Error(codigo, corpo)
	}
	ws, _, err := websocket.Dial(ctx, "ws://"+l.Addr().String()+"/api/eventos",
		&websocket.DialOptions{Subprotocols: subprotocolos(s.D.Chave)})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	cancelar()
	select {
	case err := <-fim:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("não encerrou com um WebSocket aberto")
	}
	if err := srv.Servir(context.Background(), l); err == nil {
		t.Error("ouvinte fechado")
	}
}
