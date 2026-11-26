// Package mcpimap é o servidor MCP de e-mail. Ele só lê mensagens e grava as
// etiquetas do Palantír: não há ferramenta para enviar, mover ou apagar.
package mcpimap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wkoch/Palantir/internal/extrator"
	"github.com/wkoch/Palantir/internal/proxy"
)

// Variáveis de ambiente lidas pela ferramenta.
const (
	EnvServidor  = "PALANTIR_IMAP_SERVIDOR"
	EnvPorta     = "PALANTIR_IMAP_PORTA"
	EnvUsuario   = "PALANTIR_IMAP_USUARIO"
	EnvSenha     = "PALANTIR_IMAP_SENHA"
	EnvCaixa     = "PALANTIR_IMAP_CAIXA"
	EnvSeguranca = "PALANTIR_IMAP_SEGURANCA"
)

// PrefixoEtiqueta identifica as etiquetas que a ferramenta pode alterar.
const PrefixoEtiqueta = "Palantir_"

// Classes são as classificações que viram etiquetas.
var Classes = []string{"urgente", "responder", "ler_depois", "lixo"}

// Opcoes configura a conexão.
type Opcoes struct {
	Servidor  string
	Porta     int
	Usuario   string
	Senha     string
	Caixa     string
	Seguranca string // tls, starttls ou nenhum
	Socket    string
	TLS       *tls.Config
}

// DoAmbiente monta as opções a partir das variáveis de ambiente.
func DoAmbiente(obter func(string) string, socket string) (Opcoes, error) {
	porta, err := strconv.Atoi(obter(EnvPorta))
	if err != nil {
		return Opcoes{}, fmt.Errorf("%s inválida: %w", EnvPorta, err)
	}
	o := Opcoes{Servidor: obter(EnvServidor), Porta: porta, Usuario: obter(EnvUsuario),
		Senha: obter(EnvSenha), Caixa: obter(EnvCaixa), Seguranca: obter(EnvSeguranca), Socket: socket}
	if o.Servidor == "" || o.Usuario == "" || o.Senha == "" {
		return o, errors.New("configuração de e-mail incompleta")
	}
	return o, nil
}

// Mensagem é o resumo de um e-mail.
type Mensagem struct {
	UID     uint32 `json:"uid"`
	De      string `json:"de"`
	Email   string `json:"email"`
	Assunto string `json:"assunto"`
	Data    string `json:"data"`
	Trecho  string `json:"trecho"`
}

// EntradaBuscar pede as mensagens posteriores a um UID.
type EntradaBuscar struct {
	DesdeUID uint32 `json:"desde_uid" jsonschema:"último UID já visto, ou 0"`
	Validade uint32 `json:"validade" jsonschema:"UIDVALIDITY conhecida, ou 0"`
	Limite   int    `json:"limite" jsonschema:"quantidade máxima, das mais recentes"`
}

// SaidaBuscar devolve as mensagens em ordem de UID.
type SaidaBuscar struct {
	Validade  uint32     `json:"validade"`
	Mensagens []Mensagem `json:"mensagens"`
}

// EntradaLer pede o texto completo de uma mensagem.
type EntradaLer struct {
	UID      uint32 `json:"uid"`
	Validade uint32 `json:"validade"`
}

// SaidaLer é o texto de uma mensagem.
type SaidaLer struct {
	Texto string `json:"texto"`
}

// Marca associa uma mensagem a uma classe.
type Marca struct {
	UID    uint32 `json:"uid"`
	Classe string `json:"classe"`
}

// EntradaEtiquetar grava as etiquetas.
type EntradaEtiquetar struct {
	Validade uint32  `json:"validade"`
	Marcas   []Marca `json:"marcas"`
}

// SaidaEtiquetar conta as mensagens marcadas.
type SaidaEtiquetar struct {
	Marcadas int `json:"marcadas"`
}

type conexao struct {
	c        *imapclient.Client
	validade uint32
}

