// Comando palantir-simulador mantém um modelo, um servidor IMAP e uma página
// web falsos, para demonstrar e testar o Palantír sem custo.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/wkoch/Palantir/internal/simulador"
)

func main() {
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()
	if err := simulador.Executar(ctx, os.Args[1:], os.Stdout, nil); err != nil {
		fmt.Fprintln(os.Stderr, "palantir-simulador:", err)
		os.Exit(1)
	}
}
