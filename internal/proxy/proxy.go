// Package proxy é o único caminho de rede das ferramentas. Cada ferramenta tem
// seu socket e sua lista de hosts; o tráfego passa por um túnel CONNECT.
package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wkoch/Palantir/internal/diario"
)

// Regra libera um host. Portas vazias significam 80 e 443.
type Regra struct {
	Host   string
	Portas []int
	// Local permite que o host resolva para um endereço local ou privado.
	Local bool
}

func (r Regra) permite(host string, porta int) bool {
	if Normalizar(r.Host) != host {
		return false
	}
	if len(r.Portas) == 0 {
		return porta == 80 || porta == 443
	}
	return slices.Contains(r.Portas, porta)
}

// Normalizar põe um nome de host na forma usada nas comparações.
func Normalizar(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// Resolver traduz um nome em endereços; *net.Resolver o implementa.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Proxy atende uma ferramenta.
type Proxy struct {
	Ferramenta string
	Diario     *diario.Diario
	// Fixas devolve as regras da configuração vigente.
	Fixas func() []Regra
	// PermitirLocais libera endereços privados para todas as regras (testes).
	PermitirLocais func() bool
	Resolver       Resolver

	mu       sync.Mutex
	proxima  int
	avulsas  map[int]Regra
	ouvinte  net.Listener
	conexoes map[net.Conn]struct{}
}

// Conceder libera um host temporariamente. A função devolvida revoga.
func (p *Proxy) Conceder(r Regra) func() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.avulsas == nil {
		p.avulsas = map[int]Regra{}
	}
	p.proxima++
	id := p.proxima
	p.avulsas[id] = r
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.avulsas, id)
	}
}

func (p *Proxy) regra(host string, porta int) (Regra, bool) {
	var achada Regra
	ok := false
	considerar := func(r Regra) {
		if r.permite(host, porta) {
			achada.Local = achada.Local || r.Local
			ok = true
		}
	}
	if p.Fixas != nil {
		for _, r := range p.Fixas() {
			considerar(r)
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.avulsas {
		considerar(r)
	}
	return achada, ok
}

var (
	cgnat     = netip.MustParsePrefix("100.64.0.0/10")
	reservada = netip.MustParsePrefix("0.0.0.0/8")
	nat64     = netip.MustParsePrefix("64:ff9b::/96")
)

// Publico informa se o endereço pertence à internet pública.
func Publico(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && ip.IsGlobalUnicast() && !ip.IsPrivate() &&
		!cgnat.Contains(ip) && !reservada.Contains(ip) && !nat64.Contains(ip)
}

var errBloqueado = errors.New("destino bloqueado")

// discar aplica as regras e conecta ao endereço já resolvido, para que uma
// segunda consulta de DNS não troque o destino.
func (p *Proxy) discar(ctx context.Context, alvo string) (net.Conn, error) {
	host, portaTexto, err := net.SplitHostPort(alvo)
	if err != nil {
		return nil, fmt.Errorf("%w: destino inválido", errBloqueado)
	}
	porta, err := strconv.Atoi(portaTexto)
	if err != nil || porta < 1 || porta > 65535 {
		return nil, fmt.Errorf("%w: porta inválida", errBloqueado)
	}
	host = Normalizar(host)
	regra, ok := p.regra(host, porta)
	if !ok {
		return nil, fmt.Errorf("%w: %s:%d não está na lista da ferramenta", errBloqueado, host, porta)
	}
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{ip}
	} else {
		r := p.Resolver
		if r == nil {
			r = net.DefaultResolver
		}
		if ips, err = r.LookupNetIP(ctx, "ip", host); err != nil {
			return nil, fmt.Errorf("resolver %s: %w", host, err)
		}
	}
	locais := regra.Local || p.PermitirLocais != nil && p.PermitirLocais()
	var d net.Dialer
	var ultimo error = fmt.Errorf("%w: %s não tem endereço público", errBloqueado, host)
	for _, ip := range ips {
		if !locais && !Publico(ip) {
			continue
		}
		conn, err := d.DialContext(ctx, "tcp", netip.AddrPortFrom(ip.Unmap(), uint16(porta)).String())
		if err == nil {
			return conn, nil
		}
		ultimo = err
	}
	return nil, ultimo
}

// Escutar cria o socket Unix da ferramenta e passa a atender.
func (p *Proxy) Escutar(caminho string) error {
	if err := os.Remove(caminho); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	l, err := net.Listen("unix", caminho)
	if err != nil {
		return err
	}
	if err := os.Chmod(caminho, 0o600); err != nil {
		l.Close()
		return err
	}
	p.mu.Lock()
	p.ouvinte = l
	p.conexoes = map[net.Conn]struct{}{}
	p.mu.Unlock()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go p.atender(c)
		}
	}()
	return nil
}

// Fechar encerra o socket e os túneis abertos.
func (p *Proxy) Fechar() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ouvinte == nil {
		return nil
	}
	err := p.ouvinte.Close()
	for c := range p.conexoes {
		c.Close()
	}
	p.ouvinte, p.conexoes = nil, nil
	return err
}

func (p *Proxy) acompanhar(c net.Conn, abrir bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conexoes == nil {
		return
	}
	if abrir {
		p.conexoes[c] = struct{}{}
	} else {
		delete(p.conexoes, c)
	}
}

func responder(c net.Conn, codigo int, motivo string) {
	fmt.Fprintf(c, "HTTP/1.1 %d %s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		codigo, http.StatusText(codigo), len(motivo), motivo)
}

func (p *Proxy) atender(c net.Conn) {
	defer c.Close()
	p.acompanhar(c, true)
	defer p.acompanhar(c, false)

	_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
	leitor := bufio.NewReader(io.LimitReader(c, 16<<10))
	req, err := http.ReadRequest(leitor)
	if err != nil {
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	if req.Method != http.MethodConnect {
		p.registrar(req.Host, "bloqueado", "método "+req.Method)
		responder(c, http.StatusMethodNotAllowed, "somente CONNECT")
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	remoto, err := p.discar(ctx, req.Host)
	cancelar()
	if err != nil {
		codigo, decisao := http.StatusBadGateway, "erro"
		if errors.Is(err, errBloqueado) {
			codigo, decisao = http.StatusForbidden, "bloqueado"
		}
		p.registrar(req.Host, decisao, err.Error())
		responder(c, codigo, err.Error())
		return
	}
	defer remoto.Close()
	p.acompanhar(remoto, true)
	defer p.acompanhar(remoto, false)
	// Sem registro no diário, não há acesso.
	if err := p.registrar(req.Host, "permitido", ""); err != nil {
		responder(c, http.StatusInternalServerError, "diário indisponível")
		return
	}
	if _, err := io.WriteString(c, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	// Bytes que o cliente enviou junto com o CONNECT.
	adiantados, _ := leitor.Peek(leitor.Buffered())
	if _, err := remoto.Write(adiantados); err != nil {
		return
	}
	fim := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(remoto, c); fim <- struct{}{} }()
	go func() { _, _ = io.Copy(c, remoto); fim <- struct{}{} }()
	<-fim
}

func (p *Proxy) registrar(destino, decisao, motivo string) error {
	return p.Diario.Registrar(diario.Entrada{Tipo: "rede", Nome: p.Ferramenta, Decisao: decisao,
		Motivo: motivo, Detalhe: destino})
}
