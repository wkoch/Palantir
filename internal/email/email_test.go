package email_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/email"
	"github.com/wkoch/Palantir/internal/mcpimap"
	"github.com/wkoch/Palantir/internal/segredos"
	"github.com/wkoch/Palantir/internal/simulador"
	"github.com/wkoch/Palantir/internal/testes"
)

func dados(t *testing.T, x atividades.Atividade) email.Dados { return testes.Dados[email.Dados](t, x) }

func achar(t *testing.T, d email.Dados, uid uint32) email.Mensagem {
	t.Helper()
	for _, m := range d.Mensagens {
		if m.UID == uid {
			return m
		}
	}
	t.Fatal("mensagem não encontrada", uid)
	return email.Mensagem{}
}

func TestTriagemCorrecoesEPreferencias(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	x := s.Criar("triagem", "", "")
	d := dados(t, x)
	if x.Estado != atividades.Pronta || len(d.Mensagens) != 5 || d.UltimoUID != 5 || !strings.Contains(x.Aviso, "1 urgente") {
		t.Fatal(x.Estado, x.Aviso, d)
	}
	if m := achar(t, d, 2); m.Classe != "urgente" || m.Origem != email.PorModelo || !m.Etiquetada {
		t.Errorf("%+v", m)
	}
	_, tela, err := s.D.Gerente.Tela(s.Contexto, x)
	if err != nil {
		t.Fatal(err)
	}
	v := tela.(email.Visao)
	if v.Mensagens[0].Classe != "urgente" || v.Mensagens[4].Classe != "lixo" || !v.Etiqueta {
		t.Error(v.Mensagens)
	}
	// O decisor recebe uma mensagem por chamada; o modelo barato, sem
	// ferramentas, resume só o que pede atenção.
	if n := len(s.Modelo.Decisoes()); n != 5 {
		t.Error("uma decisão por mensagem:", n)
	}
	p := s.Modelo.Pedidos()
	if len(p) != 1 || len(p[0].Ferram) != 0 || p[0].Modelo != "teste/barato" || p[0].Esquema() != "resumos_email" {
		t.Fatal(len(p))
	}
	if m := achar(t, d, 2); m.Resumo == "" {
		t.Error("urgente sem resumo")
	}
	for _, m := range d.Mensagens {
		if m.Classe == "ler_depois" && m.Resumo != "" {
			t.Error("resumo desnecessário", m)
		}
	}

	r, x := s.Agir(x.ID, "abrir", `{"uid":2}`)
	if !strings.Contains(r.(mcpimap.SaidaLer).Texto, "R$ 320,00") {
		t.Error(r)
	}
	_, x = s.Agir(x.ID, "corrigir", `{"uid":4,"classe":"urgente"}`)
	d = dados(t, x)
	if m := achar(t, d, 4); m.Classe != "urgente" || m.Origem != email.PorUsuario || !m.Etiquetada {
		t.Errorf("%+v", m)
	}
	// Corrigir para a mesma classe não precisa etiquetar de novo.
	antes := x.AtualizadaEm
	_, x = s.Agir(x.ID, "corrigir", `{"uid":4,"classe":"urgente"}`)
	if x.Estado != atividades.Pronta || x.AtualizadaEm.Before(antes) {
		t.Error(x.Estado)
	}
	_, x = s.Agir(x.ID, "concluir", `{"uid":2,"concluida":true}`)
	if !achar(t, dados(t, x), 2).Concluida {
		t.Error("não concluiu")
	}

	// A preferência vale para as próximas, em código.
	if err := s.Correio.Entregar(simulador.Email{De: "Boletim <boletim@exemplo.test>", Assunto: "Novo boletim", Corpo: "x"}); err != nil {
		t.Fatal(err)
	}
	chamadas := len(s.Modelo.Pedidos()) + len(s.Modelo.Decisoes())
	_, x = s.Agir(x.ID, "atualizar", `{}`)
	if m := achar(t, dados(t, x), 6); m.Classe != "urgente" || m.Origem != email.PorRegra || !m.Etiquetada {
		t.Errorf("%+v", m)
	}
	if len(s.Modelo.Pedidos())+len(s.Modelo.Decisoes()) != chamadas {
		t.Error("o modelo foi chamado para o que a regra resolve")
	}
	// E entra como exemplo para as demais.
	s.Correio.Entregar(simulador.Email{De: "Outro <outro@exemplo.test>", Assunto: "pode ver?", Corpo: "x"})
	_, x = s.Agir(x.ID, "atualizar", `{}`)
	ultimo := s.Modelo.Decisoes()[len(s.Modelo.Decisoes())-1]
	if !strings.Contains(ultimo.Bruto, "boletim@exemplo.test") {
		t.Error("as correções não foram enviadas")
	}
	_, x = s.Agir(x.ID, "esquecer", `{"remetente":"boletim@exemplo.test"}`)
	_, tela, _ = s.D.Gerente.Tela(s.Contexto, x)
	if len(tela.(email.Visao).Preferencias) != 0 {
		t.Error("não esqueceu")
	}
	for _, ruim := range [][2]string{{"corrigir", `{"uid":99,"classe":"lixo"}`}, {"corrigir", `{"uid":1,"classe":"spam"}`},
		{"corrigir", `[]`}, {"esquecer", `[]`}, {"concluir", `[]`}, {"concluir", `{"uid":99}`}, {"abrir", `[]`},
		{"abrir", `{"uid":99}`}, {"apagar", `{}`}, {"responder", `{}`}} {
		if _, err := s.D.Gerente.Agir(s.Contexto, x.ID, ruim[0], []byte(ruim[1])); !errors.Is(err, atividades.ErrPedidoInvalido) {
			t.Error(ruim, err)
		}
	}
}

