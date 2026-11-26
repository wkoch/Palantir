package extrator

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func arquivo(t *testing.T, nome string, conteudo []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), nome)
	if err := os.WriteFile(p, conteudo, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func extrair(t *testing.T, e Extrator, nome string, conteudo []byte) (Resultado, error) {
	t.Helper()
	return e.Extrair(context.Background(), arquivo(t, "entrada", conteudo), nome)
}

func textoDe(t *testing.T, nome string, conteudo []byte) string {
	t.Helper()
	r, err := extrair(t, Padrao(), nome, conteudo)
	if err != nil {
		t.Fatalf("%s: %v", nome, err)
	}
	var s []string
	for _, p := range r.Paginas {
		s = append(s, p.Texto)
	}
	return strings.Join(s, "\f")
}

func TestTexto(t *testing.T) {
	if got := textoDe(t, "a.md", []byte("\xef\xbb\xbf# Título\n")); got != "# Título" {
		t.Errorf("%q", got)
	}
	// Latin-1 vira UTF-8.
	if got := textoDe(t, "a.txt", []byte("a\xe7\xe3o")); got != "ação" {
		t.Errorf("%q", got)
	}
	if got := textoDe(t, "SEM-EXTENSAO", []byte("ola")); got != "ola" {
		t.Errorf("%q", got)
	}
	if got := textoDe(t, "p.HTML", []byte("<html><head><title>T</title><style>x{}</style></head><body><h1>Oi</h1><script>mau()</script><table><tr><td>a</td><td>b</td></tr></table></body></html>")); got != "Oi\n\na b" {
		t.Errorf("%q", got)
	}
	r, err := extrair(t, Padrao(), "vazio.txt", []byte("  \n"))
	if err != nil || len(r.Paginas) != 0 || r.Paginas == nil {
		t.Error(r, err)
	}
	for nome, c := range map[string][]byte{"a.exe": []byte("texto"), "a.bin": {0, 1, 2}, "b.txt": {'a', 0}} {
		if _, err := extrair(t, Padrao(), nome, c); !errors.Is(err, ErrFormato) {
			t.Error(nome, err)
		}
	}
	if _, err := Padrao().Extrair(context.Background(), "/nao/existe", "a.txt"); err == nil {
		t.Error("arquivo ausente")
	}
	if _, err := Padrao().Extrair(context.Background(), t.TempDir(), "a.txt"); err == nil {
		t.Error("diretório")
	}
}

func TestEmail(t *testing.T) {
	msg := strings.ReplaceAll(`From: =?UTF-8?B?Sm/Do28=?= <joao@exemplo.test>
To: eu@exemplo.test
Subject: =?ISO-8859-1?Q?Informa=E7=E3o?=
Date: Wed, 25 Nov 2026 10:00:00 -0300
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary=b

--b
Content-Type: text/plain; charset=iso-8859-1
Content-Transfer-Encoding: quoted-printable

Ol=E1, segue.
--b
Content-Type: text/html

<p>versão html</p>
--b
Content-Type: application/pdf
Content-Disposition: attachment; filename="nota.pdf"

xx
--b--
`, "\n", "\r\n")
	got := textoDe(t, "m.eml", []byte(msg))
	for _, quer := range []string{"De: ", "João", "Assunto: Informação", "Olá, segue.", "Anexos: nota.pdf"} {
		if !strings.Contains(got, quer) {
			t.Errorf("falta %q em %q", quer, got)
		}
	}
	if strings.Contains(got, "versão html") {
		t.Error("com texto puro, o HTML é ignorado")
	}
	soHTML := "Subject: x\r\nContent-Type: text/html\r\n\r\n<p>apenas <b>html</b></p>"
	if got := textoDe(t, "h.eml", []byte(soHTML)); !strings.Contains(got, "apenas html") {
		t.Error(got)
	}
	if _, err := extrair(t, Padrao(), "ruim.eml", []byte("isto não tem cabeçalho nem corpo\x01")); err == nil {
		t.Log("mensagem sem cabeçalhos é aceita como texto vazio")
	}
}

func zipCom(t *testing.T, partes map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for nome, c := range partes {
		w, err := z.Create(nome)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(c))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestOffice(t *testing.T) {
	docx := zipCom(t, map[string]string{"word/document.xml": `<w:document xmlns:w="w"><w:body>
		<w:p><w:r><w:t>Primeiro</w:t></w:r><w:r><w:tab/><w:t>parágrafo</w:t><w:br/><w:t>quebrado</w:t></w:r></w:p>
		<w:tbl><w:tr><w:tc><w:p><w:r><w:t>A1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>B1</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
		</w:body></w:document>`})
	got := textoDe(t, "d.docx", docx)
	if !strings.Contains(got, "Primeiro parágrafo\nquebrado") || !strings.Contains(got, "A1 | B1 |") {
		t.Errorf("%q", got)
	}
	odt := zipCom(t, map[string]string{"content.xml": `<o:document-content xmlns:o="o" xmlns:t="t"><o:automatic-styles>ignorar</o:automatic-styles>
		<o:body><t:h>Título</t:h><t:p>Um<t:s/>texto <t:span>marcado</t:span><t:line-break/>fim</t:p></o:body></o:document-content>`})
	got = textoDe(t, "d.odt", odt)
	if !strings.Contains(got, "Título\n") || !strings.Contains(got, "Um texto marcado\nfim") || strings.Contains(got, "ignorar") {
		t.Errorf("%q", got)
	}
	pptx := zipCom(t, map[string]string{"ppt/presentation.xml": "<p/>",
		"ppt/slides/slide10.xml": `<a xmlns:a="a"><a:p><a:t>décimo</a:t></a:p></a>`,
		"ppt/slides/slide2.xml":  `<a xmlns:a="a"><a:p><a:t>segundo</a:t></a:p></a>`,
		"ppt/slides/slide3.xml":  `<a/>`, "ppt/slides/slideX.xml": `<a/>`})
	r, err := extrair(t, Padrao(), "s.pptx", pptx)
	if err != nil || len(r.Paginas) != 2 || r.Paginas[0].Numero != 1 || r.Paginas[0].Texto != "segundo" {
		t.Error(r, err)
	}
	xlsx := zipCom(t, map[string]string{"xl/workbook.xml": "<w/>",
		"xl/sharedStrings.xml": `<sst><si><t>Item</t></si><si><r><t>Va</t></r><r><t>lor</t></r></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row><c t="s"><v>0</v></c><c t="s"><v>1</v></c></row>
			<row><c t="inlineStr"><is><t>Luz</t></is></c><c><v>42.5</v></c></row><row><c t="s"><v>99</v></c></row><row><c/></row></sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml": `<worksheet/>`})
	got = textoDe(t, "p.xlsx", xlsx)
	if !strings.Contains(got, "Item | Valor\nLuz | 42.5\n99") {
		t.Errorf("%q", got)
	}
	ruins := []map[string]string{
		{"outro.txt": "x"},
		{"word/document.xml": "<a><b></a>"},
		{"ppt/presentation.xml": "x", "ppt/slides/slide1.xml": "<a><b></a>"},
		{"xl/workbook.xml": "x", "xl/sharedStrings.xml": "<a><"},
		{"xl/workbook.xml": "x", "xl/worksheets/sheet1.xml": "<a><"},
		{"word/document.xml": "<a>" + strings.Repeat("x", limiteParte+1) + "</a>"},
	}
	for i, p := range ruins {
		if _, err := extrair(t, Padrao(), "r.zip", zipCom(t, p)); err == nil {
			t.Errorf("caso %d deveria falhar", i)
		}
	}
	if _, err := extrair(t, Padrao(), "r.docx", []byte("PK\x03\x04corrompido")); err == nil {
		t.Error("zip corrompido")
	}
}

// pdf monta um PDF mínimo com uma linha de texto por página.
func pdf(paginas ...string) []byte {
	var objetos []string
	var filhos []string
	n := len(paginas)
	for i, texto := range paginas {
		pagina, conteudo := 4+2*i, 5+2*i
		filhos = append(filhos, fmt.Sprintf("%d 0 R", pagina))
		fluxo := ""
		if texto != "" {
			fluxo = "BT /F1 12 Tf 72 700 Td (" + texto + ") Tj ET"
		}
		objetos = append(objetos,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", conteudo),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(fluxo), fluxo))
	}
	todos := append([]string{"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(filhos, " "), n),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}, objetos...)
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	var posicoes []int
	for i, o := range todos {
		posicoes = append(posicoes, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(todos)+1)
	for _, p := range posicoes {
		fmt.Fprintf(&b, "%010d 00000 n \n", p)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(todos)+1, xref)
	return b.Bytes()
}

func programa(t *testing.T, corpo string) string {
	t.Helper()
	p := arquivo(t, "falso", []byte("#!/bin/sh\n"+corpo+"\n"))
	if err := os.Chmod(p, 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPDF(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext não instalado (pacote poppler-utils)")
	}
	semOCR := Padrao()
	semOCR.PDFToPPM = "/nao/existe"
	r, err := extrair(t, semOCR, "d.pdf", pdf("Clausula primeira do contrato de teste", "", "Terceira pagina com bastante texto"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paginas) != 2 || r.Paginas[1].Numero != 3 || !strings.Contains(r.Paginas[0].Texto, "Clausula primeira") ||
		!strings.Contains(r.Aviso, "1 página(s) sem texto") {
		t.Errorf("%+v", r)
	}
	// Uma página sem texto passa pelo OCR.
	ocr := Padrao()
	ocr.PDFToPPM = programa(t, `for ultimo; do :; done; : > "$ultimo.png"`)
	ocr.Tesseract = programa(t, `echo "texto reconhecido na imagem"`)
	r, err = extrair(t, ocr, "e.pdf", pdf(""))
	if err != nil || len(r.Paginas) != 1 || r.Paginas[0].Texto != "texto reconhecido na imagem" || r.Aviso != "" {
		t.Errorf("%+v %v", r, err)
	}
	if _, err := extrair(t, Padrao(), "x.pdf", []byte("%PDF-1.4 quebrado")); err == nil {
		t.Error("PDF quebrado deveria falhar")
	}
}

func TestImagem(t *testing.T) {
	e := Padrao()
	e.Tesseract = programa(t, `echo "  recibo   de  teste "`)
	for nome, c := range map[string][]byte{"a.png": []byte("\x89PNG...."), "b.jpg": []byte("\xff\xd8\xff...."), "c.tif": []byte("II*")} {
		r, err := extrair(t, e, nome, c)
		if err != nil || len(r.Paginas) != 1 || r.Paginas[0].Texto != "recibo de teste" || r.Tipo != "imagem" {
			t.Error(nome, r, err)
		}
	}
	e.Tesseract = programa(t, `true`)
	r, err := extrair(t, e, "a.png", []byte("\x89PNG"))
	if err != nil || r.Aviso == "" {
		t.Error(r, err)
	}
	e.Tesseract = programa(t, `echo `+strings.Repeat("x", 400)+` >&2; exit 3`)
	if _, err := extrair(t, e, "a.png", []byte("\x89PNG")); err == nil || len(err.Error()) > 400 {
		t.Error(err)
	}
}
