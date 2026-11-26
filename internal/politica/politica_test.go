package politica

import (
	"slices"
	"strings"
	"testing"
)

func TestURLs(t *testing.T) {
	casos := map[string][]string{
		"veja http://a.com/x.":                           {"http://a.com/x"},
		"(https://a.com/x_(y)) fim":                      {"https://a.com/x_(y)"},
		"dois: http://a.com, HTTPS://b.org/?q=1!?":       {"http://a.com", "HTTPS://b.org/?q=1"},
		"<http://a.com> \"http://b.com\" 'http://c.com'": {"http://a.com", "http://b.com", "http://c.com"},
		"sem endereço":                                   nil,
	}
	for entrada, quer := range casos {
		if teve := URLs(entrada); !slices.Equal(teve, quer) {
			t.Errorf("URLs(%q) = %q, quero %q", entrada, teve, quer)
		}
	}
}

func TestAvaliar(t *testing.T) {
	liberados := []string{"liberado.org", "127.0.0.1"}
	externo := []string{"baixe de http://visto.com/segredo"}
	casos := []struct {
		nome     string
		url      string
		usuario  []string
		externos []string
		quer     Veredito
	}{
		{"inválida", "::", nil, nil, Bloquear},
		{"sem host", "http:///x", nil, nil, Bloquear},
		{"outro esquema", "ftp://liberado.org", nil, nil, Bloquear},
		{"credenciais", "http://u:s@liberado.org", nil, nil, Bloquear},
		{"porta estranha", "http://liberado.org:8080/", nil, nil, Bloquear},
		{"porta estranha digitada", "http://liberado.org:8080/", []string{"abra http://liberado.org:8080/"}, nil, Permitir},
		{"porta padrão", "https://liberado.org:443/a", nil, externo, Permitir},
		{"IP não liberado", "http://10.0.0.1/", []string{"http://10.0.0.1/"}, nil, Bloquear},
		{"localhost", "http://localhost/", nil, nil, Bloquear},
		{"subdomínio de localhost", "http://x.localhost/", nil, nil, Bloquear},
		{"IP liberado nos ajustes", "http://127.0.0.1/x", nil, externo, Permitir},
		{"digitado", "http://novo.com/a#parte", []string{"leia HTTP://NOVO.com/a"}, externo, Permitir},
		{"raiz digitada", "http://novo.com/", []string{"leia http://novo.com"}, externo, Permitir},
		{"liberado sem diferenciar caixa", "http://LIBERADO.org./a", nil, externo, Permitir},
		{"host citado, contexto limpo", "http://novo.com/b", []string{"veja o site novo.com"}, nil, Permitir},
		{"host citado em URL, contexto limpo", "http://novo.com/b", []string{"http://novo.com/a"}, nil, Permitir},
		{"inventado, contexto limpo", "http://novo.com/b", []string{"oi"}, nil, Confirmar},
		{"citado, contexto contaminado", "http://novo.com/?d=1", []string{"http://novo.com/a"}, externo, Confirmar},
		{"inventado, contaminado", "http://outro.com/", []string{"oi"}, externo, Bloquear},
		{"visto fora", "http://visto.com/segredo", []string{"oi"}, externo, Bloquear},
		{"host dentro do caminho não conta", "http://mau.com/", []string{"http://liberado.org/mau.com"}, externo, Bloquear},
		{"e-mail do usuário não cita o site", "http://empresa.com/", []string{"escreva para eu@empresa.com"}, externo, Bloquear},
		{"sufixo não é o host", "http://novo.com/", []string{"veja o super-novo.com.br"}, externo, Bloquear},
	}
	for _, c := range casos {
		d := Avaliar(Pedido{Tipo: Conversa, Ferramenta: BuscarURL, URL: c.url, TextosUsuario: c.usuario,
			TextosExternos: c.externos, HostsLiberados: liberados})
		if d.Veredito != c.quer {
			t.Errorf("%s: %s %v, quero %s", c.nome, d.Veredito, d.Motivos, c.quer)
		}
	}
	d := Avaliar(Pedido{Tipo: Conversa, Ferramenta: BuscarURL, URL: "http://visto.com/", TextosExternos: externo})
	if !strings.Contains(d.Motivos[0], "conteúdo externo") {
		t.Errorf("motivo: %v", d.Motivos)
	}
}

func TestPermissoesPorAtividade(t *testing.T) {
	proibidas := map[string][]string{
		Triagem:   {BuscarURL, BuscarAcervo, MostrarTela, "enviar_email"},
		Conversa:  {EmailBuscar, EmailLer, EmailEtiquetar},
		Relatorio: {LerDocumento, EmailLer},
		Estudo:    {BuscarURL},
		"outra":   {BuscarAcervo},
	}
	for tipo, lista := range proibidas {
		for _, f := range lista {
			if Avaliar(Pedido{Tipo: tipo, Ferramenta: f}).Veredito != Bloquear {
				t.Errorf("%s não deveria usar %s", tipo, f)
			}
		}
	}
	if Avaliar(Pedido{Tipo: Triagem, Ferramenta: EmailEtiquetar}).Veredito != Permitir {
		t.Error("a triagem etiqueta")
	}
	// A lista devolvida é uma cópia.
	p := Permissoes(Conversa)
	p[0] = "x"
	if Permissoes(Conversa)[0] == "x" {
		t.Error("permissões alteradas por fora")
	}
}