func abrir(ctx context.Context, o Opcoes, somenteLeitura bool) (*conexao, error) {
	endereco := net.JoinHostPort(o.Servidor, strconv.Itoa(o.Porta))
	bruta, err := proxy.Discador{Socket: o.Socket}.DialContext(ctx, "tcp", endereco)
	if err != nil {
		return nil, err
	}
	prazo, ok := ctx.Deadline()
	if !ok {
		prazo = time.Now().Add(2 * time.Minute)
	}
	if err := bruta.SetDeadline(prazo); err != nil {
		bruta.Close()
		return nil, err
	}
	cfg := &tls.Config{}
	if o.TLS != nil {
		cfg = o.TLS.Clone()
	}
	cfg.ServerName = o.Servidor
	opts := &imapclient.Options{TLSConfig: cfg,
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader}}
	var c *imapclient.Client
	switch o.Seguranca {
	case "tls", "":
		t := tls.Client(bruta, cfg)
		if err := t.HandshakeContext(ctx); err != nil {
			bruta.Close()
			return nil, fmt.Errorf("conexão segura com %s: %w", o.Servidor, err)
		}
		c = imapclient.New(t, opts)
	case "starttls":
		if c, err = imapclient.NewStartTLS(bruta, opts); err != nil {
			bruta.Close()
			return nil, fmt.Errorf("STARTTLS com %s: %w", o.Servidor, err)
		}
	case "nenhum":
		if o.Servidor != "localhost" && !net.ParseIP(o.Servidor).IsLoopback() {
			bruta.Close()
			return nil, errors.New("conexão sem TLS só é aceita em localhost")
		}
		c = imapclient.New(bruta, opts)
	default:
		bruta.Close()
		return nil, fmt.Errorf("modo de segurança desconhecido: %q", o.Seguranca)
	}
	if err := c.Login(o.Usuario, o.Senha).Wait(); err != nil {
		c.Close()
		return nil, fmt.Errorf("o servidor recusou o login: %w", err)
	}
	caixa := o.Caixa
	if caixa == "" {
		caixa = "INBOX"
	}
	// EXAMINE impede qualquer alteração nas operações de leitura.
	sel, err := c.Select(caixa, &imap.SelectOptions{ReadOnly: somenteLeitura}).Wait()
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("abrir a caixa %q: %w", caixa, err)
	}
	return &conexao{c: c, validade: sel.UIDValidity}, nil
}

func (x *conexao) fechar() {
	_ = x.c.Logout().Wait()
	_ = x.c.Close()
}

const (
	maxTrecho = 600
	maxBruto  = 32 << 10
	maxCheio  = 2 << 20
)

// Buscar devolve as mensagens novas sem marcá-las como lidas.
func Buscar(ctx context.Context, o Opcoes, in EntradaBuscar) (SaidaBuscar, error) {
	x, err := abrir(ctx, o, true)
	if err != nil {
		return SaidaBuscar{}, err
	}
	defer x.fechar()
	saida := SaidaBuscar{Validade: x.validade, Mensagens: []Mensagem{}}
	desde := in.DesdeUID
	if in.Validade != 0 && in.Validade != x.validade {
		desde = 0 // os UIDs antigos não valem mais
	}
	criterio := &imap.SearchCriteria{}
	if desde > 0 {
		criterio.UID = []imap.UIDSet{{{Start: imap.UID(desde + 1), Stop: 0}}}
	}
	achados, err := x.c.UIDSearch(criterio, nil).Wait()
	if err != nil {
		return saida, fmt.Errorf("procurar mensagens: %w", err)
	}
	uids := achados.AllUIDs()
	// "N:*" inclui a última mensagem mesmo quando seu UID é menor que N.
	uids = slices.DeleteFunc(uids, func(u imap.UID) bool { return uint32(u) <= desde })
	slices.Sort(uids)
	limite := in.Limite
	if limite < 1 || limite > 500 {
		limite = 50
	}
	if len(uids) > limite {
		uids = uids[len(uids)-limite:]
	}
	if len(uids) == 0 {
		return saida, nil
	}
	secao := &imap.FetchItemBodySection{Peek: true, Partial: &imap.SectionPartial{Size: maxBruto}}
	msgs, err := x.c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{
		UID: true, Envelope: true, InternalDate: true, BodySection: []*imap.FetchItemBodySection{secao},
	}).Collect()
	if err != nil {
		return saida, fmt.Errorf("baixar mensagens: %w", err)
	}
	for _, m := range msgs {
		r := Mensagem{UID: uint32(m.UID), Data: m.InternalDate.UTC().Format(time.RFC3339)}
		if e := m.Envelope; e != nil {
			r.Assunto = e.Subject
			if !e.Date.IsZero() {
				r.Data = e.Date.UTC().Format(time.RFC3339)
			}
			if len(e.From) > 0 {
				r.De, r.Email = e.From[0].Name, strings.ToLower(e.From[0].Addr())
			}
		}
		r.Trecho = trecho(m.FindBodySection(secao))
		saida.Mensagens = append(saida.Mensagens, r)
	}
	slices.SortFunc(saida.Mensagens, func(a, b Mensagem) int { return int(a.UID) - int(b.UID) })
	return saida, nil
}

