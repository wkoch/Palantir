package extrator

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	_ "github.com/emersion/go-message/charset" // ISO-8859-1 e afins
	"github.com/emersion/go-message/mail"

	"github.com/wkoch/Palantir/internal/textohtml"
)

// TextoDeEmail interpreta uma mensagem RFC 5322 e devolve os cabeçalhos
// principais seguidos do corpo em texto.
func TextoDeEmail(b []byte) (string, error) {
	r, err := mail.CreateReader(bytes.NewReader(b))
	if r == nil {
		return "", fmt.Errorf("mensagem inválida: %w", err)
	}
	defer r.Close()
	var s strings.Builder
	for _, c := range [][2]string{{"De", "From"}, {"Para", "To"}, {"Data", "Date"}, {"Assunto", "Subject"}} {
		if v, err := r.Header.Text(c[1]); err == nil && v != "" {
			s.WriteString(c[0] + ": " + v + "\n")
		}
	}
	var plano, html, anexos []string
	for range 200 {
		p, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if p == nil {
			// Uma mensagem truncada ainda vale pelo que foi lido.
			break
		}
		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			tipo, _, _ := h.ContentType()
			corpo, _ := io.ReadAll(io.LimitReader(p.Body, 4<<20))
			switch {
			case tipo == "text/html":
				if _, t, err := textohtml.Extrair(bytes.NewReader(corpo)); err == nil {
					html = append(html, t)
				}
			case tipo == "" || strings.HasPrefix(tipo, "text/"):
				plano = append(plano, paraUTF8(corpo))
			}
		case *mail.AttachmentHeader:
			if nome, err := h.Filename(); err == nil && nome != "" {
				anexos = append(anexos, nome)
			}
		}
	}
	if len(plano) == 0 {
		plano = html
	}
	s.WriteString("\n" + strings.TrimSpace(strings.Join(plano, "\n")))
	if len(anexos) > 0 {
		s.WriteString("\n\nAnexos: " + strings.Join(anexos, ", "))
	}
	return strings.TrimSpace(s.String()), nil
}

func email(b []byte) (Resultado, error) {
	t, err := TextoDeEmail(b)
	if err != nil {
		return Resultado{}, err
	}
	return unica("email", t), nil
}
