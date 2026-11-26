package acervo_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/wkoch/Palantir/internal/acervo"
	"github.com/wkoch/Palantir/internal/ferramentas"
	"github.com/wkoch/Palantir/internal/segredos"
	"github.com/wkoch/Palantir/internal/simulador"
	"github.com/wkoch/Palantir/internal/testes"
)

const contrato = "Contrato de aluguel do apartamento. O aluguel é de R$ 900,00 desde 01/02/2026."

func TestImportarGuardaOriginalImutavel(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	a, ctx := s.D.Acervo, s.Contexto
	d, novo, err := a.Importar(ctx, strings.NewReader(contrato), "../../pasta\\contrato\x00.txt", "teste")
	if err != nil || !novo || d.Nome != "contrato.txt" || d.Tipo != "text/plain" {
		t.Fatal(d, novo, err)
	}
	info, err := os.Stat(a.CaminhoOriginal(d.SHA256))
	if err != nil || info.Mode().Perm() != 0o400 {
		t.Fatal(info.Mode(), err)
	}
	d2, novo, err := a.Importar(ctx, strings.NewReader(contrato), "outro nome.txt", "teste")
	if err != nil || novo || d2.ID != d.ID || d2.Nome != "contrato.txt" {
		t.Fatal(d2, novo, err)
	}
	sobras, _ := filepath.Glob(filepath.Join(s.Caminhos.Originais(), ".importando-*"))
	if len(sobras) != 0 {
		t.Error("arquivos temporários ficaram:", sobras)
	}
	if _, _, err := a.Importar(ctx, strings.NewReader(""), "v", "t"); !errors.Is(err, acervo.ErrVazio) {
		t.Error(err)
	}
	if _, _, err := a.Importar(ctx, io.LimitReader(zeros{}, acervo.LimiteArquivo+2), "g", "t"); !errors.Is(err, acervo.ErrGrande) {
		t.Error(err)
	}
	if _, _, err := a.Importar(ctx, falha{}, "f", "t"); err == nil {
		t.Error("leitura falha")
	}
	longo := strings.Repeat("ã", 300) + ".txt"
	d3, _, err := a.Importar(ctx, strings.NewReader("outro"), longo, "t")
	if err != nil || utf8.RuneCountInString(d3.Nome) != 200 || !strings.HasSuffix(d3.Nome, ".txt") {
		t.Error(d3.Nome, err)
	}
	for _, n := range []string{"", ".", "..", "/", "x." + strings.Repeat("y", 300)} {
		d, _, err := a.Importar(ctx, strings.NewReader("c"+n), n, "t")
		if err != nil || d.Nome == "" || utf8.RuneCountInString(d.Nome) > 200 {
			t.Error(n, d.Nome, err)
		}
	}
	if _, err := a.Documento(ctx, 9999); !errors.Is(err, acervo.ErrNaoEncontrado) {
		t.Error(err)
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) { clear(p); return len(p), nil }

type falha struct{}

func (falha) Read([]byte) (int, error) { return 0, errors.New("disco ruim") }

func TestImportarPasta(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	a, ctx := s.D.Acervo, s.Contexto
	pasta := t.TempDir()
	escrever := func(nome, c string) {
		p := filepath.Join(pasta, nome)
		os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	escrever("a.txt", "um")
	escrever("sub/b.txt", "dois")
	escrever("sub/c.txt", "um")
	escrever(".oculto", "três")
	escrever(".git/config", "quatro")
	escrever("vazio.txt", "")
	if err := os.Symlink("/etc/hostname", filepath.Join(pasta, "link")); err != nil {
		t.Fatal(err)
	}
	r, err := a.ImportarCaminho(ctx, pasta, "pasta")
	if err != nil || r.Importados != 2 || r.Repetidos != 1 || len(r.Falhas) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	if b, err := os.ReadFile(filepath.Join(pasta, "a.txt")); err != nil || string(b) != "um" {
		t.Error("a origem foi alterada")
	}
	r, err = a.ImportarCaminho(ctx, filepath.Join(pasta, "a.txt"), "arquivo")
	if err != nil || r.Repetidos != 1 {
		t.Error(r, err)
	}
	if _, err := a.ImportarCaminho(ctx, "/nao/existe", "x"); err == nil {
		t.Error("caminho ausente")
	}
	if _, err := a.ImportarCaminho(ctx, filepath.Join(pasta, "vazio.txt"), "x"); err == nil {
		t.Error("arquivo vazio")
	}
	if _, err := a.ImportarCaminho(ctx, s.Caminhos.Originais(), "x"); err == nil {
		t.Error("o acervo não importa a si mesmo")
	}
	for _, proibida := range []string{s.Caminhos.Config, s.Caminhos.Dados, s.Caminhos.Segredos()} {
		if _, err := a.ImportarCaminho(ctx, proibida, "x"); !errors.Is(err, acervo.ErrPastaProibida) {
			t.Error(proibida, err)
		}
	}
	atalho := filepath.Join(pasta, "para-os-segredos")
	if err := os.Symlink(s.Caminhos.Config, atalho); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ImportarCaminho(ctx, atalho, "x"); !errors.Is(err, acervo.ErrPastaProibida) {
		t.Error("um link para a configuração", err)
	}
	// A pasta que contém a instalação é percorrida sem entrar nela: nenhum
	// segredo, banco ou diário vira documento.
	antes, _ := a.Listar(ctx, acervo.Filtro{})
	instalacao := filepath.Dir(s.Caminhos.Config)
	if err := os.WriteFile(filepath.Join(instalacao, "visivel.txt"), []byte("fora das pastas privadas"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err = a.ImportarCaminho(ctx, instalacao, "x")
	depois, _ := a.Listar(ctx, acervo.Filtro{})
	if err != nil || r.Importados != 1 || len(depois) != len(antes)+1 || depois[0].Nome != "visivel.txt" {
		t.Error(r, err, depois)
	}
	cancelado, cancelar := context.WithCancel(ctx)
	cancelar()
	if _, err := a.ImportarCaminho(cancelado, pasta, "x"); err == nil {
		t.Error("cancelado")
	}
}

// A raiz é testada só pela regra: percorrê-la copiaria o disco inteiro.
func TestContemERaiz(t *testing.T) {
	for _, c := range []struct {
		pai, filho string
		quer       bool
	}{{"/", "/a/b", true}, {"/a", "/a", true}, {"/a", "/a/b", true}, {"/a", "/ab", false}, {"/a/b", "/a", false},
		{"/a", "/a/..b", true}, {"/a", "relativo", false}} {
		if acervo.Contem(c.pai, c.filho) != c.quer {
			t.Error(c)
		}
	}
	vazio := &acervo.Acervo{Originais: t.TempDir()}
	cancelado, cancelar := context.WithCancel(context.Background())
	cancelar()
	if _, err := vazio.ImportarCaminho(cancelado, "/", "x"); !errors.Is(err, acervo.ErrPastaProibida) {
		t.Fatal("a raiz precisa ser recusada antes de qualquer leitura:", err)
	}
	v := &acervo.Vigia{Acervo: vazio, Pasta: func() string { return "/" }}
	if r, err := v.Varrer(cancelado); err != nil || r.Importados != 0 {
		t.Error(r, err)
	}
}

func TestVigia(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	a, ctx := s.D.Acervo, s.Contexto
	pasta := filepath.Join(t.TempDir(), "entrada")
	v := &acervo.Vigia{Acervo: a, Pasta: func() string { return pasta }}
	varrer := func() acervo.Relatorio {
		t.Helper()
		r, err := v.Varrer(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	varrer() // pasta ausente
	os.MkdirAll(filepath.Join(pasta, ".lixeira"), 0o700)
	arq := filepath.Join(pasta, "baixando.txt")
	os.WriteFile(arq, []byte("parte"), 0o600)
	os.WriteFile(filepath.Join(pasta, "x.part"), []byte("p"), 0o600)
	os.WriteFile(filepath.Join(pasta, ".oculto"), []byte("p"), 0o600)
	os.WriteFile(filepath.Join(pasta, ".lixeira", "z"), []byte("p"), 0o600)
	os.WriteFile(filepath.Join(pasta, "vazio"), nil, 0o600)
	if r := varrer(); r.Importados != 0 {
		t.Fatal("a primeira varredura só observa", r)
	}
	// Ainda crescendo: espera estabilizar.
	os.WriteFile(arq, []byte("parte e o resto"), 0o600)
	if r := varrer(); r.Importados != 0 {
		t.Fatal(r)
	}
	if r := varrer(); r.Importados != 1 {
		t.Fatal(r)
	}
	if r := varrer(); r.Importados+r.Repetidos != 0 {
		t.Fatal("não reimporta", r)
	}
	os.Chtimes(arq, time.Now(), time.Now().Add(time.Hour))
	varrer()
	if r := varrer(); r.Repetidos != 1 {
		t.Fatal(r)
	}
	if _, err := os.Stat(arq); err != nil {
		t.Error("o arquivo de entrada não pode sumir")
	}
	for _, p := range []string{"", s.Caminhos.Originais()} {
		v.Pasta = func() string { return p }
		if r := varrer(); r.Importados != 0 {
			t.Error(p, r)
		}
	}
}

func TestProcessarBuscarEVisoes(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	a, ctx := s.D.Acervo, s.Contexto
	id := s.Importar("contrato.txt", contrato)
	outro := s.Importar("receita.txt", "Receita de bolo de fubá com erva-doce.")
	ruim := s.Importar("foto.bin", "\x00\x01")

	d, _ := a.Documento(ctx, id)
	if d.Estado != acervo.Pronto || d.Paginas != 1 || d.Resumo == "" {
		t.Errorf("%+v", d)
	}
	if x, _ := a.Documento(ctx, ruim); x.Estado != acervo.Erro || !strings.Contains(x.Erro, "não suportado") {
		t.Errorf("%+v", x)
	}
	achados, err := a.Buscar(ctx, 0, "qual o valor do aluguel?", 3)
	if err != nil || len(achados) == 0 || achados[0].Documento != id || achados[0].Pontos <= 0 {
		t.Fatal(achados, err)
	}
	// Acentos não atrapalham.
	achados, _ = a.Buscar(ctx, 0, "FUBA", 0)
	if len(achados) == 0 || achados[0].Documento != outro {
		t.Error(achados)
	}
	for _, q := range []string{`" OR * NEAR( AND`, "", "!!!", "o que é a de"} {
		if _, err := a.Buscar(ctx, 0, q, 100); err != nil {
			t.Error(q, err)
		}
	}
	n, err := a.Numeros(ctx)
	if err != nil || n.Documentos != 3 || n.ComErro != 1 || n.Trechos != 2 || n.Vetores != 2 || n.Pendentes != 0 {
		t.Errorf("%+v %v", n, err)
	}
	trechos, _ := a.TrechosDoDocumento(ctx, id)
	ordenados, err := a.Trechos(ctx, []int64{trechos[0].ID, 9999})
	if err != nil || len(ordenados) != 1 {
		t.Error(ordenados, err)
	}
	if v, err := a.Trechos(ctx, nil); err != nil || len(v) != 0 {
		t.Error(v, err)
	}

	cols, _ := a.Colecoes(ctx)
	if len(cols) != 1 || cols[0].Nome != "Contratos" || cols[0].CriadaPor != "ia" {
		t.Fatalf("%+v", cols)
	}
	meu, err := a.CriarColecao(ctx, "  Minha   casa ")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.NaColecao(ctx, meu, id, true); err != nil {
		t.Fatal(err)
	}
	if err := a.NaColecao(ctx, meu, 9999, true); !errors.Is(err, acervo.ErrNaoEncontrado) {
		t.Error(err)
	}
	// Assumir uma coleção da IA a torna do usuário.
	idIA, err := a.CriarColecao(ctx, "Contratos")
	if err != nil || idIA != cols[0].ID {
		t.Error(idIA, err)
	}
	for _, nome := range []string{" ", strings.Repeat("x", 61)} {
		if _, err := a.CriarColecao(ctx, nome); !errors.Is(err, acervo.ErrNomeInvalido) {
			t.Error(err)
		}
	}
	lista, err := a.Listar(ctx, acervo.Filtro{Colecao: meu})
	if err != nil || len(lista) != 1 || lista[0].ID != id {
		t.Error(lista, err)
	}
	if err := a.NaColecao(ctx, meu, id, false); err != nil {
		t.Fatal(err)
	}
	if l, _ := a.Listar(ctx, acervo.Filtro{Colecao: meu}); len(l) != 0 {
		t.Error(l)
	}
	ents, err := a.Entidades(ctx, "valor", "900", 0)
	if err != nil || len(ents) != 1 || ents[0].Documentos != 1 {
		t.Fatal(ents, err)
	}
	if l, _ := a.Listar(ctx, acervo.Filtro{Entidade: "valor:" + ents[0].Valor, Limite: 10}); len(l) != 1 {
		t.Error(l)
	}
	if l, _ := a.Listar(ctx, acervo.Filtro{Nome: "recei"}); len(l) != 1 {
		t.Error(l)
	}
	for _, curinga := range []string{"%", "_", `\`} {
		if l, err := a.Listar(ctx, acervo.Filtro{Nome: curinga}); err != nil || len(l) != 0 {
			t.Error(curinga, l, err)
		}
	}
	if err := a.Reextrair(ctx, ruim); err != nil {
		t.Fatal(err)
	}
	if err := a.Reextrair(ctx, 9999); !errors.Is(err, acervo.ErrNaoEncontrado) {
		t.Error(err)
	}
}

func TestVerificarRegenerarExportar(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{})
	a, ctx := s.D.Acervo, s.Contexto
	id := s.Importar("contrato.txt", contrato)
	meu, _ := a.CriarColecao(ctx, "Casa")
	if err := a.NaColecao(ctx, meu, id, true); err != nil {
		t.Fatal(err)
	}
	d, _ := a.Documento(ctx, id)
	antes, _ := os.ReadFile(a.CaminhoOriginal(d.SHA256))
	if err := a.Regenerar(ctx); err != nil {
		t.Fatal(err)
	}
	depois, _ := os.ReadFile(a.CaminhoOriginal(d.SHA256))
	n, problemas, err := a.Verificar(ctx)
	if err != nil || n != 1 || len(problemas) != 0 || !bytes.Equal(antes, depois) {
		t.Fatal(n, problemas, err)
	}
	if achados, _ := a.Buscar(ctx, 0, "aluguel", 5); len(achados) != 1 {
		t.Error("os índices não foram refeitos", achados)
	}
	if c, _ := a.Colecoes(ctx); len(c) != 2 {
		t.Error("coleções do usuário preservadas e as da IA refeitas", c)
	}

	var b bytes.Buffer
	if err := a.Exportar(ctx, &b, map[string]any{"extra.json": []int{1}}); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	partes := map[string][]byte{}
	for _, f := range z.File {
		r, _ := f.Open()
		partes[f.Name], _ = io.ReadAll(r)
		r.Close()
	}
	var m acervo.Manifesto
	if err := json.Unmarshal(partes["metadados.json"], &m); err != nil {
		t.Fatal(err)
	}
	doc := m.Documentos[0]
	if string(partes[doc.Arquivo]) != contrato || len(doc.Entidades) == 0 || len(doc.Colecoes) != 2 || partes["extra.json"] == nil {
		t.Errorf("%+v %v", doc, partes)
	}
	if a.Exportar(ctx, escritorRuim{}, nil) == nil {
		t.Error("destino com falha")
	}
	if a.Exportar(ctx, io.Discard, map[string]any{"x": func() {}}) == nil {
		t.Error("extra não codificável")
	}

	// Um original adulterado interrompe a regeneração.
	caminho := a.CaminhoOriginal(d.SHA256)
	os.Chmod(caminho, 0o600)
	os.WriteFile(caminho, []byte("adulterado"), 0o600)
	_, problemas, _ = a.Verificar(ctx)
	if len(problemas) != 1 || !strings.Contains(problemas[0].Motivo, "mudou") {
		t.Error(problemas)
	}
	if err := a.Regenerar(ctx); !errors.Is(err, acervo.ErrOriginais) {
		t.Error(err)
	}
	if trechos, _ := a.TrechosDoDocumento(ctx, id); len(trechos) != 1 {
		t.Error("nada deveria ter sido descartado")
	}
	os.Remove(caminho)
	if _, p, _ := a.Verificar(ctx); len(p) != 1 {
		t.Error(p)
	}
	if a.Exportar(ctx, io.Discard, nil) == nil {
		t.Error("exportação sem o original")
	}
	cancelado, cancelar := context.WithCancel(ctx)
	cancelar()
	if _, _, err := a.Verificar(cancelado); err == nil {
		t.Error("cancelado")
	}
}

type escritorRuim struct{}

func (escritorRuim) Write([]byte) (int, error) { return 0, errors.New("cheio") }

func TestSemModeloOuComModeloRuim(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{SemChave: true})
	a, ctx := s.D.Acervo, s.Contexto
	id := s.Importar("c.txt", contrato)
	d, _ := a.Documento(ctx, id)
	if d.Estado != acervo.Pronto || d.Resumo != "" {
		t.Errorf("%+v", d)
	}
	if achados, err := a.Buscar(ctx, 0, "aluguel", 5); err != nil || len(achados) != 1 {
		t.Error("sem chave a busca textual continua", achados, err)
	}
	// Com a chave, os derivados que faltavam são criados.
	if err := s.D.DefinirSegredo(ctx, segredos.OpenRouter, testes.Chave); err != nil {
		t.Fatal(err)
	}
	s.Modelo.Definir(func(p simulador.PedidoRecebido) (simulador.Resposta, bool) {
		return simulador.Resposta{Texto: "não é json"}, p.Esquema() == "enriquecimento"
	})
	if err := a.Processar(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := a.Numeros(ctx); n.Vetores != 1 {
		t.Error(n)
	}
	// Uma resposta ruim não é pedida para sempre.
	antes := len(s.Modelo.Pedidos())
	if err := a.Processar(ctx); err != nil || len(s.Modelo.Pedidos()) != antes {
		t.Error(err, len(s.Modelo.Pedidos())-antes)
	}
	// Sem rede para a consulta, vale a busca textual.
	s.Modelo.Definir(nil)
	if err := s.D.DefinirSegredo(ctx, segredos.OpenRouter, "recusada"); err != nil {
		t.Fatal(err)
	}
	if achados, err := a.Buscar(ctx, 0, "aluguel", 5); err != nil || len(achados) != 1 {
		t.Error(achados, err)
	}
}

type executorQuebrado struct{}

func (executorQuebrado) Abrir(context.Context, ferramentas.Execucao) (ferramentas.Sessao, error) {
	return nil, errors.New("podman ausente")
}

func TestFerramentaIndisponivelFicaRegistrada(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{Executor: executorQuebrado{}})
	id := s.Importar("c.txt", contrato)
	d, _ := s.D.Acervo.Documento(s.Contexto, id)
	if d.Estado != acervo.Erro || !strings.Contains(d.Erro, "podman ausente") {
		t.Errorf("%+v", d)
	}
}

func TestDividir(t *testing.T) {
	if p := acervo.Dividir("  \n "); len(p) != 0 {
		t.Error(p)
	}
	frases := strings.Repeat("Uma frase de teste com algumas palavras. ", 200)
	partes := acervo.Dividir(frases)
	if len(partes) < 5 {
		t.Fatal(len(partes))
	}
	for _, p := range partes {
		if utf8.RuneCountInString(p) > 1400 || !utf8.ValidString(p) {
			t.Error("trecho inválido", len(p))
		}
	}
	// Sem espaços também termina.
	bloco := strings.Repeat("ç", 5000)
	total := 0
	for _, p := range acervo.Dividir(bloco) {
		total += utf8.RuneCountInString(p)
	}
	if total < 5000 {
		t.Error(total)
	}
	if c := acervo.ConsultaFTS("O contrato, o CONTRATO e a luz!"); c != `"contrato"* OR "luz"` {
		t.Error(c)
	}
	if c := acervo.ConsultaFTS("o que é"); c == "" {
		t.Error("só palavras vazias ainda consultam")
	}
	if c := acervo.ConsultaFTS(strings.Repeat("a1 b2 c3 d4 e5 f6 g7 h8 i9 j10 k11 l12 m13 ", 1) + "n14 o15 p16 q17 r18 s19 t20 u21 v22 w23 x24 y25 z26"); strings.Count(c, " OR ") != 23 {
		t.Error(c)
	}
}
