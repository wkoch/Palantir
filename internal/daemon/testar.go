package daemon

import (
	"context"
	"errors"

	"github.com/wkoch/Palantir/internal/esquema"
	"github.com/wkoch/Palantir/internal/modelos"
)

// Teste é o resultado de uma chamada de conferência a um modelo.
type Teste struct {
	Nome     string
	Modelo   string
	CustoUSD float64
	Erro     error
}

// TestarModelos faz uma chamada pequena e real a cada nível, com saída
// estruturada, uso de ferramenta e embeddings, para conferir a configuração.
func (d *Daemon) TestarModelos(ctx context.Context) []Teste {
	formato := esquema.JSON(esquema.Objeto(map[string]any{"ok": esquema.Booleano}))
	pergunta := func(texto string) []modelos.Mensagem {
		return []modelos.Mensagem{{Papel: modelos.PapelSistema, Conteudo: "Você responde testes de conexão."},
			{Papel: modelos.PapelUsuario, Conteudo: texto}}
	}
	var testes []Teste
	for _, nivel := range []modelos.Nivel{modelos.Barato, modelos.Principal, modelos.Forte} {
		r, err := d.Roteador.Completar(ctx, modelos.Pedido{Nivel: nivel, Finalidade: "diagnostico", MaxTokens: 1000,
			Mensagens: pergunta("Responda com ok verdadeiro."),
			Esquema:   &modelos.Esquema{Nome: "teste_de_conexao", Definicao: formato}})
		if err == nil {
			var v *esquema.Validador
			if v, err = esquema.Compilar("teste", formato); err == nil {
				err = v.ValidarJSON([]byte(r.Conteudo))
			}
		}
		testes = append(testes, Teste{"modelo " + string(nivel), r.Modelo, r.CustoUSD, err})
	}
	r, err := d.Roteador.Completar(ctx, modelos.Pedido{Nivel: modelos.Principal, Finalidade: "diagnostico",
		MaxTokens: 1000, Mensagens: pergunta("Este é um teste de ferramenta: chame teste_de_conexao com ok verdadeiro."),
		Ferramentas: []modelos.Ferramenta{{Nome: "teste_de_conexao", Descricao: "Confirma a conexão.", Esquema: formato}}})
	if err == nil && len(r.Chamadas) == 0 {
		err = errors.New("o modelo não usou a ferramenta pedida")
	}
	testes = append(testes, Teste{"ferramentas do modelo", r.Modelo, r.CustoUSD, err})
	if d.Config().Modelos.Decisoes != "" {
		antes, _ := d.Livro.NoMes(ctx)
		_, err := d.Roteador.Decidir(ctx, 0, "diagnostico", "Bom dia!", map[string]modelos.Pergunta{
			"cumprimento": {Tipo: modelos.SimOuNao, Instrucoes: "O texto é um cumprimento?",
				Criterios: map[string]string{"true": "Sim", "false": "Não"}}})
		depois, _ := d.Livro.NoMes(ctx)
		testes = append(testes, Teste{"decisões", d.Config().Modelos.Decisoes, depois - antes, err})
	}
	if d.Config().Modelos.Embeddings != "" {
		_, modelo, err := d.Roteador.Embeddings(ctx, 0, []string{"teste"})
		testes = append(testes, Teste{Nome: "busca semântica", Modelo: modelo, Erro: err})
	}
	return testes
}
