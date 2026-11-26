// Package servidores monta os servidores MCP a partir do ambiente entregue
// pelo daemon. É usado pelo binário palantir-tool e pelos testes.
package servidores

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wkoch/Palantir/internal/extrator"
	"github.com/wkoch/Palantir/internal/ferramentas"
	"github.com/wkoch/Palantir/internal/mcpextrair"
	"github.com/wkoch/Palantir/internal/mcpimap"
	"github.com/wkoch/Palantir/internal/mcpweb"
)

// Novo cria o servidor pedido; obter lê as variáveis de ambiente.
func Novo(nome string, obter func(string) string) (*mcp.Server, error) {
	socket := obter(ferramentas.EnvProxy)
	switch nome {
	case ferramentas.Web:
		return mcpweb.Novo(mcpweb.Opcoes{Socket: socket, Extrator: extrator.Padrao(),
			Busca: obter(ferramentas.EnvBusca)}), nil
	case ferramentas.IMAP:
		o, err := mcpimap.DoAmbiente(obter, socket)
		if err != nil {
			return nil, err
		}
		return mcpimap.Novo(o), nil
	case ferramentas.Extrair:
		return mcpextrair.Novo(obter(ferramentas.EnvEntrada), extrator.Padrao()), nil
	}
	return nil, fmt.Errorf("ferramenta desconhecida: %q (use web, imap ou extrair)", nome)
}
