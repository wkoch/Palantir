package proxy

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wkoch/Palantir/internal/diario"
)

type resolver map[string][]string

func (r resolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	ips, ok := r[host]
	if !ok {
		return nil, errors.New("nome desconhecido")
	}
	var out []netip.Addr
	for _, ip := range ips {
		out = append(out, netip.MustParseAddr(ip))
	}
	return out, nil
}

func TestPublico(t *testing.T) {
	for ip, quer := range map[string]bool{
		"8.8.8.8": true, "2606:4700::1111": true, "127.0.0.1": false, "10.1.2.3": false, "192.168.1.1": false,
		"172.16.0.1": false, "169.254.169.254": false, "100.64.0.1": false, "0.0.0.0": false, "0.1.2.3": false,
		"::1": false, "fe80::1": false, "fd00::1": false, "::ffff:127.0.0.1": false, "::ffff:8.8.8.8": true,
		"224.0.0.1": false, "255.255.255.255": false, "64:ff9b::7f00:1": false, "::": false,
	} {
		if Publico(netip.MustParseAddr(ip)) != quer {
			t.Errorf("Publico(%s) != %v", ip, quer)
		}
	}
}

// eco responde com uma saudação e devolve o que receber.
func eco(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				io.WriteString(c, "ola\n")
				io.Copy(c, c)
			}()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func novo(t *testing.T, p *Proxy) Discador {
	t.Helper()
	caminho := filepath.Join(t.TempDir(), "s")
	if err := p.Escutar(caminho); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Fechar() })
	return Discador{Socket: caminho}
}

func conversa(t *testing.T, d Discador, destino string) error {
	t.Helper()
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	c, err := d.DialContext(ctx, "tcp", destino)
	if err != nil {
		return err
	}
	defer c.Close()
	r := bufio.NewReader(c)
	// A saudação enviada logo após o CONNECT não se perde.
	if l, err := r.ReadString('\n'); err != nil || l != "ola\n" {
		return errors.Join(errors.New("sem saudação: "+l), err)
	}
	if _, err := io.WriteString(c, "eco\n"); err != nil {
		return err
	}
	if l, err := r.ReadString('\n'); err != nil || l != "eco\n" {
		return errors.Join(errors.New("sem eco: "+l), err)
	}
	return nil
}

func TestRegras(t *testing.T) {
	porta := eco(t)
	destino := func(h string) string { return net.JoinHostPort(h, strconv.Itoa(porta)) }
	dir := t.TempDir()
	reg, err := diario.Abrir(filepath.Join(dir, "d"))
	if err != nil {
		t.Fatal(err)
	}
	locais := false
	p := &Proxy{Ferramenta: "web", Diario: reg, PermitirLocais: func() bool { return locais },
		Resolver: resolver{"interno.test": {"127.0.0.1"}, "duplo.test": {"127.0.0.2", "127.0.0.1"}},
		Fixas: func() []Regra {
			return []Regra{{Host: "Interno.Test.", Portas: []int{porta}}, {Host: "127.0.0.1", Portas: []int{porta}, Local: true},
				{Host: "web.test"}, {Host: "sumido.test"}}
		}}
	d := novo(t, p)

	recusa := func(alvo, motivo string) {
		t.Helper()
		err := conversa(t, d, alvo)
		if err == nil || !strings.Contains(err.Error(), motivo) {
			t.Errorf("%s: %v (quero %q)", alvo, err, motivo)
		}
	}
	if err := conversa(t, d, destino("127.0.0.1")); err != nil {
		t.Fatal(err)
	}
	recusa(destino("outro.test"), "não está na lista")
	recusa("127.0.0.1:1", "não está na lista")
	// Um nome liberado que aponta para a própria máquina é bloqueado.
	recusa(destino("interno.test"), "não tem endereço público")
	recusa("web.test:8080", "não está na lista")
	recusa("web.test:443", "recusou")
	recusa("sumido.test:80", "resolver")
	recusa("semporta", "inválido")
	recusa("web.test:0", "porta inválida")
	recusa("web.test:abc", "")

	locais = true
	if err := conversa(t, d, destino("INTERNO.test.")); err != nil {
		t.Error(err)
	}
	// Concessões valem só até serem revogadas.
	revogar := p.Conceder(Regra{Host: "duplo.test", Portas: []int{porta}})
	if err := conversa(t, d, destino("duplo.test")); err != nil {
		t.Error(err)
	}
	revogar()
	recusa(destino("duplo.test"), "não está na lista")
	locais = false
	revogar = p.Conceder(Regra{Host: "interno.test", Portas: []int{porta}, Local: true})
	defer revogar()
	if err := conversa(t, d, destino("interno.test")); err != nil {
		t.Error(err)
	}

	entradas, _ := reg.Ultimas(100, 0)
	decisoes := map[string]int{}
	for _, e := range entradas {
		if e.Tipo != "rede" || e.Nome != "web" {
			t.Error(e)
		}
		decisoes[e.Decisao]++
	}
	if decisoes["permitido"] < 4 || decisoes["bloqueado"] < 5 || decisoes["erro"] < 1 {
		t.Error(decisoes)
	}
}

