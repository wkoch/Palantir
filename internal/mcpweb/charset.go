package mcpweb

import (
	"bytes"
	"io"

	"golang.org/x/net/html/charset"
)

// emUTF8 converte o corpo conforme o cabeçalho e as marcas do documento, como
// as páginas em ISO-8859-1 de órgãos públicos.
func emUTF8(corpo []byte, tipoConteudo string) (io.Reader, error) {
	return charset.NewReader(bytes.NewReader(corpo), tipoConteudo)
}
