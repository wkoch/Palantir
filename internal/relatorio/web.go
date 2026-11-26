package relatorio

import (
	"context"
	"strings"

	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/diario"
	"github.com/wkoch/Palantir/internal/politica"
	"github.com/wkoch/Palantir/internal/proveniencia"
)

const (
	maxConsultas       = 2
	paginasPorConsulta = 2
)

type resultadoDaWeb struct {
	// fontes são os trechos devolvidos pelo buscador.
	fontes []Fonte
	avisos []string
}

// pesquisarNaWeb faz as pesquisas de uma seção. As consultas vêm do plano, que
// foi feito sem conteúdo externo, ou do que o usuário escreveu num comentário.
// Devolve os endereços que valem a leitura.
func (e *Executor) pesquisarNaWeb(ctx context.Context, x atividades.Atividade, d Dados, s Secao,
	buscas []string) ([]string, resultadoDaWeb, error) {
	var ler []string
	var r resultadoDaWeb
	if !e.Web.Pesquisa() {
		return nil, r, nil
	}
	// Os comentários, no fim da lista, são o pedido mais recente.
	if len(buscas) > maxConsultas {
		buscas = append(buscas[:maxConsultas-1:maxConsultas-1], buscas[len(buscas)-1])
	}
	vistos := map[string]bool{}
	for _, consulta := range buscas {
		decisao := politica.Avaliar(politica.Pedido{Tipo: politica.Relatorio, Ferramenta: politica.PesquisarWeb,
			Consulta: consulta, TextosUsuario: textosDoUsuario(x, d)})
		if err := e.Diario.Registrar(diario.Entrada{Atividade: x.ID, Tipo: "ferramenta", Nome: politica.PesquisarWeb,
			Origem: string(proveniencia.Modelo), Decisao: string(decisao.Veredito),
			Motivo: strings.Join(decisao.Motivos, "; "), Detalhe: consulta}); err != nil {
			return nil, r, err
		}
		if decisao.Veredito != politica.Permitir {
			continue
		}
		achados, err := e.Web.Pesquisar(ctx, consulta)
		if err != nil {
			if ctx.Err() != nil {
				return nil, r, ctx.Err()
			}
			r.avisos = []string{"a pesquisa na web falhou"}
			continue
		}
		for i, it := range achados.Itens {
			if vistos[it.URL] {
				continue
			}
			vistos[it.URL] = true
			if i < paginasPorConsulta {
				ler = append(ler, it.URL)
			}
			if it.Trecho != "" {
				r.fontes = append(r.fontes, Fonte{Tipo: "url", Titulo: it.Titulo, URL: it.URL, Texto: it.Trecho})
			}
		}
	}
	return ler, r, nil
}