// corpoDaMensagem separa o texto dos cabeçalhos acrescentados na conversão.
func corpoDaMensagem(bruto []byte) string {
	t, err := extrator.TextoDeEmail(bruto)
	if err != nil {
		return ""
	}
	return t
}

func trecho(bruto []byte) string {
	t := corpoDaMensagem(bruto)
	if _, corpo, ok := strings.Cut(t, "\n\n"); ok {
		t = corpo
	} else if strings.HasPrefix(t, "De: ") || strings.HasPrefix(t, "Assunto: ") {
		t = ""
	}
	t = strings.Join(strings.Fields(t), " ")
	if r := []rune(t); len(r) > maxTrecho {
		t = string(r[:maxTrecho]) + "…"
	}
	return t
}

func conferirValidade(pedida, atual uint32) error {
	if pedida != atual {
		return fmt.Errorf("a caixa mudou no servidor (UIDVALIDITY %d, esperada %d); busque novamente", atual, pedida)
	}
	return nil
}

// Ler devolve o texto de uma mensagem sem marcá-la como lida.
func Ler(ctx context.Context, o Opcoes, in EntradaLer) (SaidaLer, error) {
	x, err := abrir(ctx, o, true)
	if err != nil {
		return SaidaLer{}, err
	}
	defer x.fechar()
	if err := conferirValidade(in.Validade, x.validade); err != nil {
		return SaidaLer{}, err
	}
	secao := &imap.FetchItemBodySection{Peek: true, Partial: &imap.SectionPartial{Size: maxCheio}}
	msgs, err := x.c.Fetch(imap.UIDSetNum(imap.UID(in.UID)), &imap.FetchOptions{
		UID: true, BodySection: []*imap.FetchItemBodySection{secao},
	}).Collect()
	if err != nil {
		return SaidaLer{}, err
	}
	if len(msgs) == 0 {
		return SaidaLer{}, fmt.Errorf("a mensagem %d não está mais na caixa", in.UID)
	}
	return SaidaLer{Texto: corpoDaMensagem(msgs[0].FindBodySection(secao))}, nil
}

// Etiquetar grava a etiqueta de cada classe, removendo as outras do Palantír.
func Etiquetar(ctx context.Context, o Opcoes, in EntradaEtiquetar) (SaidaEtiquetar, error) {
	porClasse := map[string][]imap.UID{}
	for _, m := range in.Marcas {
		if !slices.Contains(Classes, m.Classe) {
			return SaidaEtiquetar{}, fmt.Errorf("classe não permitida: %q", m.Classe)
		}
		if m.UID == 0 {
			return SaidaEtiquetar{}, errors.New("UID inválido")
		}
		porClasse[m.Classe] = append(porClasse[m.Classe], imap.UID(m.UID))
	}
	x, err := abrir(ctx, o, false)
	if err != nil {
		return SaidaEtiquetar{}, err
	}
	defer x.fechar()
	if err := conferirValidade(in.Validade, x.validade); err != nil {
		return SaidaEtiquetar{}, err
	}
	var saida SaidaEtiquetar
	for _, classe := range Classes {
		uids := porClasse[classe]
		if len(uids) == 0 {
			continue
		}
		var outras []imap.Flag
		for _, c := range Classes {
			if c != classe {
				outras = append(outras, imap.Flag(PrefixoEtiqueta+c))
			}
		}
		conjunto := imap.UIDSetNum(uids...)
		for _, s := range []imap.StoreFlags{
			{Op: imap.StoreFlagsDel, Silent: true, Flags: outras},
			{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.Flag(PrefixoEtiqueta + classe)}},
		} {
			if err := x.c.Store(conjunto, &s, nil).Close(); err != nil {
				return saida, fmt.Errorf("gravar etiquetas: %w", err)
			}
		}
		saida.Marcadas += len(uids)
	}
	return saida, nil
}

// Novo cria o servidor MCP.
func Novo(o Opcoes) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "palantir-imap", Version: "1"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "buscar", Description: "Lista as mensagens novas da caixa."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in EntradaBuscar) (*mcp.CallToolResult, SaidaBuscar, error) {
			out, err := Buscar(ctx, o, in)
			if out.Mensagens == nil {
				out.Mensagens = []Mensagem{}
			}
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "ler", Description: "Devolve o texto de uma mensagem."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in EntradaLer) (*mcp.CallToolResult, SaidaLer, error) {
			out, err := Ler(ctx, o, in)
			return nil, out, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "etiquetar", Description: "Grava a etiqueta de classificação nas mensagens."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in EntradaEtiquetar) (*mcp.CallToolResult, SaidaEtiquetar, error) {
			out, err := Etiquetar(ctx, o, in)
			return nil, out, err
		})
	return s
}
