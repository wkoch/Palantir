// Package extrator transforma documentos em texto por página. Roda dentro do
// contêiner de extração, sem rede.
package extrator

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wkoch/Palantir/internal/textohtml"
)

// Pagina é o texto de uma página, planilha ou parte.
type Pagina struct {
	Numero int    `json:"numero"`
	Texto  string `json:"texto"`
}

// Resultado é o que a extração produz.
type Resultado struct {
	Tipo    string   `json:"tipo"`
	Paginas []Pagina `json:"paginas"`
	Aviso   string   `json:"aviso,omitempty"`
}

// ErrFormato indica um tipo de arquivo que não sabemos ler.
var ErrFormato = errors.New("formato não suportado")

const (
	limiteArquivo = 256 << 20 // arquivo de entrada
	limiteParte   = 64 << 20  // parte descompactada de um ZIP
	limitePaginas = 3000
	limiteOCR     = 200
)

// Extrator configura os programas externos, substituíveis nos testes.
type Extrator struct {
	PDFToText string
	PDFToPPM  string
	Tesseract string
	Idiomas   string
}

// Padrao usa os programas instalados na imagem de ferramentas.
func Padrao() Extrator {
	return Extrator{PDFToText: "pdftotext", PDFToPPM: "pdftoppm", Tesseract: "tesseract", Idiomas: "por+eng"}
}

// Extrair lê o arquivo. nome é o nome original, usado para reconhecer o tipo.
func (e Extrator) Extrair(ctx context.Context, caminho, nome string) (Resultado, error) {
	info, err := os.Stat(caminho)
	if err != nil {
		return Resultado{}, err
	}
	if info.Size() > limiteArquivo {
		return Resultado{}, fmt.Errorf("arquivo maior que %d MB", limiteArquivo>>20)
	}
	cabeca, err := ler(caminho, 512)
	if err != nil {
		return Resultado{}, err
	}
	ext := strings.ToLower(filepath.Ext(nome))
	switch {
	case bytes.HasPrefix(cabeca, []byte("%PDF-")):
		return e.pdf(ctx, caminho)
	case bytes.HasPrefix(cabeca, []byte("\x89PNG")), bytes.HasPrefix(cabeca, []byte("\xff\xd8\xff")),
		ext == ".tif", ext == ".tiff":
		return e.imagem(ctx, caminho)
	case bytes.HasPrefix(cabeca, []byte("PK\x03\x04")):
		return compactado(caminho, ext)
	}
	conteudo, err := ler(caminho, limiteParte)
	if err != nil {
		return Resultado{}, err
	}
	if bytes.IndexByte(cabeca, 0) >= 0 {
		return Resultado{}, fmt.Errorf("%w: %s", ErrFormato, nome)
	}
	texto := paraUTF8(conteudo)
	switch ext {
	case ".html", ".htm", ".xhtml":
		_, t, err := textohtml.Extrair(strings.NewReader(texto))
		if err != nil {
			return Resultado{}, err
		}
		return unica("html", t), nil
	case ".eml":
		return email([]byte(texto))
	case ".txt", ".md", ".markdown", ".csv", ".tsv", ".json", ".xml", ".log", ".rst", ".org", ".tex",
		".yaml", ".yml", ".toml", ".ini", ".go", ".py", ".sh", ".sql", "":
		return unica("texto", texto), nil
	}
	return Resultado{}, fmt.Errorf("%w: %s", ErrFormato, ext)
}

func ler(caminho string, limite int64) ([]byte, error) {
	f, err := os.Open(caminho)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limite))
}

func unica(tipo, texto string) Resultado {
	r := Resultado{Tipo: tipo, Paginas: []Pagina{}}
	if t := strings.TrimSpace(texto); t != "" {
		r.Paginas = append(r.Paginas, Pagina{Numero: 1, Texto: t})
	}
	return r
}

// paraUTF8 trata como Latin-1 o que não for UTF-8, comum em arquivos antigos.
func paraUTF8(b []byte) string {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if utf8.Valid(b) {
		return string(b)
	}
	var s strings.Builder
	for _, c := range b {
		s.WriteRune(rune(c))
	}
	return s.String()
}

func executar(ctx context.Context, programa string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, programa, args...)
	var saida, erros bytes.Buffer
	cmd.Stdout, cmd.Stderr = &saida, &erros
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(erros.String())
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("%s: %w %s", filepath.Base(programa), err, msg)
	}
	return saida.Bytes(), nil
}

