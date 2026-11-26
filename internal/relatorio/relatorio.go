// Package relatorio é a atividade de relatório: planeja, pesquisa no acervo e
// na web e escreve seções em que toda afirmação tem fonte.
package relatorio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wkoch/Palantir/internal/acervo"
	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/capacidades"
	"github.com/wkoch/Palantir/internal/confirmacoes"
	"github.com/wkoch/Palantir/internal/diario"
	"github.com/wkoch/Palantir/internal/modelos"
	"github.com/wkoch/Palantir/internal/politica"
	"github.com/wkoch/Palantir/internal/proveniencia"
)

// Estados de uma seção.
const (
	SecaoPendente = "pendente"
	SecaoRascunho = "rascunho"
	SecaoAprovada = "aprovada"
)

// Fonte é um material consultado.
type Fonte struct {
	ID        string `json:"id"` // F1, F2...
	Tipo      string `json:"tipo"`
	Titulo    string `json:"titulo"`
	Trecho    int64  `json:"trecho,omitempty"`
	Documento int64  `json:"documento,omitempty"`
	Pagina    int    `json:"pagina,omitempty"`
	URL       string `json:"url,omitempty"`
	Texto     string `json:"texto"`
}

// Paragrafo é uma afirmação com as fontes que a sustentam.
type Paragrafo struct {
	Texto  string   `json:"texto"`
	Fontes []string `json:"fontes"`
}

// Comentario é um pedido de correção do usuário.
type Comentario struct {
	Texto    string    `json:"texto"`
	Quando   time.Time `json:"quando"`
	Atendido bool      `json:"atendido"`
}

// Secao é uma parte do relatório.
type Secao struct {
	ID          int          `json:"id"`
	Titulo      string       `json:"titulo"`
	Orientacao  string       `json:"orientacao"`
	Buscas      []string     `json:"buscas"`
	URLs        []string     `json:"urls"`
	Estado      string       `json:"estado"`
	Paragrafos  []Paragrafo  `json:"paragrafos"`
	Comentarios []Comentario `json:"comentarios"`
	Descartadas int          `json:"descartadas"`
	Aviso       string       `json:"aviso"`
	// Revisao aumenta a cada pedido, para descartar um texto escrito antes dele.
	Revisao int `json:"revisao"`
}

// Pergunta é uma dúvida que o usuário precisa responder antes do plano.
type Pergunta struct {
	Texto    string `json:"texto"`
	Resposta string `json:"resposta"`
}

// Versao registra uma versão aprovada guardada no acervo.
type Versao struct {
	Documento int64     `json:"documento"`
	Quando    time.Time `json:"quando"`
}

// Dados é o estado de um relatório.
type Dados struct {
	Profundo  bool       `json:"profundo"`
	Perguntas []Pergunta `json:"perguntas"`
	Planejado bool       `json:"planejado"`
	Secoes    []Secao    `json:"secoes"`
	Fontes    []Fonte    `json:"fontes"`
	// Liberadas são endereços que o usuário confirmou na tela.
	Liberadas []string `json:"liberadas"`
	Versoes   []Versao `json:"versoes"`
}

func (d *Dados) secao(id int) (*Secao, error) {
	for i := range d.Secoes {
		if d.Secoes[i].ID == id {
			return &d.Secoes[i], nil
		}
	}
	return nil, atividades.Invalido("a seção %d não existe", id)
}

// guardar acrescenta uma fonte, reaproveitando a que já existe.
func (d *Dados) guardar(f Fonte) string {
	for _, x := range d.Fontes {
		if x.Tipo == f.Tipo && x.Trecho == f.Trecho && x.URL == f.URL && x.Texto == f.Texto {
			return x.ID
		}
	}
	f.ID = fmt.Sprintf("F%d", len(d.Fontes)+1)
	d.Fontes = append(d.Fontes, f)
	return f.ID
}

