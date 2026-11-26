package real_test

import (
	"os"
	"strings"
	"testing"

	"github.com/wkoch/Palantir/internal/modelos"
	"github.com/wkoch/Palantir/internal/testes"
)

// TestPesquisaNaWeb exige também um SearXNG em PALANTIR_BUSCA_URL, como o de
// deploy/busca. A consulta é inventada e vai aos buscadores públicos.
func TestPesquisaNaWeb(t *testing.T) {
	busca := os.Getenv("PALANTIR_BUSCA_URL")
	if busca == "" {
		t.Skip("defina PALANTIR_BUSCA_URL")
	}
	s := montar(t, testes.Opcoes{BuscaURL: busca, Config: ""})
	x := s.Criar("conversa", "Pesquise na web o que foi a Inconfidência Mineira e em que ano ocorreu. Cite as fontes.", "")
	pronta(t, x)
	var pesquisou bool
	for _, it := range conversaDe(t, x) {
		pesquisou = pesquisou || it.Papel == modelos.PapelFerramenta && len(it.Achados) > 0
		t.Logf("%s/%s bloqueado=%v %.200s", it.Papel, it.Origem, it.Bloqueado, strings.ReplaceAll(it.Conteudo, "\n", " "))
	}
	itens := conversaDe(t, x)
	fim := itens[len(itens)-1]
	t.Logf("resposta: %s\ncitações: %+v", fim.Conteudo, fim.Citacoes)
	if !pesquisou || !strings.Contains(fim.Conteudo, "1789") || len(fim.Citacoes) == 0 {
		t.Error("a pesquisa não produziu uma resposta com fonte")
	}
}