func (e Extrator) pdf(ctx context.Context, caminho string) (Resultado, error) {
	saida, err := executar(ctx, e.PDFToText, "-enc", "UTF-8", "-q", caminho, "-")
	if err != nil {
		return Resultado{}, err
	}
	partes := strings.Split(string(saida), "\f")
	// pdftotext termina cada página com um avanço de formulário.
	if n := len(partes); n > 1 && strings.TrimSpace(partes[n-1]) == "" {
		partes = partes[:n-1]
	}
	r := Resultado{Tipo: "pdf", Paginas: []Pagina{}}
	semTexto, lidasPorOCR := 0, 0
	for i, p := range partes {
		if i >= limitePaginas {
			r.Aviso = fmt.Sprintf("só as primeiras %d páginas foram lidas", limitePaginas)
			break
		}
		texto := strings.TrimSpace(p)
		if utf8.RuneCountInString(texto) < 16 {
			if lidasPorOCR < limiteOCR {
				if t, err := e.ocrDaPagina(ctx, caminho, i+1); err == nil && t != "" {
					texto = t
					lidasPorOCR++
				}
			}
			if texto == "" {
				semTexto++
				continue
			}
		}
		r.Paginas = append(r.Paginas, Pagina{Numero: i + 1, Texto: texto})
	}
	if semTexto > 0 {
		r.Aviso = strings.TrimSpace(fmt.Sprintf("%s %d página(s) sem texto reconhecível", r.Aviso, semTexto))
	}
	return r, nil
}

func (e Extrator) ocrDaPagina(ctx context.Context, caminho string, pagina int) (string, error) {
	dir, err := os.MkdirTemp("", "ocr")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	n := strconv.Itoa(pagina)
	base := filepath.Join(dir, "p")
	if _, err := executar(ctx, e.PDFToPPM, "-r", "200", "-f", n, "-l", n, "-png", "-singlefile", caminho, base); err != nil {
		return "", err
	}
	return e.ocr(ctx, base+".png")
}

func (e Extrator) ocr(ctx context.Context, imagem string) (string, error) {
	saida, err := executar(ctx, e.Tesseract, imagem, "stdout", "-l", e.Idiomas)
	if err != nil {
		return "", err
	}
	return textohtml.Arrumar(string(saida)), nil
}

func (e Extrator) imagem(ctx context.Context, caminho string) (Resultado, error) {
	t, err := e.ocr(ctx, caminho)
	if err != nil {
		return Resultado{}, err
	}
	r := unica("imagem", t)
	if len(r.Paginas) == 0 {
		r.Aviso = "nenhum texto reconhecido na imagem"
	}
	return r, nil
}

func compactado(caminho, ext string) (Resultado, error) {
	z, err := zip.OpenReader(caminho)
	if err != nil {
		return Resultado{}, err
	}
	defer z.Close()
	partes := map[string]*zip.File{}
	for _, f := range z.File {
		partes[f.Name] = f
	}
	abrir := func(nome string) ([]byte, error) {
		f, ok := partes[nome]
		if !ok {
			return nil, fmt.Errorf("parte ausente: %s", nome)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, limiteParte+1))
		if len(b) > limiteParte {
			return nil, fmt.Errorf("parte grande demais: %s", nome)
		}
		return b, err
	}
	switch {
	case partes["word/document.xml"] != nil:
		b, err := abrir("word/document.xml")
		if err != nil {
			return Resultado{}, err
		}
		t, err := textoXML(b, regrasXML{texto: []string{"t"}, linha: []string{"p", "tr"},
			quebra: []string{"br"}, coluna: []string{"tc"}, tab: []string{"tab"}})
		return unica("docx", t), err
	case partes["content.xml"] != nil:
		b, err := abrir("content.xml")
		if err != nil {
			return Resultado{}, err
		}
		t, err := textoXML(b, regrasXML{tudo: "body", linha: []string{"p", "h", "table-row"},
			quebra: []string{"line-break"}, coluna: []string{"table-cell"}, tab: []string{"tab", "s"}})
		return unica("odf", t), err
	case partes["xl/workbook.xml"] != nil:
		return planilha(partes, abrir)
	case partes["ppt/presentation.xml"] != nil:
		return apresentacao(partes, abrir)
	}
	return Resultado{}, fmt.Errorf("%w: arquivo compactado %s", ErrFormato, ext)
}

type regrasXML struct {
	texto  []string // elementos cujo conteúdo é texto
	tudo   string   // alternativa: todo texto dentro deste elemento
	linha  []string // quebram linha ao terminar
	quebra []string // quebram linha ao aparecer
	coluna []string // separam colunas ao terminar
	tab    []string // inserem espaço ao aparecer
}

