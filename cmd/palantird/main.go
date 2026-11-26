// Comando palantird é o daemon do Palantír.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/wkoch/Palantir/internal/cli"
)

// versao é preenchida na compilação com -ldflags.
var versao = "desenvolvimento"

func main() {
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()
	os.Exit(cli.Executar(ctx, os.Args[1:], cli.Ambiente{Versao: versao,
		Entrada: os.Stdin, Saida: os.Stdout, Erros: os.Stderr}))
}
