package mcpimap_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/wkoch/Palantir/internal/ferramentas"
	"github.com/wkoch/Palantir/internal/mcpimap"
	"github.com/wkoch/Palantir/internal/proxy"
	"github.com/wkoch/Palantir/internal/simulador"
	"github.com/wkoch/Palantir/internal/testes"
)

func montar(t *testing.T, cfg *tls.Config, emails []simulador.Email) (*simulador.Correio, mcpimap.Opcoes) {
	t.Helper()
	c, err := simulador.NovoCorreio("127.0.0.1:0", emails, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Fechar() })
	p := &proxy.Proxy{Ferramenta: "imap", Fixas: func() []proxy.Regra {
		return []proxy.Regra{{Host: "127.0.0.1", Portas: []int{c.Porta()}, Local: true}}
	}}
	socket := filepath.Join(testes.Pasta(t), "s")
	if err := p.Escutar(socket); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Fechar() })
	return c, mcpimap.Opcoes{Servidor: "127.0.0.1", Porta: c.Porta(), Usuario: simulador.Usuario,
		Senha: simulador.Senha, Seguranca: "nenhum", Socket: socket}
}

func marcas(t *testing.T, porta int) map[uint32][]string {
	t.Helper()
	c, err := imapclient.DialInsecure("127.0.0.1:"+strconv.Itoa(porta), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Login(simulador.Usuario, simulador.Senha).Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Select("INBOX", &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		t.Fatal(err)
	}
	msgs, err := c.Fetch(imap.SeqSet{{Start: 1, Stop: 0}}, &imap.FetchOptions{Flags: true, UID: true}).Collect()
	if err != nil {
		t.Fatal(err)
	}
	m := map[uint32][]string{}
	for _, x := range msgs {
		for _, f := range x.Flags {
			m[uint32(x.UID)] = append(m[uint32(x.UID)], strings.ToLower(string(f)))
		}
	}
	return m
}

func TestBuscarLerEtiquetar(t *testing.T) {
	correio, o := montar(t, nil, simulador.Emails)
	ctx := context.Background()
	r, err := mcpimap.Buscar(ctx, o, mcpimap.EntradaBuscar{})
	if err != nil || len(r.Mensagens) != 5 {
		t.Fatal(r, err)
	}
	m := r.Mensagens[0]
	if m.UID != 1 || m.De != "Ana Souza" || m.Email != "ana@exemplo.test" || !strings.Contains(m.Assunto, "Reunião") ||
		m.Trecho != "Oi! Você pode confirmar a reunião de quinta às 14h?" || m.Data == "" {
		t.Errorf("%+v", m)
	}
	// Só as novas, e o limite fica com as mais recentes.
	r2, err := mcpimap.Buscar(ctx, o, mcpimap.EntradaBuscar{DesdeUID: 5, Validade: r.Validade})
	if err != nil || len(r2.Mensagens) != 0 {
		t.Fatal(r2, err)
	}
	if err := correio.Entregar(simulador.Email{De: "a@b.test", Assunto: "longa", Corpo: strings.Repeat("palavra ", 200)}); err != nil {
		t.Fatal(err)
	}
	r2, _ = mcpimap.Buscar(ctx, o, mcpimap.EntradaBuscar{DesdeUID: 4, Validade: r.Validade, Limite: 1})
	if len(r2.Mensagens) != 1 || r2.Mensagens[0].UID != 6 || len([]rune(r2.Mensagens[0].Trecho)) != 601 {
		t.Error(r2.Mensagens)
	}
	// Uma validade antiga recomeça do zero.
	r3, _ := mcpimap.Buscar(ctx, o, mcpimap.EntradaBuscar{DesdeUID: 5, Validade: r.Validade + 1, Limite: 100})
	if len(r3.Mensagens) != 6 {
		t.Error(len(r3.Mensagens))
	}

	lida, err := mcpimap.Ler(ctx, o, mcpimap.EntradaLer{UID: 2, Validade: r.Validade})
	if err != nil || !strings.Contains(lida.Texto, "R$ 320,00") || !strings.Contains(lida.Texto, "Assunto: Fatura") {
		t.Fatal(lida, err)
	}
	if _, err := mcpimap.Ler(ctx, o, mcpimap.EntradaLer{UID: 99, Validade: r.Validade}); err == nil {
		t.Error("mensagem ausente")
	}
	if _, err := mcpimap.Ler(ctx, o, mcpimap.EntradaLer{UID: 1, Validade: r.Validade + 1}); err == nil {
		t.Error("validade errada")
	}
	// Ler e buscar não marcam como lida.
	for uid, f := range marcas(t, o.Porta) {
		if slices.Contains(f, `\seen`) {
			t.Error("mensagem marcada como lida", uid)
		}
	}

	et := func(marcas ...mcpimap.Marca) (mcpimap.SaidaEtiquetar, error) {
		return mcpimap.Etiquetar(ctx, o, mcpimap.EntradaEtiquetar{Validade: r.Validade, Marcas: marcas})
	}
	s, err := et(mcpimap.Marca{UID: 1, Classe: "urgente"}, mcpimap.Marca{UID: 2, Classe: "lixo"}, mcpimap.Marca{UID: 3, Classe: "lixo"})
	if err != nil || s.Marcadas != 3 {
		t.Fatal(s, err)
	}
	if _, err := et(mcpimap.Marca{UID: 1, Classe: "responder"}); err != nil {
		t.Fatal(err)
	}
	m2 := marcas(t, o.Porta)
	if !slices.Equal(m2[1], []string{"palantir_responder"}) || !slices.Equal(m2[2], []string{"palantir_lixo"}) {
		t.Error(m2)
	}
	for _, ruim := range []mcpimap.Marca{{UID: 1, Classe: `\Deleted`}, {UID: 0, Classe: "lixo"}} {
		if _, err := et(ruim); err == nil {
			t.Error("aceitou", ruim)
		}
	}
	if _, err := mcpimap.Etiquetar(ctx, o, mcpimap.EntradaEtiquetar{Validade: r.Validade + 1}); err == nil {
		t.Error("validade errada")
	}
}

func TestCaixaVaziaEErrosDeConexao(t *testing.T) {
	_, o := montar(t, nil, []simulador.Email{})
	ctx := context.Background()
	r, err := mcpimap.Buscar(ctx, o, mcpimap.EntradaBuscar{})
	if err != nil || len(r.Mensagens) != 0 || r.Mensagens == nil {
		t.Fatal(r, err)
	}
	casos := map[string]func(*mcpimap.Opcoes){
		"recusou o login":  func(o *mcpimap.Opcoes) { o.Senha = "errada" },
		"abrir a caixa":    func(o *mcpimap.Opcoes) { o.Caixa = "Inexistente" },
		"recusou 10.0.0.1": func(o *mcpimap.Opcoes) { o.Servidor = "10.0.0.1" },
		"proxy de saída":   func(o *mcpimap.Opcoes) { o.Socket = "" },
		"desconhecido":     func(o *mcpimap.Opcoes) { o.Seguranca = "ssl" },
		"conexão segura":   func(o *mcpimap.Opcoes) { o.Seguranca = "tls" },
		"STARTTLS":         func(o *mcpimap.Opcoes) { o.Seguranca = "starttls" },
	}
	for quer, mudar := range casos {
		x := o
		mudar(&x)
		_, err1 := mcpimap.Buscar(ctx, x, mcpimap.EntradaBuscar{})
		_, err2 := mcpimap.Ler(ctx, x, mcpimap.EntradaLer{UID: 1})
		_, err3 := mcpimap.Etiquetar(ctx, x, mcpimap.EntradaEtiquetar{})
		for _, err := range []error{err1, err2, err3} {
			if err == nil || !strings.Contains(err.Error(), quer) {
				t.Errorf("%s: %v", quer, err)
			}
		}
	}
}

func TestTLSEStartTLS(t *testing.T) {
	// Aproveita o certificado de teste do httptest, válido para 127.0.0.1.
	s := httptest.NewTLSServer(nil)
	cert := s.TLS.Certificates[0]
	raizes := x509.NewCertPool()
	raizes.AddCert(s.Certificate())
	s.Close()
	_, o := montar(t, &tls.Config{Certificates: []tls.Certificate{cert}}, simulador.Emails)
	o.Seguranca = "starttls"
	// Sem confiar no certificado, a conexão é recusada.
	if _, err := mcpimap.Buscar(context.Background(), o, mcpimap.EntradaBuscar{}); err == nil {
		t.Fatal("certificado desconhecido aceito")
	}
	o.TLS = &tls.Config{RootCAs: raizes}
	r, err := mcpimap.Buscar(context.Background(), o, mcpimap.EntradaBuscar{})
	if err != nil || len(r.Mensagens) != 5 {
		t.Fatal(r, err)
	}
}

func TestDoAmbienteEServidor(t *testing.T) {
	env := map[string]string{mcpimap.EnvPorta: "x"}
	obter := func(n string) string { return env[n] }
	if _, err := mcpimap.DoAmbiente(obter, "s"); err == nil {
		t.Error("porta inválida")
	}
	env[mcpimap.EnvPorta] = "993"
	if _, err := mcpimap.DoAmbiente(obter, "s"); err == nil {
		t.Error("incompleta")
	}
	_, o := montar(t, nil, simulador.Emails)
	env = map[string]string{mcpimap.EnvServidor: o.Servidor, mcpimap.EnvPorta: strconv.Itoa(o.Porta),
		mcpimap.EnvUsuario: o.Usuario, mcpimap.EnvSenha: o.Senha, mcpimap.EnvSeguranca: "nenhum", mcpimap.EnvCaixa: "INBOX"}
	lidas, err := mcpimap.DoAmbiente(obter, o.Socket)
	if err != nil {
		t.Fatal(err)
	}
	doServidor, doCliente := transportes()
	srv := mcpimap.Novo(lidas)
	if _, err := srv.Connect(context.Background(), doServidor, nil); err != nil {
		t.Fatal(err)
	}
	s, err := ferramentas.Conectar(context.Background(), doCliente)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	var r mcpimap.SaidaBuscar
	if err := s.Chamar(ctx, "buscar", mcpimap.EntradaBuscar{Limite: 2}, &r); err != nil || len(r.Mensagens) != 2 {
		t.Fatal(r, err)
	}
	if err := s.Chamar(ctx, "ler", mcpimap.EntradaLer{UID: 5, Validade: r.Validade}, nil); err != nil {
		t.Error(err)
	}
	if err := s.Chamar(ctx, "etiquetar", mcpimap.EntradaEtiquetar{Validade: r.Validade,
		Marcas: []mcpimap.Marca{{UID: 5, Classe: "lixo"}}}, nil); err != nil {
		t.Error(err)
	}
	// Não existe ferramenta para apagar ou enviar.
	for _, nome := range []string{"apagar", "enviar", "mover"} {
		if s.Chamar(ctx, nome, struct{}{}, nil) == nil {
			t.Error("ferramenta", nome, "existe")
		}
	}
}