// Buscador é a parte do acervo usada pelo relatório.
type Buscador interface {
	Buscar(ctx context.Context, atividade int64, consulta string, limite int) ([]acervo.Achado, error)
	Importar(ctx context.Context, r io.Reader, nome, origem string) (acervo.Documento, bool, error)
}

// Executor implementa atividades.Executor.
type Executor struct {
	Armazem        *atividades.Armazem
	Modelos        modelos.Completador
	Acervo         Buscador
	Web            *capacidades.Web
	Confirmacoes   *confirmacoes.Fila
	Diario         *diario.Diario
	HostsLiberados func() []string
}

// Iniciar implementa atividades.Executor.
func (e *Executor) Iniciar(_ context.Context, x *atividades.Atividade, opcoes json.RawMessage) error {
	var d Dados
	if len(opcoes) > 0 {
		o, err := atividades.Decodificar[struct {
			Profundo bool `json:"profundo"`
		}](opcoes)
		if err != nil {
			return err
		}
		d.Profundo = o.Profundo
	}
	if strings.TrimSpace(x.Objetivo) == "" {
		return atividades.Invalido("descreva o que o relatório deve responder")
	}
	var err error
	x.Dados, err = json.Marshal(d)
	return err
}

func (e *Executor) ler(ctx context.Context, id int64) (atividades.Atividade, Dados, error) {
	x, err := e.Armazem.Obter(ctx, id)
	if err != nil {
		return x, Dados{}, err
	}
	d, err := atividades.Dados[Dados](x)
	return x, d, err
}

func (e *Executor) mudar(ctx context.Context, id int64, f func(*Dados) error) (Dados, error) {
	return atividades.Modificar(ctx, e.Armazem, id, f)
}

// textosDoUsuario é tudo o que o usuário escreveu: a fonte de autorização.
func textosDoUsuario(x atividades.Atividade, d Dados) []string {
	t := []string{x.Objetivo}
	for _, p := range d.Perguntas {
		t = append(t, p.Resposta)
	}
	for _, s := range d.Secoes {
		for _, c := range s.Comentarios {
			t = append(t, c.Texto)
		}
	}
	return append(t, d.Liberadas...)
}

func pedidoDoUsuario(x atividades.Atividade, d Dados) string {
	var b strings.Builder
	b.WriteString("Pedido: " + x.Objetivo)
	for _, p := range d.Perguntas {
		if p.Resposta != "" {
			b.WriteString("\nPergunta: " + p.Texto + "\nResposta do usuário: " + p.Resposta)
		}
	}
	return b.String()
}

// Executar implementa atividades.Executor.
func (e *Executor) Executar(ctx context.Context, id int64) (atividades.Resultado, error) {
	var nada atividades.Resultado
	x, d, err := e.ler(ctx, id)
	if err != nil {
		return nada, err
	}
	if !d.Planejado {
		for _, p := range d.Perguntas {
			if p.Resposta == "" {
				return atividades.Resultado{Estado: atividades.Esperando, Aviso: "Responda às perguntas para continuar."}, nil
			}
		}
		if d, err = e.planejar(ctx, x, d); err != nil {
			return nada, err
		}
		if !d.Planejado {
			return atividades.Resultado{Estado: atividades.Esperando, Aviso: "Responda às perguntas para continuar."}, nil
		}
	}
	for {
		if x, d, err = e.ler(ctx, id); err != nil {
			return nada, err
		}
		i := slices.IndexFunc(d.Secoes, func(s Secao) bool { return s.Estado == SecaoPendente })
		if i < 0 {
			break
		}
		if err := e.escreverSecao(ctx, x, d, d.Secoes[i].ID); err != nil {
			return nada, err
		}
	}
	pendentes, err := e.Confirmacoes.Pendentes(ctx)
	if err != nil {
		return nada, err
	}
	if slices.ContainsFunc(pendentes, func(c confirmacoes.Confirmacao) bool { return c.Atividade == id }) {
		return atividades.Resultado{Estado: atividades.Esperando,
			Aviso: "O rascunho está pronto; há fontes aguardando a sua confirmação."}, nil
	}
	return atividades.Resultado{Estado: atividades.Pronta, Aviso: "Relatório pronto para revisão."}, nil
}

