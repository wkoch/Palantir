package atividades

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wkoch/Palantir/internal/banco"
	"github.com/wkoch/Palantir/internal/modelos"
	"github.com/wkoch/Palantir/internal/orcamento"
	"github.com/wkoch/Palantir/internal/proveniencia"
)

type falso struct {
	mu        sync.Mutex
	rodar     func(ctx context.Context, id int64) (Resultado, error)
	vezes     int
	efeito    Efeito
	errAgir   error
	errNascer error
}

func (f *falso) Iniciar(_ context.Context, x *Atividade, o json.RawMessage) error {
	if string(o) == `"sem-titulo"` {
		x.Titulo = ""
	}
	return f.errNascer
}

func (f *falso) Executar(ctx context.Context, id int64) (Resultado, error) {
	f.mu.Lock()
	f.vezes++
	rodar := f.rodar
	f.mu.Unlock()
	if rodar == nil {
		return Resultado{}, nil
	}
	return rodar(ctx, id)
}

func (f *falso) Agir(context.Context, Atividade, string, json.RawMessage) (Efeito, error) {
	return f.efeito, f.errAgir
}

func (f *falso) Tela(context.Context, Atividade) (string, any, error) { return "t", 1, nil }

func (f *falso) contagem() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vezes
}

func novo(t *testing.T) (*Gerente, *falso, string) {
	t.Helper()
	caminho := filepath.Join(t.TempDir(), "p.db")
	db, err := banco.Abrir(context.Background(), caminho)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	g := NovoGerente(&Armazem{DB: db}, 2, func() float64 { return 3 })
	f := &falso{}
	g.Registrar("t", f)
	t.Cleanup(g.Parar)
	return g, f, caminho
}

