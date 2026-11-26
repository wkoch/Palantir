// Package conversa é a atividade de conversa com o acervo e a web.
package conversa

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wkoch/Palantir/internal/agente"
	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/confirmacoes"
	"github.com/wkoch/Palantir/internal/modelos"
	"github.com/wkoch/Palantir/internal/politica"
	"github.com/wkoch/Palantir/internal/proveniencia"
)

// Dados é o estado de uma conversa.
type Dados struct {
	Forte bool          `json:"forte"`
	Itens []agente.Item `json:"itens"`
	// Respondido é o identificador do último item que já foi atendido.
	Respondido int `json:"respondido"`
}

func (d *Dados) acrescentar(it agente.Item) {
	it.ID = len(d.Itens) + 1
	if it.Quando.IsZero() {
		it.Quando = time.Now().UTC()
	}
	d.Itens = append(d.Itens, it)
}

// pendente informa se há algo que o modelo ainda não respondeu.
func (d Dados) pendente() bool {
	for _, it := range d.Itens {
		// Fala do usuário ou resultado de uma ação confirmada ou recusada.
		if it.ID > d.Respondido && it.Papel == modelos.PapelUsuario {
			return true
		}
	}
	return false
}

// Executor implementa atividades.Executor.
type Executor struct {
	Armazem *atividades.Armazem
	Agente  *agente.Agente
}

const maxMensagem = 40_000

func mensagemValida(texto string) (string, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return "", atividades.Invalido("a mensagem está vazia")
	}
	if utf8.RuneCountInString(texto) > maxMensagem {
		return "", atividades.Invalido("mensagem longa demais; importe o texto no acervo")
	}
	return texto, nil
}

// Iniciar implementa atividades.Executor.
func (e *Executor) Iniciar(_ context.Context, x *atividades.Atividade, opcoes json.RawMessage) error {
	var o struct {
		Forte bool `json:"forte"`
	}
	if len(opcoes) > 0 {
		var err error
		if o, err = atividades.Decodificar[struct {
			Forte bool `json:"forte"`
		}](opcoes); err != nil {
			return err
		}
	}
	texto, err := mensagemValida(x.Objetivo)
	if err != nil {
		return err
	}
	d := Dados{Forte: o.Forte}
	d.acrescentar(agente.Item{Origem: proveniencia.Usuario, Papel: modelos.PapelUsuario, Conteudo: texto})
	x.Dados, err = json.Marshal(d)
	return err
}

const sistema = `Você é o assistente do Palantír, um sistema pessoal de um único usuário. Responda em português
do Brasil, de forma direta. Para perguntas sobre os documentos do usuário, procure no acervo antes de
responder e cite cada informação com a marca da fonte entre colchetes, como [T12]. Se não encontrar, diga
que não encontrou; não invente. Use a ferramenta mostrar_tela quando indicadores ou uma tabela explicarem
melhor que o texto. Você não envia, apaga, publica nem paga nada.`

// Executar implementa atividades.Executor.
func (e *Executor) Executar(ctx context.Context, id int64) (atividades.Resultado, error) {
	x, err := e.Armazem.Obter(ctx, id)
	if err != nil {
		return atividades.Resultado{}, err
	}
	d, err := atividades.Dados[Dados](x)
	if err != nil {
		return atividades.Resultado{}, err
	}
	if !d.pendente() {
		return atividades.Resultado{Estado: atividades.Pronta}, nil
	}
	historico := fecharChamadas(d.Itens)
	ate := historico[len(historico)-1].ID
	nivel := modelos.Principal
	if d.Forte {
		nivel = modelos.Forte
	}
	res, err := e.Agente.Executar(ctx, agente.Pedido{Atividade: id, Tipo: politica.Conversa, Nivel: nivel,
		Sistema: sistema + "\nHoje é " + time.Now().Format("02/01/2006") + ".", Historico: historico},
		func(it agente.Item) error {
			_, err := atividades.Modificar(ctx, e.Armazem, id, func(d *Dados) error {
				d.acrescentar(it)
				return nil
			})
			return err
		})
	if err != nil {
		return atividades.Resultado{}, err
	}
	// O que o usuário escreveu durante a rodada continua pendente.
	if _, err := atividades.Modificar(ctx, e.Armazem, id, func(d *Dados) error {
		d.Respondido = max(d.Respondido, ate)
		return nil
	}); err != nil {
		return atividades.Resultado{}, err
	}
	if res.AguardandoConfirmacao {
		return atividades.Resultado{Estado: atividades.Esperando, Aviso: "Uma ação aguarda a sua confirmação."}, nil
	}
	return atividades.Resultado{Estado: atividades.Pronta}, nil
}