func (e *Executor) planejar(ctx context.Context, x atividades.Atividade, d Dados) (Dados, error) {
	podePerguntar := len(d.Perguntas) == 0
	instrucao := sistemaPlano
	if !podePerguntar {
		instrucao += "\nNão faça novas perguntas."
	}
	var plano struct {
		Perguntas []string `json:"perguntas"`
		Secoes    []struct {
			Titulo     string   `json:"titulo"`
			Orientacao string   `json:"orientacao"`
			Buscas     []string `json:"buscas"`
			URLs       []string `json:"urls"`
		} `json:"secoes"`
	}
	// O plano só vê o que o usuário escreveu.
	err := modelos.CompletarJSON(ctx, e.Modelos, modelos.Pedido{Nivel: modelos.Principal, Atividade: x.ID,
		Finalidade: politica.Relatorio, Esquema: &modelos.Esquema{Nome: "plano_relatorio", Definicao: esquemaPlano},
		Mensagens: []modelos.Mensagem{
			{Papel: modelos.PapelSistema, Conteudo: instrucao},
			{Papel: modelos.PapelUsuario, Conteudo: pedidoDoUsuario(x, d)},
		}}, &plano)
	if err != nil {
		return d, err
	}
	return e.mudar(ctx, x.ID, func(d *Dados) error {
		if podePerguntar && len(plano.Perguntas) > 0 {
			for i, p := range plano.Perguntas {
				if i < 4 && strings.TrimSpace(p) != "" {
					d.Perguntas = append(d.Perguntas, Pergunta{Texto: p})
				}
			}
			if len(d.Perguntas) > 0 {
				return nil
			}
		}
		for i, s := range plano.Secoes {
			if i >= 12 || strings.TrimSpace(s.Titulo) == "" {
				continue
			}
			buscas := s.Buscas
			if len(buscas) == 0 {
				buscas = []string{s.Titulo}
			}
			d.Secoes = append(d.Secoes, Secao{ID: len(d.Secoes) + 1, Titulo: s.Titulo, Orientacao: s.Orientacao,
				Buscas: buscas[:min(len(buscas), 5)], URLs: s.URLs[:min(len(s.URLs), 5)], Estado: SecaoPendente,
				Paragrafos: []Paragrafo{}, Comentarios: []Comentario{}})
		}
		if len(d.Secoes) == 0 {
			return fmt.Errorf("%w: o plano veio sem seções", modelos.ErrRespostaRuim)
		}
		d.Planejado = true
		return nil
	})
}

const (
	maxFontesPorSecao = 14
	maxFontesDaWeb    = 6
	maxTextoDaFonte   = 3000
)