func TestSomenteConnect(t *testing.T) {
	p := &Proxy{Ferramenta: "x"}
	d := novo(t, p)
	cliente := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", d.Socket)
	}}}
	res, err := cliente.Get("http://exemplo.test/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Error(res.StatusCode)
	}
	// Lixo é descartado.
	c, err := net.Dial("unix", d.Socket)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(c, "lixo\r\n\r\n")
	b, _ := io.ReadAll(c)
	if len(b) != 0 {
		t.Errorf("%q", b)
	}
	c.Close()
}

func TestDiarioIndisponivelBloqueia(t *testing.T) {
	porta := eco(t)
	dir := t.TempDir()
	reg, err := diario.Abrir(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	// Transforma o diário num diretório, que não aceita escrita.
	if err := replaceWithDir(filepath.Join(dir, "sub")); err != nil {
		t.Fatal(err)
	}
	p := &Proxy{Ferramenta: "x", Diario: reg,
		Fixas: func() []Regra { return []Regra{{Host: "127.0.0.1", Portas: []int{porta}, Local: true}} }}
	d := novo(t, p)
	if err := conversa(t, d, "127.0.0.1:"+strconv.Itoa(porta)); err == nil {
		t.Error("sem diário não há acesso")
	}
}

func TestEscutarEFechar(t *testing.T) {
	p := &Proxy{}
	if p.Fechar() != nil {
		t.Error("fechar sem escutar")
	}
	if p.Escutar("/proc/nao/existe") == nil {
		t.Error("deveria falhar")
	}
	porta := eco(t)
	p.Fixas = func() []Regra { return []Regra{{Host: "127.0.0.1", Portas: []int{porta}, Local: true}} }
	d := novo(t, p)
	c, err := d.DialContext(context.Background(), "tcp", "127.0.0.1:"+strconv.Itoa(porta))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// Fechar o proxy derruba os túneis abertos.
	if err := p.Fechar(); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadAll(c); err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			t.Error("o túnel continuou aberto")
		}
	}
}

func TestDiscadorErros(t *testing.T) {
	ctx := context.Background()
	if _, err := (Discador{}).DialContext(ctx, "tcp", "a:1"); err == nil {
		t.Error("sem socket")
	}
	if _, err := (Discador{Socket: "/nao/existe"}).DialContext(ctx, "tcp", "a:1"); err == nil {
		t.Error("socket ausente")
	}
	caminho := filepath.Join(t.TempDir(), "s")
	l, err := net.Listen("unix", caminho)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, err := l.Accept()
		if err == nil {
			c.Close()
		}
	}()
	if _, err := (Discador{Socket: caminho}).DialContext(ctx, "tcp", "a:1"); err == nil {
		t.Error("proxy que fecha")
	}
}
