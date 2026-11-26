// Package estudo é a atividade de estudo para concurso: árvore de assuntos do
// edital, material do acervo, questões, cartões e repetição espaçada.
//
// O modelo é chamado para ler o edital e para criar material. Agendar
// revisões, corrigir respostas e medir o desempenho é código, e funciona sem
// rede.
package estudo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wkoch/Palantir/internal/acervo"
	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/capacidades"
	"github.com/wkoch/Palantir/internal/esquema"
	"github.com/wkoch/Palantir/internal/modelos"
	"github.com/wkoch/Palantir/internal/politica"
	"github.com/wkoch/Palantir/internal/proveniencia"
)

// Assunto é um item da árvore do edital.
type Assunto struct {
	ID       int     `json:"id"`
	Pai      int     `json:"pai"` // 0 para os assuntos principais
	Titulo   string  `json:"titulo"`
	Trechos  []int64 `json:"trechos"`
	Pendente bool    `json:"pendente"`
	Aviso    string  `json:"aviso"`
}

// Dados é o estado de um estudo.
type Dados struct {
	EditalDocumento int64     `json:"edital_documento"`
	Extraido        bool      `json:"extraido"`
	Assuntos        []Assunto `json:"assuntos"`
}

// folha informa se o assunto não tem subtópicos.
func (d Dados) folha(id int) bool {
	return !slices.ContainsFunc(d.Assuntos, func(a Assunto) bool { return a.Pai == id })
}

func (d *Dados) assunto(id int) (*Assunto, error) {
	for i := range d.Assuntos {
		if d.Assuntos[i].ID == id {
			return &d.Assuntos[i], nil
		}
	}
	return nil, atividades.Invalido("o assunto %d não existe", id)
}

// caminho é o título completo, com o assunto principal.
func (d Dados) caminho(a Assunto) string {
	if p, err := d.assunto(a.Pai); a.Pai != 0 && err == nil {
		return p.Titulo + ": " + a.Titulo
	}
	return a.Titulo
}

// Acervo é a parte do acervo usada no estudo.
type Acervo interface {
	Buscar(ctx context.Context, atividade int64, consulta string, limite int) ([]acervo.Achado, error)
	Trechos(ctx context.Context, ids []int64) ([]acervo.Trecho, error)
	TrechosDoDocumento(ctx context.Context, id int64) ([]acervo.Trecho, error)
	Documento(ctx context.Context, id int64) (acervo.Documento, error)
}

// Executor implementa atividades.Executor.
type Executor struct {
	Armazem *atividades.Armazem
	DB      *sql.DB
	Modelos modelos.Completador
	Acervo  Acervo
	Agora   func() time.Time
}

func (e *Executor) agora() time.Time {
	if e.Agora != nil {
		return e.Agora()
	}
	return time.Now()
}

// Iniciar implementa atividades.Executor.
func (e *Executor) Iniciar(ctx context.Context, x *atividades.Atividade, opcoes json.RawMessage) error {
	var d Dados
	if len(opcoes) > 0 {
		o, err := atividades.Decodificar[struct {
			EditalDocumento int64 `json:"edital_documento"`
		}](opcoes)
		if err != nil {
			return err
		}
		d.EditalDocumento = o.EditalDocumento
	}
	if d.EditalDocumento != 0 {
		doc, err := e.Acervo.Documento(ctx, d.EditalDocumento)
		if err != nil {
			return atividades.Invalido("edital: %v", err)
		}
		if x.Titulo == "" {
			x.Titulo = "Estudo: " + doc.Nome
		}
	} else if strings.TrimSpace(x.Objetivo) == "" {
		return atividades.Invalido("cole o conteúdo programático ou escolha o edital no acervo")
	}
	var err error
	x.Dados, err = json.Marshal(d)
	return err
}

var esquemaArvore = esquema.JSON(esquema.Objeto(map[string]any{
	"assuntos": esquema.Lista(esquema.Objeto(map[string]any{
		"titulo": esquema.Texto, "subtopicos": esquema.Lista(esquema.Texto),
	})),
}))

const sistemaArvore = `Você lê o edital de um concurso público e extrai o conteúdo programático como uma
árvore de dois níveis: assuntos (disciplinas) e seus subtópicos. Use os nomes do edital, em português, sem
inventar itens que não estejam nele. Se o usuário indicar um cargo, considere apenas o conteúdo dele.`

