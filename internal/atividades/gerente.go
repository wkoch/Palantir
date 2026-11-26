package atividades

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/wkoch/Palantir/internal/modelos"
	"github.com/wkoch/Palantir/internal/orcamento"
	"github.com/wkoch/Palantir/internal/proveniencia"
)

// Resultado diz em que estado a atividade ficou depois de uma execução.
type Resultado struct {
	Estado Estado
	Aviso  string
}

// Efeito diz o que fazer depois de uma ação do usuário.
type Efeito struct {
	// Executar agenda o trabalho em segundo plano.
	Executar bool
	// Resposta é devolvida à interface.
	Resposta any
}

// Executor implementa um tipo de atividade.
type Executor interface {
	// Iniciar prepara os dados de uma atividade nova a partir das opções.
	Iniciar(ctx context.Context, x *Atividade, opcoes json.RawMessage) error
	// Executar faz o trabalho pendente em segundo plano.
	Executar(ctx context.Context, id int64) (Resultado, error)
	// Agir aplica uma ação pedida pelo usuário na interface.
	Agir(ctx context.Context, x Atividade, acao string, corpo json.RawMessage) (Efeito, error)
	// Tela devolve o nome da tela e os dados dela.
	Tela(ctx context.Context, x Atividade) (string, any, error)
}

// ConfirmacaoDecidida é a ação interna disparada quando o usuário responde a
// uma confirmação. A interface não pode chamá-la diretamente.
const ConfirmacaoDecidida = "confirmacao_decidida"

// Erros de uso.
var (
	ErrPedidoInvalido = errors.New("pedido inválido")
	ErrEstado         = errors.New("a atividade não permite isso no estado atual")
)

// Invalido cria um erro causado pela entrada do usuário.
func Invalido(formato string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrPedidoInvalido, fmt.Sprintf(formato, args...))
}

type execucao struct {
	cancelar context.CancelFunc
	repetir  bool
}

// Gerente agenda as execuções e aplica as transições de estado.
type Gerente struct {
	Armazem      *Armazem
	OrcamentoUSD func() float64
	Log          *slog.Logger

	executores map[string]Executor
	vagas      chan struct{}

	mu       sync.Mutex
	ativas   map[int64]*execucao
	parado   bool
	ctx      context.Context
	cancelar context.CancelFunc
	grupo    sync.WaitGroup
}

// NovoGerente cria o gerente com um limite de execuções simultâneas.
func NovoGerente(a *Armazem, simultaneas int, orcamentoUSD func() float64) *Gerente {
	ctx, cancelar := context.WithCancel(context.Background())
	return &Gerente{Armazem: a, OrcamentoUSD: orcamentoUSD, Log: slog.Default(),
		executores: map[string]Executor{}, vagas: make(chan struct{}, max(1, simultaneas)),
		ativas: map[int64]*execucao{}, ctx: ctx, cancelar: cancelar}
}

// Registrar associa um executor a um tipo.
func (g *Gerente) Registrar(tipo string, e Executor) { g.executores[tipo] = e }

func (g *Gerente) executor(tipo string) (Executor, error) {
	e, ok := g.executores[tipo]
	if !ok {
		return nil, Invalido("tipo de atividade desconhecido: %q", tipo)
	}
	return e, nil
}

const (
	maxTitulo   = 120
	maxObjetivo = 200_000
)

func resumir(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n-1])) + "…"
}

// Criar grava uma atividade e inicia o trabalho.
func (g *Gerente) Criar(ctx context.Context, tipo, titulo, objetivo string, opcoes json.RawMessage) (Atividade, error) {
	var x Atividade
	e, err := g.executor(tipo)
	if err != nil {
		return x, err
	}
	objetivo = strings.TrimSpace(objetivo)
	if utf8.RuneCountInString(objetivo) > maxObjetivo {
		return x, Invalido("objetivo longo demais")
	}
	if titulo = strings.TrimSpace(titulo); titulo == "" {
		titulo = objetivo
	}
	x = Atividade{Tipo: tipo, Titulo: resumir(titulo, maxTitulo), Objetivo: objetivo, Estado: Rodando,
		OrcamentoUSD: g.OrcamentoUSD()}
	if err := e.Iniciar(ctx, &x, opcoes); err != nil {
		return x, err
	}
	if x.Titulo == "" {
		return x, Invalido("informe um título ou objetivo")
	}
	if err := g.Armazem.Inserir(ctx, &x); err != nil {
		return x, err
	}
	if x.Objetivo != "" {
		if err := g.Armazem.Registrar(ctx, x.ID, proveniencia.Usuario, "criada", x.Objetivo); err != nil {
			return x, err
		}
	}
	g.Agendar(x.ID)
	return x, nil
}

