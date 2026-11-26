// Package mcpextrair é o servidor MCP de extração de documentos.
package mcpextrair

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wkoch/Palantir/internal/extrator"
)

// Entrada são os argumentos da ferramenta "extrair".
type Entrada struct {
	Nome string `json:"nome" jsonschema:"nome original do arquivo"`
}

// Novo cria o servidor para o arquivo montado em caminho.
func Novo(caminho string, e extrator.Extrator) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "palantir-extrair", Version: "1"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "extrair", Description: "Extrai o texto do documento montado."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in Entrada) (*mcp.CallToolResult, extrator.Resultado, error) {
			if caminho == "" {
				return nil, extrator.Resultado{}, errors.New("nenhum documento foi montado")
			}
			r, err := e.Extrair(ctx, caminho, in.Nome)
			if r.Paginas == nil {
				r.Paginas = []extrator.Pagina{}
			}
			return nil, r, err
		})
	return s
}
