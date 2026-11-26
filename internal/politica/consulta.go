package politica

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxConsulta limita o que uma única pesquisa pode levar para fora.
const maxConsulta = 300

// palavrasLivres não carregam informação e podem ligar as palavras do usuário.
var palavrasLivres = map[string]bool{"a": true, "o": true, "as": true, "os": true, "de": true, "da": true,
	"do": true, "das": true, "dos": true, "e": true, "em": true, "no": true, "na": true, "nos": true, "nas": true,
	"um": true, "uma": true, "para": true, "por": true, "com": true, "sobre": true, "que": true}

func palavrasDe(t string) []string {
	return strings.FieldsFunc(strings.ToLower(t), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

// soPalavrasDe informa se a consulta não acrescenta nenhuma palavra ao que o
// usuário escreveu.
func soPalavrasDe(consulta string, textos []string) bool {
	escritas := map[string]bool{}
	for _, t := range textos {
		for _, p := range palavrasDe(t) {
			escritas[p] = true
		}
	}
	for _, p := range palavrasDe(consulta) {
		if !escritas[p] && !palavrasLivres[p] {
			return false
		}
	}
	return true
}

// avaliarConsulta trata a pesquisa como saída de dados: o texto dela chega a um
// serviço de fora.
func avaliarConsulta(p Pedido) Decisao {
	consulta := strings.TrimSpace(p.Consulta)
	switch {
	case consulta == "":
		return decisao(Bloquear, "consulta vazia")
	case utf8.RuneCountInString(consulta) > maxConsulta:
		return decisao(Bloquear, "consulta longa demais para uma pesquisa")
	case len(p.TextosExternos) == 0:
		return decisao(Permitir, "pesquisa feita antes de haver conteúdo externo")
	case soPalavrasDe(consulta, p.TextosUsuario):
		return decisao(Permitir, "a consulta só usa palavras que você escreveu")
	}
	return decisao(Confirmar, "a consulta tem palavras que você não escreveu, e há conteúdo externo na conversa")
}