func estado(t *testing.T, g *Gerente, id int64) Atividade {
	t.Helper()
	x, err := g.Armazem.Obter(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func TestCriarEResultados(t *testing.T) {
	g, f, _ := novo(t)
	ctx := context.Background()
	casos := []struct {
		res    Resultado
		err    error
		panico bool
		quer   Estado
		aviso  string
	}{
		{quer: Pronta},
		{res: Resultado{Esperando, "pergunta"}, quer: Esperando, aviso: "pergunta"},
		{err: orcamento.ErrEsgotado, quer: Pausada, aviso: "orçamento esgotado"},
		{err: modelos.ErrSemChave, quer: Esperando, aviso: "Configure a chave"},
		{err: modelos.ErrSemRede, quer: Esperando, aviso: "Retome quando"},
		{err: errors.New("quebrou"), quer: Esperando, aviso: "Falha: quebrou"},
		{panico: true, quer: Esperando, aviso: "falha interna"},
	}
	for _, c := range casos {
		f.rodar = func(context.Context, int64) (Resultado, error) {
			if c.panico {
				panic("ops")
			}
			return c.res, c.err
		}
		x, err := g.Criar(ctx, "t", "", "  objetivo\n com   espaços ", nil)
		if err != nil {
			t.Fatal(err)
		}
		if x.Titulo != "objetivo com espaços" || x.OrcamentoUSD != 3 {
			t.Error(x.Titulo, x.OrcamentoUSD)
		}
		g.Esperar()
		y := estado(t, g, x.ID)
		if y.Estado != c.quer || !strings.Contains(y.Aviso, c.aviso) {
			t.Errorf("%v: %s %q", c.err, y.Estado, y.Aviso)
		}
		eventos, err := g.Armazem.Eventos(ctx, x.ID)
		if err != nil || eventos[0].Origem != proveniencia.Usuario || (c.err != nil && len(eventos) != 2) {
			t.Error(eventos, err)
		}
	}
	longo, err := g.Criar(ctx, "t", strings.Repeat("t", 300), "", nil)
	if err != nil || len([]rune(longo.Titulo)) != maxTitulo {
		t.Error(len(longo.Titulo), err)
	}
}

func TestCriarRecusas(t *testing.T) {
	g, f, _ := novo(t)
	ctx := context.Background()
	if _, err := g.Criar(ctx, "outro", "", "x", nil); !errors.Is(err, ErrPedidoInvalido) {
		t.Error(err)
	}
	if _, err := g.Criar(ctx, "t", "", strings.Repeat("a", maxObjetivo+1), nil); !errors.Is(err, ErrPedidoInvalido) {
		t.Error(err)
	}
	if _, err := g.Criar(ctx, "t", "", "", nil); !errors.Is(err, ErrPedidoInvalido) {
		t.Error(err)
	}
	f.errNascer = Invalido("não")
	if _, err := g.Criar(ctx, "t", "a", "b", nil); !errors.Is(err, ErrPedidoInvalido) {
		t.Error(err)
	}
	if l, err := g.Armazem.Listar(ctx, true); err != nil || len(l) != 0 {
		t.Error(l, err)
	}
}

func TestPausaRetomadaEReinicio(t *testing.T) {
	g, f, _ := novo(t)
	ctx := context.Background()
	comecou := make(chan struct{}, 10)
	f.rodar = func(ctx context.Context, _ int64) (Resultado, error) {
		comecou <- struct{}{}
		<-ctx.Done()
		return Resultado{}, ctx.Err()
	}
	x, err := g.Criar(ctx, "t", "a", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	<-comecou
	if _, err := g.Mudar(ctx, x.ID, Pausada); err != nil {
		t.Fatal(err)
	}
	g.Esperar()
	if estado(t, g, x.ID).Estado != Pausada {
		t.Fatal("a pausa prevalece sobre o fim da execução")
	}
	// Retomar volta a executar.
	if _, err := g.Mudar(ctx, x.ID, Rodando); err != nil {
		t.Fatal(err)
	}
	<-comecou
	// Desligar preserva "rodando", e um gerente novo retoma.
	g.Parar()
	if estado(t, g, x.ID).Estado != Rodando {
		t.Fatal("o estado deveria ser preservado")
	}
	antes := f.contagem()
	g.Agendar(x.ID) // parado: ignora
	g2 := NovoGerente(g.Armazem, 0, g.OrcamentoUSD)
	f2 := &falso{}
	g2.Registrar("t", f2)
	defer g2.Parar()
	if err := g2.Retomar(ctx); err != nil {
		t.Fatal(err)
	}
	g2.Esperar()
	if f2.contagem() != 1 || f.contagem() != antes || estado(t, g2, x.ID).Estado != Pronta {
		t.Error(f2.contagem(), estado(t, g2, x.ID).Estado)
	}
}

func TestSegundoPedidoRodaDeNovo(t *testing.T) {
	g, f, _ := novo(t)
	libera := make(chan struct{})
	comecou := make(chan struct{}, 10)
	f.rodar = func(context.Context, int64) (Resultado, error) {
		comecou <- struct{}{}
		<-libera
		return Resultado{}, nil
	}
	x, err := g.Criar(context.Background(), "t", "a", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	<-comecou
	g.Agendar(x.ID)
	g.Agendar(x.ID)
	close(libera)
	g.Esperar()
	if f.contagem() != 2 {
		t.Error("esperava exatamente mais uma execução:", f.contagem())
	}
}

func TestLimiteDeSimultaneas(t *testing.T) {
	g, f, _ := novo(t)
	var mu sync.Mutex
	atual, pico := 0, 0
	f.rodar = func(context.Context, int64) (Resultado, error) {
		mu.Lock()
		atual++
		pico = max(pico, atual)
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		atual--
		mu.Unlock()
		return Resultado{}, nil
	}
	for range 6 {
		if _, err := g.Criar(context.Background(), "t", "a", "", nil); err != nil {
			t.Fatal(err)
		}
	}
	g.Esperar()
	if pico != 2 || f.contagem() != 6 {
		t.Error(pico, f.contagem())
	}
}

func TestAcoesEOrcamento(t *testing.T) {
	g, f, _ := novo(t)
	ctx := context.Background()
	x, _ := g.Criar(ctx, "t", "a", "", nil)
	g.Esperar()
	f.efeito = Efeito{Resposta: 5}
	r, err := g.Agir(ctx, x.ID, "a", nil)
	if err != nil || r != 5 || f.contagem() != 1 {
		t.Fatal(r, err)
	}
	if _, err := g.Mudar(ctx, x.ID, Pausada); err != nil {
		t.Fatal(err)
	}
	f.efeito = Efeito{Executar: true}
	if _, err := g.Agir(ctx, x.ID, "a", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	g.Esperar()
	if f.contagem() != 2 || estado(t, g, x.ID).Estado != Pronta {
		t.Error("a ação deveria retomar a atividade pausada")
	}
	f.errAgir = errors.New("x")
	if _, err := g.Agir(ctx, x.ID, "a", nil); err == nil {
		t.Error("erro da ação")
	}
	if _, err := g.Agir(ctx, 999, "a", nil); !errors.Is(err, ErrNaoEncontrada) {
		t.Error(err)
	}
	if y, err := g.DefinirOrcamento(ctx, x.ID, 7); err != nil || y.OrcamentoUSD != 7 {
		t.Error(y, err)
	}
	for _, v := range []float64{-1, 20000} {
		if _, err := g.DefinirOrcamento(ctx, x.ID, v); !errors.Is(err, ErrPedidoInvalido) {
			t.Error(v, err)
		}
	}
	if _, err := g.DefinirOrcamento(ctx, 999, 1); !errors.Is(err, ErrNaoEncontrada) {
		t.Error(err)
	}
	if _, err := g.Mudar(ctx, x.ID, Pronta); !errors.Is(err, ErrPedidoInvalido) {
		t.Error(err)
	}
	if _, err := g.Mudar(ctx, x.ID, Arquivada); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Mudar(ctx, x.ID, Pausada); !errors.Is(err, ErrEstado) {
		t.Error(err)
	}
	if _, err := g.Agir(ctx, x.ID, "a", nil); !errors.Is(err, ErrEstado) {
		t.Error(err)
	}
	// Uma arquivada não volta a rodar sozinha.
	antes := f.contagem()
	g.Agendar(x.ID)
	g.Agendar(999)
	g.Esperar()
	if f.contagem() != antes {
		t.Error("executou uma atividade arquivada")
	}
	if l, _ := g.Armazem.Listar(ctx, false); len(l) != 0 {
		t.Error(l)
	}
	if nome, v, err := g.Tela(ctx, x); err != nil || nome != "t" || v != 1 {
		t.Error(nome, v, err)
	}
	if _, _, err := g.Tela(ctx, Atividade{Tipo: "?"}); err == nil {
		t.Error("tipo desconhecido")
	}
}

func TestModificarEDecodificar(t *testing.T) {
	g, _, _ := novo(t)
	ctx := context.Background()
	x := Atividade{Tipo: "t", Titulo: "a", Estado: Pronta}
	if err := g.Armazem.Inserir(ctx, &x); err != nil {
		t.Fatal(err)
	}
	type d struct{ N int }
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, err := Modificar(ctx, g.Armazem, x.ID, func(v *d) error { v.N++; return nil }); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	v, err := Dados[d](estado(t, g, x.ID))
	if err != nil || v.N != 20 {
		t.Fatal(v, err)
	}
	if _, err := Modificar(ctx, g.Armazem, x.ID, func(*d) error { return errors.New("não") }); err == nil {
		t.Error("erro propagado")
	}
	if _, err := Modificar(ctx, g.Armazem, x.ID, func(*[]int) error { return nil }); err == nil {
		t.Error("tipo incompatível")
	}
	if _, err := Dados[[]int](estado(t, g, x.ID)); err == nil {
		t.Error("tipo incompatível")
	}
	if _, err := Decodificar[d](json.RawMessage(`{"N":1,"x":2}`)); !errors.Is(err, ErrPedidoInvalido) {
		t.Error(err)
	}
	if !Indisponivel(orcamento.ErrEsgotado) || !Indisponivel(modelos.ErrSemRede) || Indisponivel(ErrEstado) {
		t.Error("indisponibilidade")
	}
	g.Armazem.DB.Close()
	if g.Retomar(ctx) == nil {
		t.Error("banco fechado")
	}
	g.Agendar(x.ID)
	g.Esperar()
}