var esquemaMaterial = esquema.JSON(esquema.Objeto(map[string]any{
	"cartoes": esquema.Lista(esquema.Objeto(map[string]any{
		"frente": esquema.Texto, "verso": esquema.Texto, "fonte": esquema.Texto,
	})),
	"questoes": esquema.Lista(esquema.Objeto(map[string]any{
		"enunciado": esquema.Texto, "alternativas": esquema.Lista(esquema.Texto),
		"correta": esquema.Inteiro, "explicacao": esquema.Texto, "fonte": esquema.Texto,
	})),
}))

const sistemaMaterial = `Você prepara material de estudo para concurso, em português do Brasil, usando somente
os trechos fornecidos. Crie de 4 a 8 cartões (frente com uma pergunta objetiva, verso com a resposta curta) e
de 3 a 6 questões de múltipla escolha com 5 alternativas, uma única correta (campo correta é o índice a partir
de 0) e uma explicação. Em fonte, informe a marca do trecho que sustenta o item, como T12. Não use
conhecimento que não esteja nos trechos.`

const (
	maxEdital      = 60_000
	trechosPorItem = 8
	maxAssuntos    = 200
)

func cortar(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > n {
		s = string([]rune(s)[:n])
	}
	return s
}

func (e *Executor) dados(ctx context.Context, id int64) (atividades.Atividade, Dados, error) {
	x, err := e.Armazem.Obter(ctx, id)
	if err != nil {
		return x, Dados{}, err
	}
	d, err := atividades.Dados[Dados](x)
	return x, d, err
}

func (e *Executor) extrairArvore(ctx context.Context, x atividades.Atividade, d Dados) error {
	pedido := x.Objetivo
	if d.EditalDocumento != 0 {
		trechos, err := e.Acervo.TrechosDoDocumento(ctx, d.EditalDocumento)
		if err != nil {
			return err
		}
		if len(trechos) == 0 {
			return errors.New("o edital ainda não tem texto extraído; aguarde o acervo e retome")
		}
		var texto strings.Builder
		for _, t := range trechos {
			if texto.Len()+len(t.Texto) > maxEdital {
				break
			}
			texto.WriteString(t.Texto + "\n")
		}
		pedido += "\n\n" + proveniencia.Embrulhar(proveniencia.Documento, "edital", texto.String())
	}
	var v struct {
		Assuntos []struct {
			Titulo     string   `json:"titulo"`
			Subtopicos []string `json:"subtopicos"`
		} `json:"assuntos"`
	}
	if err := modelos.CompletarJSON(ctx, e.Modelos, modelos.Pedido{Nivel: modelos.Principal, Atividade: x.ID,
		Finalidade: politica.Estudo, Cache: true,
		Esquema: &modelos.Esquema{Nome: "arvore_assuntos", Definicao: esquemaArvore},
		Mensagens: []modelos.Mensagem{
			{Papel: modelos.PapelSistema, Conteudo: sistemaArvore + "\n\n" + proveniencia.Instrucao},
			{Papel: modelos.PapelUsuario, Conteudo: pedido},
		}}, &v); err != nil {
		return err
	}
	var arvore []Assunto
	acrescentar := func(pai int, titulo string) int {
		titulo = cortar(titulo, 200)
		if titulo == "" || len(arvore) >= maxAssuntos {
			return 0
		}
		arvore = append(arvore, Assunto{ID: len(arvore) + 1, Pai: pai, Titulo: titulo, Trechos: []int64{}})
		return len(arvore)
	}
	for _, a := range v.Assuntos {
		if pai := acrescentar(0, a.Titulo); pai != 0 {
			for _, s := range a.Subtopicos {
				acrescentar(pai, s)
			}
		}
	}
	if len(arvore) == 0 {
		return fmt.Errorf("%w: nenhum assunto encontrado no edital", modelos.ErrRespostaRuim)
	}
	if _, err := atividades.Modificar(ctx, e.Armazem, x.ID, func(d *Dados) error {
		d.Assuntos, d.Extraido = arvore, true
		return nil
	}); err != nil {
		return err
	}
	return e.ligar(ctx, x.ID)
}

