package relatorio_test

import (
	"strings"
	"testing"

	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/testes"
)

func TestRelatorioPesquisaNaWeb(t *testing.T) {
	s := testes.Novo(t, testes.Opcoes{Busca: true})
	x := s.Criar("relatorio", "Prazos de recurso administrativo", "")
	if x.Estado != atividades.Pronta {
		t.Fatal(x.Estado, x.Aviso)
	}
	d := dados(t, x)
	var daWeb int
	for _, f := range d.Fontes {
		if f.Tipo == "url" && strings.HasSuffix(f.URL, "/pagina") {
			daWeb++
		}
	}
	if daWeb == 0 {
		t.Fatalf("a pesquisa não virou fonte: %+v", d.Fontes)
	}
	entradas, err := s.D.Diario.Ultimas(200, 0)
	if err != nil {
		t.Fatal(err)
	}
	var consultas int
	for _, e := range entradas {
		if e.Nome == "pesquisar_web" && e.Decisao == "permitir" {
			consultas++
		}
	}
	if consultas == 0 {
		t.Error("as consultas não foram registradas")
	}
}
