// Package acervo guarda os documentos: originais imutáveis endereçados por
// hash e derivados que podem ser refeitos a qualquer momento.
package acervo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/diario"
	"github.com/wkoch/Palantir/internal/ferramentas"
	"github.com/wkoch/Palantir/internal/modelos"
)

// Estados da extração de um documento.
const (
	Pendente = "pendente"
	Pronto   = "pronto"
	Erro     = "erro"
)

// Erros de uso do acervo.
var (
	ErrPastaProibida = errors.New("essa pasta não pode ser importada")
	ErrNaoEncontrado = errors.New("documento não encontrado")
	ErrGrande        = errors.New("arquivo grande demais")
	ErrVazio         = errors.New("arquivo vazio")
)

// LimiteArquivo é o maior original aceito.
const LimiteArquivo = 256 << 20

// Documento descreve um original.
type Documento struct {
	ID          int64     `json:"id"`
	SHA256      string    `json:"sha256"`
	Nome        string    `json:"nome"`
	Tipo        string    `json:"tipo"`
	Tamanho     int64     `json:"tamanho"`
	Origem      string    `json:"origem"`
	ImportadoEm time.Time `json:"importado_em"`
	Estado      string    `json:"estado"`
	Erro        string    `json:"erro,omitempty"`
	Resumo      string    `json:"resumo,omitempty"`
	Paginas     int       `json:"paginas"`
}

// Modelos é o que o acervo pede ao roteador.
type Modelos interface {
	modelos.Completador
	Embeddings(ctx context.Context, atividade int64, textos []string) ([][]float32, string, error)
}

// Acervo é o repositório de documentos.
type Acervo struct {
	DB        *sql.DB
	Originais string
	// Privadas são pastas do próprio Palantír, com segredos, banco e diário.
	// Nada dentro delas entra no acervo.
	Privadas   []string
	Executor   ferramentas.Executor
	Modelos    Modelos
	Config     func() config.Modelos
	Diario     *diario.Diario
	Notificar  func()
	acordar    chan struct{}
	umaVez     sync.Once
	importacao sync.Mutex
	// processamento impede que duas rodadas tratem o mesmo documento.
	processamento sync.Mutex
}

func (a *Acervo) sinal() chan struct{} {
	a.umaVez.Do(func() { a.acordar = make(chan struct{}, 1) })
	return a.acordar
}

// Acordar pede uma rodada de processamento ao trabalhador.
func (a *Acervo) Acordar() {
	select {
	case a.sinal() <- struct{}{}:
	default:
	}
}

func (a *Acervo) avisar() {
	if a.Notificar != nil {
		a.Notificar()
	}
}

// CaminhoOriginal é onde fica o original de um hash.
func (a *Acervo) CaminhoOriginal(hash string) string { return filepath.Join(a.Originais, hash) }

func nomeLimpo(nome string) string {
	nome = filepath.Base(strings.ReplaceAll(nome, "\\", "/"))
	nome = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, strings.ToValidUTF8(nome, ""))
	if nome == "." || nome == "/" || nome == ".." || nome == "" {
		return "sem-nome"
	}
	if r := []rune(nome); len(r) > 200 {
		ext := []rune(filepath.Ext(nome))
		if len(ext) > 20 {
			ext = nil
		}
		nome = string(r[:200-len(ext)]) + string(ext)
	}
	return nome
}

func tipoDe(nome string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(nome))); t != "" {
		t, _, _ = strings.Cut(t, ";")
		return t
	}
	return "application/octet-stream"
}