// Agendar executa a atividade assim que houver vaga. Se ela já estiver em
// execução, roda mais uma vez ao terminar.
func (g *Gerente) Agendar(id int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.parado {
		return
	}
	if e, ok := g.ativas[id]; ok {
		e.repetir = true
		return
	}
	ctx, cancelar := context.WithCancel(g.ctx)
	e := &execucao{cancelar: cancelar}
	g.ativas[id] = e
	g.grupo.Add(1)
	go g.rodar(ctx, id, e)
}

func (g *Gerente) rodar(ctx context.Context, id int64, e *execucao) {
	defer g.grupo.Done()
	for {
		g.umaVez(ctx, id)
		g.mu.Lock()
		if e.repetir && ctx.Err() == nil {
			e.repetir = false
			g.mu.Unlock()
			continue
		}
		if g.ativas[id] == e {
			delete(g.ativas, id)
		}
		g.mu.Unlock()
		e.cancelar()
		return
	}
}

var errIgnorar = errors.New("sem mudança")

func (g *Gerente) umaVez(ctx context.Context, id int64) {
	select {
	case g.vagas <- struct{}{}:
		defer func() { <-g.vagas }()
	case <-ctx.Done():
		return
	}
	x, err := g.Armazem.Atualizar(ctx, id, func(x *Atividade) error {
		if x.Estado == Pausada || x.Estado == Arquivada {
			return errIgnorar
		}
		x.Estado, x.Aviso = Rodando, ""
		return nil
	})
	if err != nil {
		if !errors.Is(err, errIgnorar) && ctx.Err() == nil {
			g.Log.Error("iniciar atividade", "atividade", id, "erro", err)
		}
		return
	}
	res, err := g.chamar(ctx, x)
	// Interrompida por pausa ou desligamento: o estado fica como está, e uma
	// atividade "rodando" é retomada na próxima partida.
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		res = traduzir(err)
		g.Log.Warn("atividade interrompida", "atividade", id, "erro", err)
		if errReg := g.Armazem.Registrar(ctx, id, proveniencia.Sistema, "erro", res.Aviso); errReg != nil {
			g.Log.Error("registrar erro", "erro", errReg)
		}
	}
	if res.Estado == "" {
		res.Estado = Pronta
	}
	_, err = g.Armazem.Atualizar(ctx, id, func(x *Atividade) error {
		if x.Estado != Rodando {
			return errIgnorar
		}
		x.Estado, x.Aviso = res.Estado, res.Aviso
		return nil
	})
	if err != nil && !errors.Is(err, errIgnorar) && ctx.Err() == nil {
		g.Log.Error("concluir atividade", "atividade", id, "erro", err)
	}
}

func (g *Gerente) chamar(ctx context.Context, x Atividade) (res Resultado, err error) {
	e, err := g.executor(x.Tipo)
	if err != nil {
		return res, err
	}
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("falha interna: %v", p)
		}
	}()
	return e.Executar(ctx, x.ID)
}

func traduzir(err error) Resultado {
	switch {
	case errors.Is(err, orcamento.ErrEsgotado):
		return Resultado{Estado: Pausada, Aviso: err.Error() + ". Aumente o limite para continuar."}
	case errors.Is(err, modelos.ErrSemChave):
		return Resultado{Estado: Esperando, Aviso: "Configure a chave do OpenRouter em Ajustes e retome a atividade."}
	case modelos.Indisponivel(err):
		return Resultado{Estado: Esperando, Aviso: err.Error() + ". Retome quando o serviço voltar."}
	}
	return Resultado{Estado: Esperando, Aviso: "Falha: " + err.Error()}
}