func textoXML(b []byte, r regrasXML) (string, error) {
	d := xml.NewDecoder(bytes.NewReader(b))
	var s strings.Builder
	dentro := 0
	// Dentro de uma célula, o fim de parágrafo não quebra a linha da tabela.
	celula := 0
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return textohtml.Arrumar(s.String()), nil
		}
		if err != nil {
			return "", fmt.Errorf("XML inválido: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := t.Name.Local
			if slices.Contains(r.texto, n) || n == r.tudo {
				dentro++
			}
			if slices.Contains(r.coluna, n) {
				celula++
			}
			if slices.Contains(r.quebra, n) {
				s.WriteByte('\n')
			}
			if slices.Contains(r.tab, n) {
				s.WriteByte(' ')
			}
		case xml.EndElement:
			n := t.Name.Local
			if slices.Contains(r.texto, n) || n == r.tudo {
				dentro--
			}
			switch {
			case slices.Contains(r.coluna, n):
				celula--
				s.WriteString(" | ")
			case slices.Contains(r.linha, n) && celula == 0:
				s.WriteByte('\n')
			case slices.Contains(r.linha, n):
				s.WriteByte(' ')
			}
		case xml.CharData:
			if dentro > 0 {
				s.Write(t)
			}
		}
	}
}

// numeradas devolve as partes "prefixoN.xml" em ordem numérica.
func numeradas(partes map[string]*zip.File, prefixo string) []string {
	type item struct {
		n    int
		nome string
	}
	var itens []item
	for nome := range partes {
		if resto, ok := strings.CutPrefix(nome, prefixo); ok {
			if n, err := strconv.Atoi(strings.TrimSuffix(resto, ".xml")); err == nil {
				itens = append(itens, item{n, nome})
			}
		}
	}
	slices.SortFunc(itens, func(a, b item) int { return a.n - b.n })
	nomes := make([]string, len(itens))
	for i, it := range itens {
		nomes[i] = it.nome
	}
	return nomes
}

func apresentacao(partes map[string]*zip.File, abrir func(string) ([]byte, error)) (Resultado, error) {
	r := Resultado{Tipo: "pptx", Paginas: []Pagina{}}
	for i, nome := range numeradas(partes, "ppt/slides/slide") {
		b, err := abrir(nome)
		if err != nil {
			return r, err
		}
		t, err := textoXML(b, regrasXML{texto: []string{"t"}, linha: []string{"p"}})
		if err != nil {
			return r, err
		}
		if t != "" {
			r.Paginas = append(r.Paginas, Pagina{Numero: i + 1, Texto: t})
		}
	}
	return r, nil
}

func planilha(partes map[string]*zip.File, abrir func(string) ([]byte, error)) (Resultado, error) {
	r := Resultado{Tipo: "xlsx", Paginas: []Pagina{}}
	var compartilhados []string
	if partes["xl/sharedStrings.xml"] != nil {
		b, err := abrir("xl/sharedStrings.xml")
		if err != nil {
			return r, err
		}
		var sst struct {
			Itens []struct {
				T    string   `xml:"t"`
				Runs []string `xml:"r>t"`
			} `xml:"si"`
		}
		if err := xml.Unmarshal(b, &sst); err != nil {
			return r, err
		}
		for _, it := range sst.Itens {
			compartilhados = append(compartilhados, it.T+strings.Join(it.Runs, ""))
		}
	}
	for i, nome := range numeradas(partes, "xl/worksheets/sheet") {
		b, err := abrir(nome)
		if err != nil {
			return r, err
		}
		var ws struct {
			Linhas []struct {
				Celulas []struct {
					Tipo   string `xml:"t,attr"`
					Valor  string `xml:"v"`
					Direto string `xml:"is>t"`
				} `xml:"c"`
			} `xml:"sheetData>row"`
		}
		if err := xml.Unmarshal(b, &ws); err != nil {
			return r, err
		}
		var s strings.Builder
		for _, l := range ws.Linhas {
			var valores []string
			for _, c := range l.Celulas {
				v := c.Valor
				switch c.Tipo {
				case "s":
					if n, err := strconv.Atoi(v); err == nil && n >= 0 && n < len(compartilhados) {
						v = compartilhados[n]
					}
				case "inlineStr":
					v = c.Direto
				}
				valores = append(valores, v)
			}
			if strings.TrimSpace(strings.Join(valores, "")) != "" {
				s.WriteString(strings.Join(valores, " | ") + "\n")
			}
		}
		if t := strings.TrimSpace(s.String()); t != "" {
			r.Paginas = append(r.Paginas, Pagina{Numero: i + 1, Texto: t})
		}
	}
	return r, nil
}