func TestSemModeloUsaRegrasEEspera(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{SemChave: true})
	if _, err := s.D.DB.Exec(`INSERT INTO preferencias_email VALUES ('ana@exemplo.test','responder','',1)`); err != nil {
		t.Fatal(err)
	}
	x := s.Criar("triagem", "", "")
	d := dados(t, x)
	if x.Estado != atividades.Esperando || len(d.Mensagens) != 5 {
		t.Fatal(x.Estado, x.Aviso)
	}
	if m := achar(t, d, 1); m.Classe != "responder" || !m.Etiquetada {
		t.Errorf("a regra vale sem modelo: %+v", m)
	}
	if m := achar(t, d, 2); m.Classe != "" {
		t.Error(m)
	}
	if got := email.Resumo(d.Mensagens); got != "1 para responder, 4 sem classificação." {
		t.Error(got)
	}
	// Com a chave, retomar classifica sem buscar de novo.
	if err := s.D.DefinirSegredo(s.Contexto, segredos.OpenRouter, testes.Chave); err != nil {
		t.Fatal(err)
	}
	s.D.Gerente.Mudar(s.Contexto, x.ID, atividades.Rodando)
	if x = s.Esperar(x.ID); x.Estado != atividades.Pronta || achar(t, dados(t, x), 2).Classe == "" {
		t.Error(x.Estado, x.Aviso)
	}
}

func TestMuitasMensagensEmLotesELimite(t *testing.T) {
	var emails []simulador.Email
	for i := range 320 {
		emails = append(emails, simulador.Email{De: fmt.Sprintf("p%d@exemplo.test", i), Assunto: fmt.Sprintf("Assunto %d", i), Corpo: "c"})
	}
	// Sem decisor, o modelo barato classifica em lotes.
	s := testes.Novo(t, testes.Opcoes{Emails: emails, Modelos: `decisoes = ""`})
	if err := s.D.Ajustar(config.Ajustes{IMAP: func() *config.IMAP {
		c := s.D.Config().IMAP
		c.Limite, c.Etiquetar = 500, false
		return &c
	}()}); err != nil {
		t.Fatal(err)
	}
	x := s.Criar("triagem", "", "")
	d := dados(t, x)
	if len(d.Mensagens) != 300 || d.Mensagens[0].UID != 21 || d.Mensagens[299].UID != 320 {
		t.Fatal(len(d.Mensagens))
	}
	if n := len(s.Modelo.Pedidos()); n != 20 {
		t.Error("esperava 20 lotes de 15:", n)
	}
	for _, m := range d.Mensagens {
		if m.Etiquetada || m.Classe == "" {
			t.Fatal(m)
		}
	}
}