// fecharChamadas reordena o histórico para o modelo: cada pedido de ferramenta
// é seguido dos seus resultados, e chamadas interrompidas recebem um aviso.
// Uma fala do usuário feita durante a rodada vem depois deles.
func fecharChamadas(itens []agente.Item) []agente.Item {
	resultados := map[string]agente.Item{}
	for _, it := range itens {
		if it.Papel == modelos.PapelFerramenta {
			resultados[it.ChamadaID] = it
		}
	}
	saida := make([]agente.Item, 0, len(itens))
	for _, it := range itens {
		if it.Papel == modelos.PapelFerramenta {
			continue
		}
		saida = append(saida, it)
		for _, c := range it.Chamadas {
			r, ok := resultados[c.ID]
			if !ok {
				r = agente.Item{ID: it.ID, Origem: proveniencia.Sistema, Papel: modelos.PapelFerramenta,
					ChamadaID: c.ID, Fonte: c.Nome, Conteudo: "A execução foi interrompida antes do resultado."}
			}
			saida = append(saida, r)
		}
	}
	// O identificador do último item define o que ficou respondido.
	if n := len(saida); n > 0 {
		maior := 0
		for _, it := range itens {
			maior = max(maior, it.ID)
		}
		saida[n-1].ID = maior
	}
	return saida
}

// Agir implementa atividades.Executor.
func (e *Executor) Agir(ctx context.Context, x atividades.Atividade, acao string, corpo json.RawMessage) (atividades.Efeito, error) {
	switch acao {
	case "mensagem":
		in, err := atividades.Decodificar[struct {
			Texto string `json:"texto"`
			Forte bool   `json:"forte"`
		}](corpo)
		if err != nil {
			return atividades.Efeito{}, err
		}
		texto, err := mensagemValida(in.Texto)
		if err != nil {
			return atividades.Efeito{}, err
		}
		_, err = atividades.Modificar(ctx, e.Armazem, x.ID, func(d *Dados) error {
			d.Forte = in.Forte
			d.acrescentar(agente.Item{Origem: proveniencia.Usuario, Papel: modelos.PapelUsuario, Conteudo: texto})
			return nil
		})
		return atividades.Efeito{Executar: true}, err
	case atividades.ConfirmacaoDecidida:
		in, err := atividades.Decodificar[confirmacoes.Confirmacao](corpo)
		if err != nil {
			return atividades.Efeito{}, err
		}
		it := agente.Item{Origem: proveniencia.Sistema, Papel: modelos.PapelUsuario,
			Conteudo: "O usuário recusou a chamada de " + in.Ferramenta + ". Siga sem ela."}
		if in.Estado == confirmacoes.Aprovada {
			if it, err = e.Agente.ExecutarAprovada(ctx, politica.Conversa, in); err != nil {
				return atividades.Efeito{}, err
			}
		}
		_, err = atividades.Modificar(ctx, e.Armazem, x.ID, func(d *Dados) error {
			d.acrescentar(it)
			return nil
		})
		return atividades.Efeito{Executar: true}, err
	}
	return atividades.Efeito{}, atividades.Invalido("ação desconhecida: %q", acao)
}

// Tela implementa atividades.Executor.
func (e *Executor) Tela(_ context.Context, x atividades.Atividade) (string, any, error) {
	d, err := atividades.Dados[Dados](x)
	if err != nil {
		return "", nil, err
	}
	if d.Itens == nil {
		d.Itens = []agente.Item{}
	}
	return "conversa", d, nil
}