func normalizar(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// minimoColado é o tamanho a partir do qual um texto colado identifica o edital.
const minimoColado = 40

// ligar associa cada subtópico a trechos do acervo, em código.
func (e *Executor) ligar(ctx context.Context, id int64) error {
	x, d, err := e.dados(ctx, id)
	if err != nil {
		return err
	}
	// Um edital colado que também está no acervo é reconhecido pelo texto.
	colado := normalizar(x.Objetivo)
	ehEdital := func(t acervo.Achado) bool {
		return t.Documento == d.EditalDocumento ||
			len(colado) >= minimoColado && strings.Contains(normalizar(t.Texto), colado)
	}
	ligacoes := map[int][]int64{}
	for _, a := range d.Assuntos {
		if !d.folha(a.ID) {
			continue
		}
		achados, err := e.Acervo.Buscar(ctx, id, d.caminho(a), 3*trechosPorItem)
		if err != nil {
			return err
		}
		ids := []int64{}
		for _, x := range achados {
			// O edital lista os assuntos, mas não ensina nenhum.
			if !ehEdital(x) && len(ids) < trechosPorItem {
				ids = append(ids, x.ID)
			}
		}
		ligacoes[a.ID] = ids
	}
	_, err = atividades.Modificar(ctx, e.Armazem, id, func(d *Dados) error {
		for i := range d.Assuntos {
			if ids, ok := ligacoes[d.Assuntos[i].ID]; ok {
				d.Assuntos[i].Trechos = ids
			}
		}
		return nil
	})
	return err
}

type fonte struct {
	Documento int64  `json:"documento"`
	Pagina    int    `json:"pagina"`
	Titulo    string `json:"titulo"`
	Trecho    int64  `json:"trecho"`
	Texto     string `json:"texto"`
}

type material struct {
	Cartoes []struct {
		Frente string `json:"frente"`
		Verso  string `json:"verso"`
		Fonte  string `json:"fonte"`
	} `json:"cartoes"`
	Questoes []struct {
		Enunciado    string   `json:"enunciado"`
		Alternativas []string `json:"alternativas"`
		Correta      int      `json:"correta"`
		Explicacao   string   `json:"explicacao"`
		Fonte        string   `json:"fonte"`
	} `json:"questoes"`
}

func alternativasValidas(a []string, correta int) bool {
	if len(a) < 2 || len(a) > 5 || correta < 0 || correta >= len(a) {
		return false
	}
	vistas := map[string]bool{}
	for _, x := range a {
		x = strings.ToLower(strings.TrimSpace(x))
		if x == "" || vistas[x] {
			return false
		}
		vistas[x] = true
	}
	return true
}

func (e *Executor) gerar(ctx context.Context, id int64, assunto int) error {
	_, d, err := e.dados(ctx, id)
	if err != nil {
		return err
	}
	a, err := d.assunto(assunto)
	if err != nil {
		return err
	}
	trechos, err := e.Acervo.Trechos(ctx, a.Trechos)
	if err != nil {
		return err
	}
	concluir := func(aviso string) error {
		_, err := atividades.Modificar(ctx, e.Armazem, id, func(d *Dados) error {
			a, err := d.assunto(assunto)
			if err == nil {
				a.Pendente, a.Aviso = false, aviso
			}
			return err
		})
		return err
	}
	if len(trechos) == 0 {
		return concluir("Não há material no acervo sobre este assunto. Importe um livro ou apostila e ligue novamente.")
	}
	fontes := map[string]fonte{}
	var texto strings.Builder
	for _, t := range trechos {
		c := capacidades.CitacaoDeTrecho(t)
		fontes[c.Marca] = fonte{Documento: t.Documento, Pagina: t.Pagina, Titulo: t.Nome, Trecho: t.ID, Texto: t.Texto}
		texto.WriteString(proveniencia.Embrulhar(proveniencia.Documento, c.Marca+": "+t.Nome, "["+c.Marca+"] "+t.Texto) + "\n")
	}
	var m material
	if err := modelos.CompletarJSON(ctx, e.Modelos, modelos.Pedido{Nivel: modelos.Principal, Atividade: id,
		Finalidade: politica.Estudo, Esquema: &modelos.Esquema{Nome: "material_estudo", Definicao: esquemaMaterial},
		Mensagens: []modelos.Mensagem{
			{Papel: modelos.PapelSistema, Conteudo: sistemaMaterial + "\n\n" + proveniencia.Instrucao},
			{Papel: modelos.PapelUsuario, Conteudo: "Assunto: " + d.caminho(*a) + "\n\n" + texto.String()},
		}}, &m); err != nil {
		return err
	}
	titulo := d.caminho(*a)
	chave := fmt.Sprint(assunto)
	origem := func(marca string) (string, bool) {
		f, ok := fontes[strings.Trim(strings.TrimSpace(marca), "[]")]
		if !ok {
			return "", false
		}
		b, err := json.Marshal(f)
		return string(b), err == nil
	}
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	agora := e.agora().UnixMilli()
	aceitos, recusados := 0, 0
	for _, c := range m.Cartoes {
		f, ok := origem(c.Fonte)
		if !ok || strings.TrimSpace(c.Frente) == "" || strings.TrimSpace(c.Verso) == "" {
			recusados++
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO cartoes
			(atividade_id, assunto, frente, verso, fonte, proxima_em) VALUES (?, ?, ?, ?, ?, ?)`,
			id, chave, strings.TrimSpace(c.Frente), strings.TrimSpace(c.Verso), f, agora); err != nil {
			return err
		}
		aceitos++
	}
	for _, q := range m.Questoes {
		f, ok := origem(q.Fonte)
		if !ok || strings.TrimSpace(q.Enunciado) == "" || !alternativasValidas(q.Alternativas, q.Correta) {
			recusados++
			continue
		}
		alternativas, err := json.Marshal(q.Alternativas)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO questoes
			(atividade_id, assunto, enunciado, alternativas, correta, explicacao, fonte, criada_em)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, chave, strings.TrimSpace(q.Enunciado), string(alternativas),
			q.Correta, strings.TrimSpace(q.Explicacao), f, agora); err != nil {
			return err
		}
		aceitos++
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	aviso := ""
	switch {
	case aceitos == 0:
		aviso = "O material gerado não citava fontes válidas e foi descartado. Tente novamente (" + titulo + ")."
	case recusados > 0:
		aviso = fmt.Sprintf("%d item(ns) sem fonte ou malformados foram descartados.", recusados)
	}
	return concluir(aviso)
}

// Executar implementa atividades.Executor.
func (e *Executor) Executar(ctx context.Context, id int64) (atividades.Resultado, error) {
	var nada atividades.Resultado
	x, d, err := e.dados(ctx, id)
	if err != nil {
		return nada, err
	}
	if !d.Extraido {
		if err := e.extrairArvore(ctx, x, d); err != nil {
			return nada, err
		}
	}
	for {
		if _, d, err = e.dados(ctx, id); err != nil {
			return nada, err
		}
		i := slices.IndexFunc(d.Assuntos, func(a Assunto) bool { return a.Pendente })
		if i < 0 {
			break
		}
		if err := e.gerar(ctx, id, d.Assuntos[i].ID); err != nil {
			return nada, err
		}
	}
	return atividades.Resultado{Estado: atividades.Pronta}, nil
}

// Correcao é o resultado de uma questão respondida.
type Correcao struct {
	Acertou    bool   `json:"acertou"`
	Correta    int    `json:"correta"`
	Explicacao string `json:"explicacao"`
	Fonte      fonte  `json:"fonte"`
}

// Agir implementa atividades.Executor.
func (e *Executor) Agir(ctx context.Context, x atividades.Atividade, acao string, corpo json.RawMessage) (atividades.Efeito, error) {
	var nada atividades.Efeito
	switch acao {
	case "gerar":
		in, err := atividades.Decodificar[struct {
			Assunto int `json:"assunto"`
		}](corpo)
		if err != nil {
			return nada, err
		}
		_, err = atividades.Modificar(ctx, e.Armazem, x.ID, func(d *Dados) error {
			a, err := d.assunto(in.Assunto)
			if err != nil {
				return err
			}
			// Um assunto principal gera material para todos os seus subtópicos.
			marcados := 0
			for i := range d.Assuntos {
				s := &d.Assuntos[i]
				if (s.ID == a.ID || s.Pai == a.ID) && d.folha(s.ID) {
					s.Pendente, s.Aviso = true, ""
					marcados++
				}
			}
			if marcados == 0 {
				return atividades.Invalido("não há subtópicos para gerar")
			}
			return nil
		})
		return atividades.Efeito{Executar: true}, err
	case "religar":
		if err := e.ligar(ctx, x.ID); err != nil {
			return nada, err
		}
		return nada, nil
	case "revisar":
		in, err := atividades.Decodificar[struct {
			Cartao int64 `json:"cartao"`
			Nota   int   `json:"nota"`
		}](corpo)
		if err != nil {
			return nada, err
		}
		if in.Nota < Errei || in.Nota > Facil {
			return nada, atividades.Invalido("a nota vai de 1 a 4")
		}
		proxima, err := e.revisar(ctx, x.ID, in.Cartao, in.Nota)
		return atividades.Efeito{Resposta: map[string]any{"proxima_em": proxima}}, err
	case "responder":
		in, err := atividades.Decodificar[struct {
			Questao     int64 `json:"questao"`
			Alternativa int   `json:"alternativa"`
		}](corpo)
		if err != nil {
			return nada, err
		}
		c, err := e.responder(ctx, x.ID, in.Questao, in.Alternativa)
		return atividades.Efeito{Resposta: c}, err
	}
	return nada, atividades.Invalido("ação desconhecida: %q", acao)
}

func (e *Executor) revisar(ctx context.Context, atividade, cartao int64, nota int) (time.Time, error) {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Rollback()
	var m Memoria
	var assunto string
	var revisado int64
	err = tx.QueryRowContext(ctx, `SELECT assunto, estabilidade, dificuldade, repeticoes, lapsos, revisado_em
		FROM cartoes WHERE id = ? AND atividade_id = ?`, cartao, atividade).
		Scan(&assunto, &m.Estabilidade, &m.Dificuldade, &m.Repeticoes, &m.Lapsos, &revisado)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, atividades.Invalido("o cartão %d não existe", cartao)
	}
	if err != nil {
		return time.Time{}, err
	}
	m.RevisadoEm = time.UnixMilli(revisado)
	agora := e.agora()
	m, proxima := Revisar(m, nota, agora)
	if _, err := tx.ExecContext(ctx, `UPDATE cartoes SET estabilidade = ?, dificuldade = ?, repeticoes = ?,
		lapsos = ?, revisado_em = ?, proxima_em = ? WHERE id = ?`, m.Estabilidade, m.Dificuldade,
		m.Repeticoes, m.Lapsos, agora.UnixMilli(), proxima.UnixMilli(), cartao); err != nil {
		return time.Time{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO respostas (atividade_id, assunto, tipo, item_id, nota, correta, quando)
		VALUES (?, ?, 'cartao', ?, ?, ?, ?)`, atividade, assunto, cartao, nota, nota > Errei, agora.UnixMilli()); err != nil {
		return time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, err
	}
	e.avisar(atividade)
	return proxima.UTC(), nil
}

func (e *Executor) avisar(id int64) {
	if e.Armazem.Notificar != nil {
		e.Armazem.Notificar(id)
	}
}

func (e *Executor) responder(ctx context.Context, atividade, questao int64, alternativa int) (Correcao, error) {
	var c Correcao
	var assunto, alternativas, f string
	err := e.DB.QueryRowContext(ctx, `SELECT assunto, alternativas, correta, explicacao, fonte
		FROM questoes WHERE id = ? AND atividade_id = ?`, questao, atividade).
		Scan(&assunto, &alternativas, &c.Correta, &c.Explicacao, &f)
	if errors.Is(err, sql.ErrNoRows) {
		return c, atividades.Invalido("a questão %d não existe", questao)
	}
	if err != nil {
		return c, err
	}
	var opcoes []string
	if err := errors.Join(json.Unmarshal([]byte(alternativas), &opcoes), json.Unmarshal([]byte(f), &c.Fonte)); err != nil {
		return c, err
	}
	if alternativa < 0 || alternativa >= len(opcoes) {
		return c, atividades.Invalido("alternativa inexistente")
	}
	c.Acertou = alternativa == c.Correta
	nota := Errei
	if c.Acertou {
		nota = Bom
	}
	if _, err := e.DB.ExecContext(ctx, `INSERT INTO respostas (atividade_id, assunto, tipo, item_id, nota, correta, quando)
		VALUES (?, ?, 'questao', ?, ?, ?, ?)`, atividade, assunto, questao, nota, c.Acertou, e.agora().UnixMilli()); err != nil {
		return c, err
	}
	e.avisar(atividade)
	return c, nil
}

// Tela implementa atividades.Executor.
func (e *Executor) Tela(ctx context.Context, x atividades.Atividade) (string, any, error) {
	d, err := atividades.Dados[Dados](x)
	if err != nil {
		return "", nil, err
	}
	v, err := e.visao(ctx, x.ID, d)
	return "estudo", v, err
}
