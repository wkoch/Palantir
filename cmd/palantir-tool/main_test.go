package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wkoch/Palantir/internal/ferramentas"
)

func TestExecutar(t *testing.T) {
	vazio := func(string) string { return "" }
	var erros bytes.Buffer
	ctx := context.Background()
	if c := executar(ctx, nil, vazio, nil, &erros); c != 2 || !strings.Contains(erros.String(), "uso:") {
		t.Error(c, erros.String())
	}
	if c := executar(ctx, []string{"voar"}, vazio, nil, &erros); c != 2 {
		t.Error(c)
	}
	if c := executar(ctx, []string{"imap"}, vazio, nil, &erros); c != 2 {
		t.Error("e-mail sem configuração", c)
	}
	for _, nome := range []string{"web", "extrair"} {
		ctx, cancelar := context.WithCancel(context.Background())
		doServidor, doCliente := mcp.NewInMemoryTransports()
		fim := make(chan int)
		go func() { fim <- executar(ctx, []string{nome}, vazio, doServidor, &erros) }()
		s, err := ferramentas.Conectar(ctx, doCliente)
		if err != nil {
			t.Fatal(err)
		}
		s.Close()
		cancelar()
		select {
		case c := <-fim:
			if c != 0 {
				t.Error(nome, c, erros.String())
			}
		case <-time.After(5 * time.Second):
			t.Fatal("não encerrou")
		}
	}
	// Um transporte que falha é informado.
	if c := executar(ctx, []string{"web"}, vazio, quebrado{}, &erros); c != 1 {
		t.Error(c)
	}
}

type quebrado struct{}

func (quebrado) Connect(context.Context) (mcp.Connection, error) {
	return nil, errors.New("sem conexão")
}
