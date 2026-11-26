package mcpimap_test

import "github.com/modelcontextprotocol/go-sdk/mcp"

func transportes() (*mcp.InMemoryTransport, *mcp.InMemoryTransport) {
	return mcp.NewInMemoryTransports()
}
