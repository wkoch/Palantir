// Package politica decide o que cada atividade pode fazer. As regras são
// código: nenhum modelo as relaxa.
package politica

import (
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Veredito é o resultado de uma avaliação.
type Veredito string

// Vereditos possíveis, do mais ao menos permissivo.
const (
	Permitir  Veredito = "permitir"
	Confirmar Veredito = "confirmar"
	Bloquear  Veredito = "bloquear"
)

// Ferramentas que o daemon conhece.
const (
	BuscarAcervo   = "buscar_acervo"
	LerDocumento   = "ler_documento"
	BuscarURL      = "buscar_url"
	PesquisarWeb   = "pesquisar_web"
	MostrarTela    = "mostrar_tela"
	EmailBuscar    = "email_buscar"
	EmailLer       = "email_ler"
	EmailEtiquetar = "email_etiquetar"
)

// Tipos de atividade.
const (
	Conversa  = "conversa"
	Relatorio = "relatorio"
	Triagem   = "triagem"
	Estudo    = "estudo"
)

// permissoes é fixa no código: nem o modelo nem o conteúdo externo a alteram.
var permissoes = map[string][]string{
	Conversa:  {BuscarAcervo, LerDocumento, BuscarURL, PesquisarWeb, MostrarTela},
	Relatorio: {BuscarAcervo, BuscarURL, PesquisarWeb},
	Triagem:   {EmailBuscar, EmailLer, EmailEtiquetar},
	Estudo:    {BuscarAcervo, LerDocumento},
}

// Permissoes devolve as ferramentas de um tipo de atividade.
func Permissoes(tipo string) []string { return slices.Clone(permissoes[tipo]) }

// Decisao é o veredito e as razões.
type Decisao struct {
	Veredito Veredito `json:"veredito"`
	Motivos  []string `json:"motivos"`
}

func decisao(v Veredito, motivos ...string) Decisao { return Decisao{Veredito: v, Motivos: motivos} }

// Pedido é uma chamada de ferramenta proposta.
type Pedido struct {
	Tipo       string
	Ferramenta string
	// URL é o endereço pedido, para ferramentas que acessam a rede.
	URL string
	// Consulta é o texto de uma pesquisa na web.
	Consulta string
	// Achados são endereços devolvidos por pesquisas já feitas.
	Achados []string
	// TextosUsuario é o que o usuário escreveu: a única fonte de autorização.
	TextosUsuario []string
	// TextosExternos é o conteúdo de fora que já entrou no contexto.
	TextosExternos []string
	// HostsLiberados vem da configuração.
	HostsLiberados []string
}

var reURL = regexp.MustCompile("(?i)\\bhttps?://[^\\s<>\"'`\\]\\[{}|\\\\^]+")

// URLs encontra os endereços num texto, sem a pontuação do fim.
func URLs(texto string) []string {
	var achadas []string
	for _, u := range reURL.FindAllString(texto, -1) {
		for {
			cortada := strings.TrimRight(u, ".,;:!?…*_")
			if strings.HasSuffix(cortada, ")") && strings.Count(cortada, "(") < strings.Count(cortada, ")") {
				cortada = strings.TrimSuffix(cortada, ")")
			}
			if cortada == u {
				break
			}
			u = cortada
		}
		achadas = append(achadas, u)
	}
	return achadas
}

var reHost = regexp.MustCompile(`(?i)(?:[\p{L}\p{N}_-]+\.)+[\p{L}]{2,}`)

// citaHost informa se o texto menciona exatamente esse host, em uma URL ou solto.
func citaHost(textos []string, host string) bool {
	for _, t := range textos {
		for _, u := range URLs(t) {
			if p, err := url.Parse(u); err == nil && strings.EqualFold(strings.TrimSuffix(p.Hostname(), "."), host) {
				return true
			}
		}
		// Remove as URLs para que um host no caminho ou na consulta não conte.
		solto := reURL.ReplaceAllString(t, " ")
		// Endereços de e-mail não citam um site.
		for _, palavra := range strings.Fields(solto) {
			if strings.Contains(palavra, "@") {
				continue
			}
			for _, h := range reHost.FindAllString(palavra, -1) {
				if strings.EqualFold(h, host) {
					return true
				}
			}
		}
	}
	return false
}

func citaURL(textos []string, alvo string) bool {
	alvo = canonica(alvo)
	for _, t := range textos {
		for _, u := range URLs(t) {
			if canonica(u) == alvo {
				return true
			}
		}
	}
	return false
}

// canonica ignora diferenças que não mudam o destino.
func canonica(bruta string) string {
	u, err := url.Parse(strings.TrimSpace(bruta))
	if err != nil {
		return bruta
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment, u.RawFragment = "", ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}

// Avaliar aplica as regras a uma chamada.
func Avaliar(p Pedido) Decisao {
	if !slices.Contains(permissoes[p.Tipo], p.Ferramenta) {
		return decisao(Bloquear, "a ferramenta "+p.Ferramenta+" não faz parte das permissões desta atividade")
	}
	if p.Ferramenta == PesquisarWeb {
		return avaliarConsulta(p)
	}
	if p.Ferramenta != BuscarURL {
		return decisao(Permitir)
	}
	u, err := url.Parse(strings.TrimSpace(p.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return decisao(Bloquear, "endereço inválido")
	}
	if u.User != nil {
		return decisao(Bloquear, "credenciais no endereço não são aceitas")
	}
	// O endereço já existia no mecanismo de busca: o modelo não o montou.
	alvo := canonica(p.URL)
	achado := slices.ContainsFunc(p.Achados, func(a string) bool { return canonica(a) == alvo })
	if porta := u.Port(); porta != "" && porta != "80" && porta != "443" {
		if !achado && !citaURL(p.TextosUsuario, p.URL) {
			return decisao(Bloquear, "porta fora do padrão em endereço que você não digitou")
		}
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	liberado := slices.ContainsFunc(p.HostsLiberados, func(h string) bool { return strings.EqualFold(h, host) })
	if !liberado && (net.ParseIP(host) != nil || host == "localhost" || strings.HasSuffix(host, ".localhost")) {
		return decisao(Bloquear, "endereços IP e locais não são acessados pela pesquisa")
	}
	if citaURL(p.TextosUsuario, p.URL) {
		return decisao(Permitir, "endereço digitado por você")
	}
	if liberado {
		return decisao(Permitir, "host liberado nos ajustes")
	}
	// Um achado não pode carregar dados seus.
	if achado {
		return decisao(Permitir, "resultado de uma pesquisa desta atividade")
	}
	doUsuario := citaHost(p.TextosUsuario, host)
	if len(p.TextosExternos) == 0 {
		if doUsuario {
			return decisao(Permitir, "host citado por você")
		}
		return decisao(Confirmar, "o host "+host+" não foi citado por você nem está liberado")
	}
	// Há conteúdo externo no contexto: o modelo pode estar seguindo ordens dele.
	if doUsuario {
		return decisao(Confirmar, "você citou o host, mas não este endereço, e há conteúdo externo na conversa")
	}
	motivo := "o host " + host + " não foi citado por você nem está liberado"
	if citaHost(p.TextosExternos, host) {
		motivo = "o host " + host + " só apareceu em conteúdo externo"
	}
	return decisao(Bloquear, motivo)
}
