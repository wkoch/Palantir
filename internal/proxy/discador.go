package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Discador conecta a destinos externos através do socket do proxy. É o lado
// usado pelas ferramentas, dentro do contêiner.
type Discador struct{ Socket string }

// DialContext tem a assinatura esperada por http.Transport.
func (d Discador) DialContext(ctx context.Context, _, endereco string) (net.Conn, error) {
	if d.Socket == "" {
		return nil, errors.New("proxy de saída não configurado")
	}
	var nd net.Dialer
	c, err := nd.DialContext(ctx, "unix", d.Socket)
	if err != nil {
		return nil, fmt.Errorf("proxy de saída: %w", err)
	}
	prazo, ok := ctx.Deadline()
	if !ok {
		prazo = time.Now().Add(30 * time.Second)
	}
	if err := c.SetDeadline(prazo); err != nil {
		c.Close()
		return nil, err
	}
	if _, err := fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", endereco, endereco); err != nil {
		c.Close()
		return nil, err
	}
	r := bufio.NewReader(c)
	res, err := http.ReadResponse(r, &http.Request{Method: http.MethodConnect})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("proxy de saída: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		motivo, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		c.Close()
		return nil, fmt.Errorf("proxy de saída recusou %s: %s", endereco, strings.TrimSpace(string(motivo)))
	}
	if err := c.SetDeadline(time.Time{}); err != nil {
		c.Close()
		return nil, err
	}
	return &comBuffer{Conn: c, r: r}, nil
}

// comBuffer preserva o que o destino enviou logo após a resposta do CONNECT,
// como a saudação de um servidor IMAP.
type comBuffer struct {
	net.Conn
	r *bufio.Reader
}

func (c *comBuffer) Read(p []byte) (int, error) { return c.r.Read(p) }