// Importar grava um original. O mesmo conteúdo é guardado uma única vez, e o
// segundo valor informa se o documento é novo.
func (a *Acervo) Importar(ctx context.Context, r io.Reader, nome, origem string) (Documento, bool, error) {
	nome = nomeLimpo(nome)
	tmp, err := os.CreateTemp(a.Originais, ".importando-*")
	if err != nil {
		return Documento{}, false, err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name()) // falha sem efeito depois do link
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, LimiteArquivo+1))
	if err != nil {
		return Documento{}, false, fmt.Errorf("copiar %s: %w", nome, err)
	}
	switch {
	case n > LimiteArquivo:
		return Documento{}, false, fmt.Errorf("%w: %s", ErrGrande, nome)
	case n == 0:
		return Documento{}, false, fmt.Errorf("%w: %s", ErrVazio, nome)
	}
	if err := tmp.Sync(); err != nil {
		return Documento{}, false, err
	}
	if err := tmp.Chmod(0o400); err != nil {
		return Documento{}, false, err
	}
	hash := hex.EncodeToString(h.Sum(nil))

	a.importacao.Lock()
	defer a.importacao.Unlock()
	// Link falha se o destino existir: um original nunca é sobrescrito.
	if err := os.Link(tmp.Name(), a.CaminhoOriginal(hash)); err != nil && !errors.Is(err, fs.ErrExist) {
		return Documento{}, false, fmt.Errorf("guardar o original: %w", err)
	}
	if d, err := a.porHash(ctx, hash); err == nil {
		return d, false, nil
	} else if !errors.Is(err, ErrNaoEncontrado) {
		return Documento{}, false, err
	}
	res, err := a.DB.ExecContext(ctx, `INSERT INTO documentos (sha256, nome, tipo, tamanho, origem, importado_em, estado)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, hash, nome, tipoDe(nome), n, origem, time.Now().UnixMilli(), Pendente)
	if err != nil {
		return Documento{}, false, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Documento{}, false, err
	}
	if err := a.Diario.Registrar(diario.Entrada{Tipo: "sistema", Nome: "importar", Origem: origem,
		Decisao: "ok", Detalhe: nome + " " + hash}); err != nil {
		return Documento{}, false, err
	}
	a.Acordar()
	a.avisar()
	d, err := a.Documento(ctx, id)
	return d, true, err
}

// Relatorio resume uma importação de vários arquivos.
type Relatorio struct {
	Importados int      `json:"importados"`
	Repetidos  int      `json:"repetidos"`
	Falhas     []string `json:"falhas"`
}

func (r *Relatorio) somar(novo bool) {
	if novo {
		r.Importados++
	} else {
		r.Repetidos++
	}
}

func (a *Acervo) importarArquivo(ctx context.Context, caminho, origem string) (bool, error) {
	f, err := os.Open(caminho)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, novo, err := a.Importar(ctx, f, filepath.Base(caminho), origem)
	return novo, err
}

// LimiteDeArquivos é o máximo de arquivos de uma única importação de pasta.
const LimiteDeArquivos = 20_000

// resolver devolve o caminho absoluto, sem links simbólicos quando existe.
func resolver(caminho string) string {
	if abs, err := filepath.Abs(caminho); err == nil {
		caminho = abs
	}
	if real, err := filepath.EvalSymlinks(caminho); err == nil {
		caminho = real
	}
	return caminho
}

// Contem informa se filho é a própria pasta pai ou está abaixo dela.
func Contem(pai, filho string) bool {
	rel, err := filepath.Rel(pai, filho)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (a *Acervo) privadas() []string {
	lista := []string{resolver(a.Originais)}
	for _, p := range a.Privadas {
		lista = append(lista, resolver(p))
	}
	return lista
}

// conferirCaminho recusa a raiz do sistema e as pastas do próprio Palantír.
func (a *Acervo) conferirCaminho(real string) error {
	if filepath.Dir(real) == real {
		return fmt.Errorf("%w: a raiz do sistema inteira", ErrPastaProibida)
	}
	for _, p := range a.privadas() {
		if Contem(p, real) {
			return fmt.Errorf("%w: pertence ao próprio Palantír", ErrPastaProibida)
		}
	}
	return nil
}

// ImportarCaminho importa um arquivo ou uma pasta inteira. Os arquivos de
// origem não são alterados. Arquivos ocultos, links simbólicos e as pastas do
// próprio Palantír são ignorados.
func (a *Acervo) ImportarCaminho(ctx context.Context, caminho, origem string) (Relatorio, error) {
	rel := Relatorio{Falhas: []string{}}
	info, err := os.Stat(caminho)
	if err != nil {
		return rel, err
	}
	real := resolver(caminho)
	if err := a.conferirCaminho(real); err != nil {
		return rel, err
	}
	if !info.IsDir() {
		novo, err := a.importarArquivo(ctx, real, origem)
		if err != nil {
			return rel, err
		}
		rel.somar(novo)
		return rel, nil
	}
	privadas := a.privadas()
	vistos := 0
	err = filepath.WalkDir(real, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			rel.Falhas = append(rel.Falhas, err.Error())
			return nil
		}
		oculto := p != real && strings.HasPrefix(d.Name(), ".")
		if d.IsDir() {
			if oculto || slices.Contains(privadas, p) {
				return fs.SkipDir
			}
			return nil
		}
		if oculto || !d.Type().IsRegular() {
			return nil
		}
		if vistos++; vistos > LimiteDeArquivos {
			rel.Falhas = append(rel.Falhas, fmt.Sprintf("a importação parou em %d arquivos; escolha pastas menores", LimiteDeArquivos))
			return fs.SkipAll
		}
		novo, err := a.importarArquivo(ctx, p, origem)
		if err != nil {
			rel.Falhas = append(rel.Falhas, p+": "+err.Error())
			return nil
		}
		rel.somar(novo)
		return nil
	})
	return rel, err
}

const colunas = `id, sha256, nome, tipo, tamanho, origem, importado_em, estado, erro, resumo, paginas`

func lerDocumento(s interface{ Scan(...any) error }) (Documento, error) {
	var d Documento
	var quando int64
	err := s.Scan(&d.ID, &d.SHA256, &d.Nome, &d.Tipo, &d.Tamanho, &d.Origem, &quando, &d.Estado, &d.Erro, &d.Resumo, &d.Paginas)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNaoEncontrado
	}
	d.ImportadoEm = time.UnixMilli(quando).UTC()
	return d, err
}

// Documento lê um documento pelo identificador.
func (a *Acervo) Documento(ctx context.Context, id int64) (Documento, error) {
	return lerDocumento(a.DB.QueryRowContext(ctx, `SELECT `+colunas+` FROM documentos WHERE id = ?`, id))
}

func (a *Acervo) porHash(ctx context.Context, hash string) (Documento, error) {
	return lerDocumento(a.DB.QueryRowContext(ctx, `SELECT `+colunas+` FROM documentos WHERE sha256 = ?`, hash))
}

// Filtro restringe uma listagem.
type Filtro struct {
	Nome     string
	Colecao  int64
	Entidade string // "tipo:valor"
	Limite   int
}

func (a *Acervo) documentos(ctx context.Context, q string, args ...any) ([]Documento, error) {
	rows, err := a.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lista := []Documento{}
	for rows.Next() {
		d, err := lerDocumento(rows)
		if err != nil {
			return nil, err
		}
		lista = append(lista, d)
	}
	return lista, rows.Err()
}

// Listar devolve documentos, dos mais novos para os mais antigos.
func (a *Acervo) Listar(ctx context.Context, f Filtro) ([]Documento, error) {
	q := `SELECT ` + colunas + ` FROM documentos WHERE 1 = 1`
	var args []any
	if f.Nome != "" {
		q += ` AND nome LIKE ? ESCAPE '\'`
		args = append(args, "%"+strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(f.Nome)+"%")
	}
	if f.Colecao > 0 {
		q += ` AND id IN (SELECT documento_id FROM colecao_documentos WHERE colecao_id = ?)`
		args = append(args, f.Colecao)
	}
	if tipo, valor, ok := strings.Cut(f.Entidade, ":"); ok {
		q += ` AND id IN (SELECT documento_id FROM entidades WHERE tipo = ? AND valor = ?)`
		args = append(args, tipo, valor)
	}
	if f.Limite <= 0 || f.Limite > 2000 {
		f.Limite = 500
	}
	return a.documentos(ctx, q+` ORDER BY importado_em DESC, id DESC LIMIT ?`, append(args, f.Limite)...)
}