// Retomar agenda tudo o que estava em andamento quando o daemon parou.
func (g *Gerente) Retomar(ctx context.Context) error {
	lista, err := g.Armazem.Listar(ctx, false)
	if err != nil {
		return err
	}
	for _, x := range lista {
		if x.Estado == Rodando {
			g.Agendar(x.ID)
		}
	}
	return nil
}

func (g *Gerente) interromper(id int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if e, ok := g.ativas[id]; ok {
		e.cancelar()
		// Uma nova execução pode começar sem esperar a que foi cancelada.
		delete(g.ativas, id)
	}
}

// Mudar aplica um pedido de estado vindo do usuário: pausada, rodando
// (retomar) ou arquivada.
func (g *Gerente) Mudar(ctx context.Context, id int64, novo Estado) (Atividade, error) {
	x, err := g.Armazem.Atualizar(ctx, id, func(x *Atividade) error {
		switch novo {
		case Pausada:
			if x.Estado == Arquivada {
				return ErrEstado
			}
		case Rodando, Arquivada:
		default:
			return Invalido("estado %q não pode ser pedido", novo)
		}
		x.Estado, x.Aviso = novo, ""
		return nil
	})
	if err != nil {
		return x, err
	}
	if novo == Rodando {
		g.Agendar(id)
	} else {
		g.interromper(id)
	}
	return x, g.Armazem.Registrar(ctx, id, proveniencia.Usuario, "estado", string(novo))
}

// DefinirOrcamento muda o limite de gasto de uma atividade.
func (g *Gerente) DefinirOrcamento(ctx context.Context, id int64, usd float64) (Atividade, error) {
	if usd < 0 || usd > 10_000 {
		return Atividade{}, Invalido("orçamento fora do intervalo")
	}
	x, err := g.Armazem.Atualizar(ctx, id, func(x *Atividade) error {
		x.OrcamentoUSD = usd
		return nil
	})
	if err != nil {
		return x, err
	}
	return x, g.Armazem.Registrar(ctx, id, proveniencia.Usuario, "orcamento", fmt.Sprintf("US$ %.2f", usd))
}

// Agir encaminha uma ação da interface ao executor.
func (g *Gerente) Agir(ctx context.Context, id int64, acao string, corpo json.RawMessage) (any, error) {
	x, err := g.Armazem.Obter(ctx, id)
	if err != nil {
		return nil, err
	}
	if x.Estado == Arquivada {
		return nil, ErrEstado
	}
	e, err := g.executor(x.Tipo)
	if err != nil {
		return nil, err
	}
	if len(corpo) == 0 {
		corpo = json.RawMessage("{}")
	}
	efeito, err := e.Agir(ctx, x, acao, corpo)
	if err != nil {
		return nil, err
	}
	if efeito.Executar {
		// Uma ação do usuário que pede trabalho retoma uma atividade pausada.
		if _, err := g.Armazem.Atualizar(ctx, id, func(x *Atividade) error {
			x.Estado, x.Aviso = Rodando, ""
			return nil
		}); err != nil {
			return nil, err
		}
		g.Agendar(id)
	}
	return efeito.Resposta, nil
}

// Tela devolve a tela atual de uma atividade.
func (g *Gerente) Tela(ctx context.Context, x Atividade) (string, any, error) {
	e, err := g.executor(x.Tipo)
	if err != nil {
		return "", nil, err
	}
	return e.Tela(ctx, x)
}

// Esperar aguarda as execuções em andamento terminarem.
func (g *Gerente) Esperar() { g.grupo.Wait() }

// Parar cancela as execuções e aguarda. O estado "rodando" é preservado.
func (g *Gerente) Parar() {
	g.mu.Lock()
	g.parado = true
	g.mu.Unlock()
	g.cancelar()
	g.grupo.Wait()
}

// Decodificar lê o corpo de uma ação, recusando campos desconhecidos.
func Decodificar[T any](corpo json.RawMessage) (T, error) {
	var v T
	d := json.NewDecoder(strings.NewReader(string(corpo)))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return v, Invalido("%v", err)
	}
	return v, nil
}

// Indisponivel informa se o erro permite concluir parte do trabalho sem o
// modelo.
func Indisponivel(err error) bool {
	return modelos.Indisponivel(err) || errors.Is(err, orcamento.ErrEsgotado)
}