func TestErrosDeConfiguracaoEServidor(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	if _, err := s.D.Gerente.Criar(s.Contexto, "triagem", "", "", []byte(`{"x":1}`)); !errors.Is(err, atividades.ErrPedidoInvalido) {
		t.Error(err)
	}
	if err := s.D.DefinirSegredo(s.Contexto, segredos.SenhaIMAP, "senha-errada-123"); err != nil {
		t.Fatal(err)
	}
	x := s.Criar("triagem", "", "")
	if x.Estado != atividades.Esperando || !strings.Contains(x.Aviso, "login") || strings.Contains(x.Aviso, "senha-errada-123") {
		t.Error(x.Estado, x.Aviso)
	}
	if err := s.D.DefinirSegredo(s.Contexto, segredos.SenhaIMAP, ""); err != nil {
		t.Fatal(err)
	}
	x = s.Criar("triagem", "", "")
	if x.Estado != atividades.Esperando || !strings.Contains(x.Aviso, "Ajustes") {
		t.Error(x.Aviso)
	}
	if got := email.Resumo(nil); got != "Nenhuma mensagem pendente." {
		t.Error(got)
	}
}

func TestMudancaDeValidadeRecomeca(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	x := s.Criar("triagem", "", "")
	if _, err := atividades.Modificar(s.Contexto, s.D.Armazem, x.ID, func(d *email.Dados) error {
		d.Validade, d.UltimoUID = d.Validade+1, 999
		d.Mensagens = []email.Mensagem{{Mensagem: mcpimap.Mensagem{UID: 999}, Classe: "lixo"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, x = s.Agir(x.ID, "atualizar", `{}`)
	if d := dados(t, x); len(d.Mensagens) != 5 || d.UltimoUID != 5 {
		t.Error(d)
	}
}

func TestDecisorEmDuvidaPassaAoModeloDeConversa(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	// Um texto que dá ordens vai para o lixo, mesmo com pouca confiança; as
	// demais dúvidas seguem para o modelo de conversa.
	s.Modelo.DefinirDecisao(func(p simulador.PedidoDeDecisao) (simulador.Resposta, bool) {
		assunto := p.Estado["email"].(map[string]any)["assunto"].(string)
		ordem := 0.01
		if strings.Contains(strings.ToLower(assunto), "fatura") {
			ordem = 0.99
		}
		return simulador.Resposta{JSON: map[string]any{
			"classe": simulador.Opcao("lixo", 0.4), "ordem": simulador.Sim(ordem)}}, true
	})
	x := s.Criar("triagem", "", "")
	var avisadas int
	for _, m := range dados(t, x).Mensagens {
		switch {
		case strings.HasPrefix(m.Resumo, "Atenção") && m.Classe == "lixo":
			avisadas++
		case m.Classe == "" || !strings.HasPrefix(m.Resumo, "Resumo: "):
			t.Errorf("a dúvida não passou ao modelo de conversa: %+v", m)
		}
	}
	p := s.Modelo.Pedidos()
	if avisadas == 0 || len(p) != 1 || p[0].Esquema() != "classificacao_email" {
		t.Fatal(avisadas, len(p))
	}
}

func TestDecisorRecusadoUsaOModeloDeConversa(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	s.Modelo.DefinirDecisao(func(simulador.PedidoDeDecisao) (simulador.Resposta, bool) {
		return simulador.Resposta{Status: 404, Texto: "sem esse modelo"}, true
	})
	x := s.Criar("triagem", "", "")
	if x.Estado != atividades.Pronta {
		t.Fatal(x.Estado, x.Aviso)
	}
	p := s.Modelo.Pedidos()
	if len(p) != 1 || p[0].Esquema() != "classificacao_email" {
		t.Fatal(len(p))
	}
	for _, m := range dados(t, x).Mensagens {
		if m.Classe == "" {
			t.Error(m)
		}
	}
}

func TestDecisorSemRedeNaoClassificaPorOutroCaminho(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	s.D.Roteador.Espera = time.Millisecond
	s.Modelo.DefinirDecisao(func(simulador.PedidoDeDecisao) (simulador.Resposta, bool) {
		return simulador.Resposta{Status: 503}, true
	})
	x := s.Criar("triagem", "", "")
	if x.Estado == atividades.Pronta || len(s.Modelo.Pedidos()) != 0 {
		t.Fatal(x.Estado, len(s.Modelo.Pedidos()))
	}
}
