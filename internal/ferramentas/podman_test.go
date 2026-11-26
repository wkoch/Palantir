//go:build podman

// Testes com o Podman de verdade (scripts/testar-conteiner.sh).
package ferramentas_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/extrator"
	"github.com/wkoch/Palantir/internal/ferramentas"
	"github.com/wkoch/Palantir/internal/mcpimap"
	"github.com/wkoch/Palantir/internal/mcpweb"
	"github.com/wkoch/Palantir/internal/proxy"
	"github.com/wkoch/Palantir/internal/simulador"
	"github.com/wkoch/Palantir/internal/testes"
)

func imagem(t *testing.T) string {
	t.Helper()
	// O Podman sem root fala com o systemd do usuário pelo D-Bus.
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", testes.BarramentoOriginal)
	i := os.Getenv("PALANTIR_IMAGEM_TESTE")
	if i == "" {
		t.Skip("defina PALANTIR_IMAGEM_TESTE")
	}
	return i
}

func executor(t *testing.T) *ferramentas.Processo {
	img := imagem(t)
	return &ferramentas.Processo{Prazo: 2 * time.Minute, Config: func() config.Ferramentas {
		return config.Ferramentas{Isolamento: "podman", Imagem: img}
	}}
}

func original(t *testing.T, conteudo []byte) string {
	p := filepath.Join(testes.Pasta(t), "original")
	if err := os.WriteFile(p, conteudo, 0o400); err != nil {
		t.Fatal(err)
	}
	return p
}

func extrair(t *testing.T, nome string, conteudo []byte) extrator.Resultado {
	t.Helper()
	var r extrator.Resultado
	err := ferramentas.Chamar(context.Background(), executor(t),
		ferramentas.Execucao{Servidor: ferramentas.Extrair, Entrada: original(t, conteudo)},
		"extrair", map[string]any{"nome": nome}, &r)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPodmanExtracaoDeTexto(t *testing.T) {
	r := extrair(t, "nota.txt", []byte("texto dentro do contêiner"))
	if len(r.Paginas) != 1 || r.Paginas[0].Texto != "texto dentro do contêiner" {
		t.Fatal(r)
	}
}

func TestPodmanPDF(t *testing.T) {
	r := extrair(t, "contrato.pdf", pdfDeTeste("Contrato com texto verdadeiro dentro do PDF"))
	if len(r.Paginas) != 1 || !strings.Contains(r.Paginas[0].Texto, "texto verdadeiro") {
		t.Fatal(r)
	}
}

// TestPodmanOCR desenha texto num PDF, converte em imagem e reconhece.
func TestPodmanOCR(t *testing.T) {
	img := imagem(t)
	dir := testes.Pasta(t)
	if err := os.WriteFile(filepath.Join(dir, "t.pdf"), pdfDeTeste("CONTRATO DE LOCACAO RESIDENCIAL"), 0o600); err != nil {
		t.Fatal(err)
	}
	saida, err := exec.Command("podman", "run", "--rm", "--network=none", "--security-opt=label=disable",
		"--volume="+dir+":/t", img, "pdftoppm", "-r", "150", "-png", "-singlefile", "/t/t.pdf", "/t/pagina").CombinedOutput()
	if err != nil {
		t.Fatal(string(saida), err)
	}
	png, err := os.ReadFile(filepath.Join(dir, "pagina.png"))
	if err != nil {
		t.Fatal(err)
	}
	r := extrair(t, "digitalizado.png", png)
	if len(r.Paginas) != 1 || !strings.Contains(r.Paginas[0].Texto, "LOCACAO") {
		t.Fatal("o OCR não leu o texto:", r)
	}
}

func abrirProxy(t *testing.T, nome string, porta int) string {
	t.Helper()
	p := &proxy.Proxy{Ferramenta: nome, Fixas: func() []proxy.Regra {
		return []proxy.Regra{{Host: "127.0.0.1", Portas: []int{porta}, Local: true}}
	}}
	socket := filepath.Join(testes.Pasta(t), "proxy.sock")
	if err := p.Escutar(socket); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Fechar() })
	return socket
}

func TestPodmanWebSoPeloProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("alcançado pelo proxy"))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	porta, _ := strconv.Atoi(u.Port())
	var p mcpweb.Pagina
	ctx := context.Background()
	err := ferramentas.Chamar(ctx, executor(t), ferramentas.Execucao{Servidor: ferramentas.Web,
		ProxySocket: abrirProxy(t, "web", porta)}, "buscar_url", mcpweb.Entrada{URL: srv.URL}, &p)
	if err != nil || p.Texto != "alcançado pelo proxy" {
		t.Fatal(p, err)
	}
	// Fora da lista, o proxy recusa.
	err = ferramentas.Chamar(ctx, executor(t), ferramentas.Execucao{Servidor: ferramentas.Web,
		ProxySocket: abrirProxy(t, "web", porta+1)}, "buscar_url", mcpweb.Entrada{URL: srv.URL}, &p)
	if err == nil || !strings.Contains(err.Error(), "recusou") {
		t.Fatal(err)
	}
	// Sem o socket não há caminho nenhum.
	err = ferramentas.Chamar(ctx, executor(t), ferramentas.Execucao{Servidor: ferramentas.Web},
		"buscar_url", mcpweb.Entrada{URL: srv.URL}, &p)
	if err == nil {
		t.Fatal("a ferramenta acessou a rede sem proxy")
	}
}

func TestPodmanContainerSemRede(t *testing.T) {
	img := imagem(t)
	l, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	script := "cat /proc/net/dev | tail -n +3 | cut -d: -f1 | tr -d ' '; " +
		"timeout 3 bash -c 'exec 3<>/dev/tcp/1.1.1.1/443' && echo ALCANCOU || echo isolado"
	saida, err := exec.Command("podman", "run", "--rm", "--network=none", img, "bash", "-c", script).Output()
	if err != nil {
		t.Fatal(string(saida), err)
	}
	s := strings.Fields(string(saida))
	if strings.Contains(string(saida), "ALCANCOU") || s[0] != "lo" || s[len(s)-1] != "isolado" {
		t.Fatal(string(saida))
	}
}

func TestPodmanIMAPPeloProxy(t *testing.T) {
	correio, err := simulador.NovoCorreio("127.0.0.1:0", simulador.Emails, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer correio.Fechar()
	var r mcpimap.SaidaBuscar
	err = ferramentas.Chamar(context.Background(), executor(t), ferramentas.Execucao{Servidor: ferramentas.IMAP,
		ProxySocket: abrirProxy(t, "imap", correio.Porta()),
		Env: map[string]string{mcpimap.EnvServidor: "127.0.0.1", mcpimap.EnvPorta: strconv.Itoa(correio.Porta()),
			mcpimap.EnvUsuario: simulador.Usuario, mcpimap.EnvSenha: simulador.Senha, mcpimap.EnvCaixa: "INBOX",
			mcpimap.EnvSeguranca: "nenhum"}}, "buscar", mcpimap.EntradaBuscar{}, &r)
	if err != nil || len(r.Mensagens) != len(simulador.Emails) {
		t.Fatal(r, err)
	}
}

// pdfDeTeste monta um PDF de uma página.
func pdfDeTeste(texto string) []byte {
	fluxo := "BT /F1 28 Tf 40 700 Td (" + texto + ") Tj ET"
	objetos := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [4 0 R] /Count 1 >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents 5 0 R >>",
		"<< /Length " + strconv.Itoa(len(fluxo)) + " >>\nstream\n" + fluxo + "\nendstream"}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	var posicoes []int
	for i, o := range objetos {
		posicoes = append(posicoes, b.Len())
		b.WriteString(strconv.Itoa(i+1) + " 0 obj\n" + o + "\nendobj\n")
	}
	xref := b.Len()
	b.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for _, p := range posicoes {
		s := strconv.Itoa(p)
		b.WriteString(strings.Repeat("0", 10-len(s)) + s + " 00000 n \n")
	}
	b.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n" + strconv.Itoa(xref) + "\n%%EOF\n")
	return []byte(b.String())
}
