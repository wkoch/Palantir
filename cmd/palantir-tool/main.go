// Comando palantir-tool executa um servidor MCP do Palantír pela entrada e
// saída padrão. Em produção ele roda num contêiner sem rede.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wkoch/Palantir/internal/servidores"
)

func main() {
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()
	os.Exit(executar(ctx, os.Args[1:], os.Getenv, &mcp.StdioTransport{}, os.Stderr))
}

func executar(ctx context.Context, args []string, obter func(string) string, t mcp.Transport, erros io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(erros, "uso: palantir-tool web|imap|extrair")
		return 2
	}
	s, err := servidores.Novo(args[0], obter)
	if err != nil {
		fmt.Fprintln(erros, "palantir-tool:", err)
		return 2
	}
	if err := s.Run(ctx, t); err != nil && ctx.Err() == nil {
		fmt.Fprintln(erros, "palantir-tool:", err)
		return 1
	}
	return 0
}