func cortar(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// pesquisar reúne as fontes de uma seção no acervo e na web.
func (e *Executor) pesquisar(ctx context.Context, x atividades.Atividade, d Dados, s Secao) ([]Fonte, []string, error) {
	var fontes []Fonte
	var avisos []string
	buscas := slices.Clone(s.Buscas)
	urls := slices.Clone(s.URLs)
	for _, c := range s.Comentarios {
		if !c.Atendido {
			buscas = append(buscas, c.Texto)
			urls = append(urls, politica.URLs(c.Texto)...)
		}
	}
	// Os endereços que o usuário escreveu no pedido valem para todas as seções.
	urls = append(politica.URLs(strings.Join(textosDoUsuario(x, d), "\n")), urls...)

	// Intercala os resultados das buscas para nenhuma dominar a seção.
	var porBusca [][]acervo.Achado
	for _, b := range buscas {
		achados, err := e.Acervo.Buscar(ctx, x.ID, b, 6)
		if err != nil {
			return nil, nil, err
		}
		porBusca = append(porBusca, achados)
	}
	vistos := map[int64]bool{}
	for i := 0; i < 6; i++ {
		for _, achados := range porBusca {
			if i < len(achados) && !vistos[achados[i].ID] {
				vistos[achados[i].ID] = true
				a := achados[i]
				fontes = append(fontes, Fonte{Tipo: "documento", Titulo: a.Nome, Trecho: a.ID,
					Documento: a.Documento, Pagina: a.Pagina, Texto: cortar(a.Texto, maxTextoDaFonte)})
			}
		}
	}

	var daWeb []Fonte
	var externos []string
	for _, f := range fontes {
		externos = append(externos, f.Texto)
	}
	achados, doBuscador, err := e.pesquisarNaWeb(ctx, x, d, s, buscas)
	if err != nil {
		return nil, nil, err
	}
	avisos = append(avisos, doBuscador.avisos...)
	// Os endereços do plano e do pedido vêm antes; os da pesquisa completam.
	urls = append(urls, achados...)

	lidas := map[string]bool{}
	for _, u := range urls {
		if lidas[u] || len(daWeb) >= maxFontesDaWeb || e.Web == nil {
			continue
		}
		lidas[u] = true
		// Os endereços sugeridos pelo plano passam pelas regras como se o contexto
		// ainda estivesse limpo, pois o plano foi feito sem conteúdo externo.
		decisao := politica.Avaliar(politica.Pedido{Tipo: politica.Relatorio, Ferramenta: politica.BuscarURL, URL: u,
			TextosUsuario: textosDoUsuario(x, d), HostsLiberados: e.hosts(), Achados: achados})
		if err := e.Diario.Registrar(diario.Entrada{Atividade: x.ID, Tipo: "ferramenta", Nome: politica.BuscarURL,
			Origem: string(proveniencia.Modelo), Decisao: string(decisao.Veredito),
			Motivo: strings.Join(decisao.Motivos, "; "), Detalhe: u}); err != nil {
			return nil, nil, err
		}
		switch decisao.Veredito {
		case politica.Bloquear:
			avisos = append(avisos, "bloqueado: "+u)
		case politica.Confirmar:
			args, err := json.Marshal(map[string]any{"url": u, "secao": s.ID})
			if err != nil {
				return nil, nil, err
			}
			if _, err := e.Confirmacoes.Criar(ctx, confirmacoes.Confirmacao{Atividade: x.ID,
				Ferramenta: politica.BuscarURL, Argumentos: args, Motivos: decisao.Motivos}); err != nil {
				return nil, nil, err
			}
			avisos = append(avisos, "aguardando confirmação: "+u)
		default:
			p, err := e.Web.Buscar(ctx, u)
			if err != nil {
				if ctx.Err() != nil {
					return nil, nil, ctx.Err()
				}
				avisos = append(avisos, "não foi possível ler "+u)
				continue
			}
			if p.Texto == "" {
				continue
			}
			titulo := p.Titulo
			if titulo == "" {
				titulo = p.URL
			}
			daWeb = append(daWeb, Fonte{Tipo: "url", Titulo: titulo, URL: p.URL, Texto: cortar(p.Texto, 3*maxTextoDaFonte)})
		}
	}
	// O trecho do buscador vale como fonte para o que não foi lido por inteiro.
	obtidas := map[string]bool{}
	for _, f := range daWeb {
		obtidas[f.URL] = true
	}
	for _, f := range doBuscador.fontes {
		if len(daWeb) >= maxFontesDaWeb {
			break
		}
		if !obtidas[f.URL] {
			daWeb = append(daWeb, f)
		}
	}
	if sobra := maxFontesPorSecao - len(daWeb); len(fontes) > sobra {
		fontes = fontes[:sobra]
	}
	return append(daWeb, fontes...), avisos, nil
}

func (e *Executor) hosts() []string {
	if e.HostsLiberados == nil {
		return nil
	}
	return e.HostsLiberados()
}

type secaoEscrita struct {
	Paragrafos []Paragrafo `json:"paragrafos"`
}

// separar divide os parágrafos entre os que citam só fontes recebidas e os
// demais.
func separar(paragrafos []Paragrafo, validas map[string]bool) (com []Paragrafo, sem int) {
	for _, p := range paragrafos {
		var fontes []string
		for _, f := range p.Fontes {
			f = strings.Trim(strings.TrimSpace(f), "[]")
			if validas[f] && !slices.Contains(fontes, f) {
				fontes = append(fontes, f)
			}
		}
		switch {
		case strings.TrimSpace(p.Texto) == "":
		case len(fontes) == 0:
			sem++
		default:
			com = append(com, Paragrafo{Texto: strings.TrimSpace(p.Texto), Fontes: fontes})
		}
	}
	return com, sem
}

func (e *Executor) escreverSecao(ctx context.Context, x atividades.Atividade, d Dados, id int) error {
	atual, err := d.secao(id)
	if err != nil {
		return err
	}
	s := *atual
	fontes, avisos, err := e.pesquisar(ctx, x, d, s)
	if err != nil {
		return err
	}
	var ids []string
	d, err = e.mudar(ctx, x.ID, func(d *Dados) error {
		ids = ids[:0]
		for _, f := range fontes {
			ids = append(ids, d.guardar(f))
		}
		return nil
	})
	if err != nil {
		return err
	}

	var escrita []Paragrafo
	descartadas := 0
	if len(ids) == 0 {
		avisos = append(avisos, "nenhuma fonte encontrada; importe documentos ou informe um endereço num comentário")
	} else {
		validas := map[string]bool{}
		var material strings.Builder
		for i, fid := range ids {
			validas[fid] = true
			f := fontes[i]
			origem, nome := proveniencia.Documento, fmt.Sprintf("%s: %s, página %d", fid, f.Titulo, f.Pagina)
			if f.Tipo == "url" {
				origem, nome = proveniencia.Ferramenta, fid+": "+f.URL
			}
			material.WriteString(proveniencia.Embrulhar(origem, nome, "["+fid+"] "+f.Texto) + "\n")
		}
		var pedido strings.Builder
		pedido.WriteString(pedidoDoUsuario(x, d) + "\n\nSeção: " + s.Titulo + "\nOrientação: " + s.Orientacao)
		for _, c := range s.Comentarios {
			pedido.WriteString("\nCorreção pedida pelo usuário: " + c.Texto)
		}
		if len(s.Paragrafos) > 0 {
			anterior, err := json.Marshal(s.Paragrafos)
			if err != nil {
				return err
			}
			pedido.WriteString("\n\nVersão anterior desta seção:\n" +
				proveniencia.Embrulhar(proveniencia.Documento, "versão anterior", string(anterior)))
		}
		pedido.WriteString("\n\nFontes:\n" + material.String())

		nivel := modelos.Principal
		if d.Profundo {
			nivel = modelos.Forte
		}
		msgs := []modelos.Mensagem{
			{Papel: modelos.PapelSistema, Conteudo: sistemaSecao + "\n\n" + proveniencia.Instrucao},
			{Papel: modelos.PapelUsuario, Conteudo: pedido.String()},
		}
		for tentativa := 1; tentativa <= 2; tentativa++ {
			var r secaoEscrita
			if err := modelos.CompletarJSON(ctx, e.Modelos, modelos.Pedido{Nivel: nivel, Atividade: x.ID,
				Finalidade: politica.Relatorio, Mensagens: msgs,
				Esquema: &modelos.Esquema{Nome: "secao_relatorio", Definicao: esquemaSecao}}, &r); err != nil {
				return err
			}
			escrita, descartadas = separar(r.Paragrafos, validas)
			if descartadas == 0 {
				break
			}
			bruto, err := json.Marshal(r)
			if err != nil {
				return err
			}
			msgs = append(msgs, modelos.Mensagem{Papel: modelos.PapelAssistente, Conteudo: string(bruto)},
				modelos.Mensagem{Papel: modelos.PapelUsuario, Conteudo: fmt.Sprintf("%d parágrafo(s) não citam uma "+
					"fonte válida. As fontes válidas são %s. Reescreva a seção mantendo apenas o que elas sustentam.",
					descartadas, strings.Join(ids, ", "))})
		}
	}
	_, err = e.mudar(ctx, x.ID, func(d *Dados) error {
		alvo, err := d.secao(id)
		if err != nil {
			return err
		}
		// Um comentário feito enquanto o texto era escrito pede nova versão.
		if alvo.Revisao != s.Revisao {
			return nil
		}
		alvo.Paragrafos = append([]Paragrafo{}, escrita...)
		alvo.Estado, alvo.Descartadas, alvo.Aviso = SecaoRascunho, descartadas, strings.Join(avisos, "; ")
		for i := range alvo.Comentarios {
			alvo.Comentarios[i].Atendido = true
		}
		return nil
	})
	return err
}

// Agir implementa atividades.Executor.
func (e *Executor) Agir(ctx context.Context, x atividades.Atividade, acao string, corpo json.RawMessage) (atividades.Efeito, error) {
	switch acao {
	case "responder":
		in, err := atividades.Decodificar[struct {
			Respostas []string `json:"respostas"`
		}](corpo)
		if err != nil {
			return atividades.Efeito{}, err
		}
		_, err = e.mudar(ctx, x.ID, func(d *Dados) error {
			if d.Planejado || len(in.Respostas) != len(d.Perguntas) {
				return atividades.Invalido("envie uma resposta para cada pergunta pendente")
			}
			for i, r := range in.Respostas {
				if strings.TrimSpace(r) == "" {
					return atividades.Invalido("há resposta vazia")
				}
				d.Perguntas[i].Resposta = strings.TrimSpace(r)
			}
			return nil
		})
		return atividades.Efeito{Executar: true}, err
	case "comentar":
		in, err := atividades.Decodificar[struct {
			Secao int    `json:"secao"`
			Texto string `json:"texto"`
		}](corpo)
		if err != nil {
			return atividades.Efeito{}, err
		}
		if strings.TrimSpace(in.Texto) == "" {
			return atividades.Efeito{}, atividades.Invalido("o comentário está vazio")
		}
		_, err = e.mudar(ctx, x.ID, func(d *Dados) error {
			s, err := d.secao(in.Secao)
			if err != nil {
				return err
			}
			s.Comentarios = append(s.Comentarios, Comentario{Texto: strings.TrimSpace(in.Texto), Quando: time.Now().UTC()})
			s.Estado, s.Revisao = SecaoPendente, s.Revisao+1
			return nil
		})
		return atividades.Efeito{Executar: true}, err
	case "aprovar_secao":
		in, err := atividades.Decodificar[struct {
			Secao    int  `json:"secao"`
			Aprovada bool `json:"aprovada"`
		}](corpo)
		if err != nil {
			return atividades.Efeito{}, err
		}
		_, err = e.mudar(ctx, x.ID, func(d *Dados) error {
			s, err := d.secao(in.Secao)
			if err != nil {
				return err
			}
			if s.Estado == SecaoPendente {
				return atividades.Invalido("a seção ainda está sendo escrita")
			}
			s.Estado = SecaoRascunho
			if in.Aprovada {
				s.Estado = SecaoAprovada
			}
			return nil
		})
		return atividades.Efeito{}, err
	case "aprovar":
		return e.aprovar(ctx, x)
	case atividades.ConfirmacaoDecidida:
		c, err := atividades.Decodificar[confirmacoes.Confirmacao](corpo)
		if err != nil {
			return atividades.Efeito{}, err
		}
		if c.Estado != confirmacoes.Aprovada {
			return atividades.Efeito{Executar: true}, nil
		}
		var args struct {
			URL   string `json:"url"`
			Secao int    `json:"secao"`
		}
		if err := json.Unmarshal(c.Argumentos, &args); err != nil {
			return atividades.Efeito{}, err
		}
		_, err = e.mudar(ctx, x.ID, func(d *Dados) error {
			s, err := d.secao(args.Secao)
			if err != nil {
				return err
			}
			d.Liberadas = append(d.Liberadas, args.URL)
			if !slices.Contains(s.URLs, args.URL) {
				s.URLs = append(s.URLs, args.URL)
			}
			s.Estado, s.Revisao = SecaoPendente, s.Revisao+1
			return nil
		})
		return atividades.Efeito{Executar: true}, err
	}
	return atividades.Efeito{}, atividades.Invalido("ação desconhecida: %q", acao)
}

// Markdown monta o documento com as fontes citadas, numeradas por ordem de uso.
func Markdown(titulo string, d Dados) string {
	var b strings.Builder
	b.WriteString("# " + titulo + "\n")
	numero := map[string]int{}
	var usadas []string
	for _, s := range d.Secoes {
		b.WriteString("\n## " + s.Titulo + "\n")
		for _, p := range s.Paragrafos {
			b.WriteString("\n" + p.Texto)
			for _, f := range p.Fontes {
				if numero[f] == 0 {
					usadas = append(usadas, f)
					numero[f] = len(usadas)
				}
				fmt.Fprintf(&b, " [%d]", numero[f])
			}
			b.WriteString("\n")
		}
		if len(s.Paragrafos) == 0 {
			b.WriteString("\n*Sem conteúdo: nenhuma fonte sustentou esta seção.*\n")
		}
	}
	b.WriteString("\n## Fontes\n\n")
	for i, id := range usadas {
		for _, f := range d.Fontes {
			if f.ID != id {
				continue
			}
			if f.Tipo == "url" {
				fmt.Fprintf(&b, "%d. %s: <%s>\n", i+1, f.Titulo, f.URL)
			} else {
				fmt.Fprintf(&b, "%d. %s, página %d (acervo)\n", i+1, f.Titulo, f.Pagina)
			}
		}
	}
	return b.String()
}

func (e *Executor) aprovar(ctx context.Context, x atividades.Atividade) (atividades.Efeito, error) {
	var nada atividades.Efeito
	d, err := atividades.Dados[Dados](x)
	if err != nil {
		return nada, err
	}
	if !d.Planejado || slices.ContainsFunc(d.Secoes, func(s Secao) bool { return s.Estado == SecaoPendente }) {
		return nada, atividades.Invalido("o relatório ainda está sendo escrito")
	}
	nome := fmt.Sprintf("%s - versão %d.md", strings.NewReplacer("/", "-", "\\", "-").Replace(x.Titulo), len(d.Versoes)+1)
	doc, _, err := e.Acervo.Importar(ctx, strings.NewReader(Markdown(x.Titulo, d)), nome, "relatorio")
	if err != nil {
		return nada, err
	}
	_, err = e.mudar(ctx, x.ID, func(d *Dados) error {
		for i := range d.Secoes {
			d.Secoes[i].Estado = SecaoAprovada
		}
		d.Versoes = append(d.Versoes, Versao{Documento: doc.ID, Quando: time.Now().UTC()})
		return nil
	})
	if err != nil {
		return nada, err
	}
	if _, err := e.Armazem.Atualizar(ctx, x.ID, func(x *atividades.Atividade) error {
		x.Estado, x.Aviso = atividades.Pronta, "Versão aprovada guardada no acervo."
		return nil
	}); err != nil {
		return nada, err
	}
	return atividades.Efeito{Resposta: map[string]any{"documento": doc.ID}}, nil
}

// Tela implementa atividades.Executor.
func (e *Executor) Tela(_ context.Context, x atividades.Atividade) (string, any, error) {
	d, err := atividades.Dados[Dados](x)
	if err != nil {
		return "", nil, err
	}
	if d.Perguntas == nil {
		d.Perguntas = []Pergunta{}
	}
	if d.Secoes == nil {
		d.Secoes = []Secao{}
	}
	if d.Fontes == nil {
		d.Fontes = []Fonte{}
	}
	if d.Versoes == nil {
		d.Versoes = []Versao{}
	}
	return "relatorio", d, nil
}