// Trecho é um pedaço de texto de uma página.
type Trecho struct {
	ID        int64  `json:"id"`
	Documento int64  `json:"documento"`
	Nome      string `json:"nome"`
	Pagina    int    `json:"pagina"`
	Texto     string `json:"texto"`
}

const colunasTrecho = `t.id, t.documento_id, d.nome, t.pagina, t.texto`

func (a *Acervo) trechos(ctx context.Context, q string, args ...any) ([]Trecho, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT `+colunasTrecho+` FROM trechos t
		JOIN documentos d ON d.id = t.documento_id `+q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lista := []Trecho{}
	for rows.Next() {
		var t Trecho
		if err := rows.Scan(&t.ID, &t.Documento, &t.Nome, &t.Pagina, &t.Texto); err != nil {
			return nil, err
		}
		lista = append(lista, t)
	}
	return lista, rows.Err()
}

// Trechos devolve os trechos pedidos, na ordem dos identificadores dados.
func (a *Acervo) Trechos(ctx context.Context, ids []int64) ([]Trecho, error) {
	if len(ids) == 0 {
		return []Trecho{}, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	lista, err := a.trechos(ctx, `WHERE t.id IN (?`+strings.Repeat(",?", len(ids)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(lista, func(x, y Trecho) int { return slices.Index(ids, x.ID) - slices.Index(ids, y.ID) })
	return lista, nil
}

// TrechosDoDocumento devolve o texto extraído em ordem.
func (a *Acervo) TrechosDoDocumento(ctx context.Context, id int64) ([]Trecho, error) {
	return a.trechos(ctx, `WHERE t.documento_id = ? ORDER BY t.ordem`, id)
}

const (
	tamanhoTrecho = 1400
	sobreposicao  = 150
)

// Dividir corta o texto de uma página em trechos, preferindo quebrar em
// parágrafos ou frases, com uma pequena sobreposição.
func Dividir(texto string) []string {
	texto = strings.TrimSpace(strings.ToValidUTF8(texto, ""))
	var partes []string
	for utf8.RuneCountInString(texto) > tamanhoTrecho {
		r := []rune(texto)
		janela := string(r[:tamanhoTrecho])
		corte := -1
		for _, sep := range []string{"\n\n", "\n", ". ", " "} {
			if i := strings.LastIndex(janela, sep); i > len(janela)/2 {
				corte = i + len(sep)
				break
			}
		}
		if corte < 0 {
			corte = len(janela)
		}
		partes = append(partes, strings.TrimSpace(texto[:corte]))
		// Recua alguns caracteres para repetir o contexto no trecho seguinte.
		antes := []rune(texto[:corte])
		inicio := corte
		if len(antes) > sobreposicao {
			cauda := string(antes[len(antes)-sobreposicao:])
			if i := strings.IndexAny(cauda, " \n"); i >= 0 {
				inicio = corte - (len(cauda) - i)
			}
		}
		texto = strings.TrimSpace(texto[inicio:])
	}
	if texto != "" {
		partes = append(partes, texto)
	}
	return partes
}
