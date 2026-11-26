// Package textohtml extrai o texto legível de um documento HTML.
package textohtml

import (
	"io"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var ignorados = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Template: true,
	atom.Svg: true, atom.Iframe: true, atom.Head: true,
}

var blocos = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Br: true, atom.Li: true, atom.Tr: true, atom.Table: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Section: true, atom.Article: true, atom.Header: true, atom.Footer: true, atom.Blockquote: true,
	atom.Pre: true, atom.Ul: true, atom.Ol: true, atom.Nav: true, atom.Main: true, atom.Hr: true,
}

// Extrair devolve o título e o texto, com uma linha por bloco.
func Extrair(r io.Reader) (titulo, texto string, err error) {
	raiz, err := html.Parse(r)
	if err != nil {
		return "", "", err
	}
	var b strings.Builder
	var visitar func(n *html.Node)
	visitar = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(strings.Map(func(r rune) rune {
				if r == '\n' || r == '\r' || r == '\t' {
					return ' '
				}
				return r
			}, n.Data))
			return
		case html.ElementNode:
			if ignorados[n.DataAtom] {
				return
			}
			if blocos[n.DataAtom] {
				b.WriteByte('\n')
			}
			if n.DataAtom == atom.Td || n.DataAtom == atom.Th {
				b.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visitar(c)
		}
		if n.Type == html.ElementNode && blocos[n.DataAtom] {
			b.WriteByte('\n')
		}
	}
	var acharTitulo func(n *html.Node)
	acharTitulo = func(n *html.Node) {
		if titulo != "" {
			return
		}
		if n.Type == html.ElementNode && n.DataAtom == atom.Title && n.FirstChild != nil {
			titulo = strings.Join(strings.Fields(n.FirstChild.Data), " ")
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			acharTitulo(c)
		}
	}
	acharTitulo(raiz)
	visitar(raiz)
	return titulo, Arrumar(b.String()), nil
}

// Arrumar junta espaços repetidos e remove linhas vazias em sequência.
func Arrumar(s string) string {
	linhas := strings.Split(s, "\n")
	saida := linhas[:0]
	vazia := true
	for _, l := range linhas {
		l = strings.Join(strings.Fields(l), " ")
		if l == "" {
			if !vazia {
				saida = append(saida, "")
			}
			vazia = true
			continue
		}
		saida, vazia = append(saida, l), false
	}
	return strings.TrimSpace(strings.Join(saida, "\n"))
}
