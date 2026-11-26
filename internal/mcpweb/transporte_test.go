package mcpweb_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func transporte(t *testing.T, s *mcp.Server) mcp.Transport {
	t.Helper()
	doServidor, doCliente := mcp.NewInMemoryTransports()
	if _, err := s.Connect(context.Background(), doServidor, nil); err != nil {
		t.Fatal(err)
	}
	return doCliente
}
