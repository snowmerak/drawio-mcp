package server_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/snowmerak/drawio-mcp/internal/catalog"
	"github.com/snowmerak/drawio-mcp/internal/server"
	templates "github.com/snowmerak/drawio-mcp/template"
)

func TestMCPToolsAreCallable(t *testing.T) {
	shapeCatalog, err := catalog.Load(templates.FS)
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.New(shapeCatalog).Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "drawio-mcp-test", Version: "0.1.0"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	tools, err := clientSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 7 {
		t.Fatalf("tool count = %d, want 7", len(tools.Tools))
	}

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "find_shapes",
		Arguments: map[string]any{"query": "wrangler", "limit": 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("find_shapes returned an error: %v", result.GetError())
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured result has type %T", result.StructuredContent)
	}
	if count, ok := structured["count"].(float64); !ok || count < 1 {
		t.Fatalf("unexpected structured result: %#v", structured)
	}
}
