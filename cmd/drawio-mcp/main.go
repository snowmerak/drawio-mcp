package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/snowmerak/drawio-mcp/internal/catalog"
	"github.com/snowmerak/drawio-mcp/internal/server"
	templates "github.com/snowmerak/drawio-mcp/template"
)

func main() {
	shapeCatalog, err := catalog.Load(templates.FS)
	if err != nil {
		log.Fatalf("load embedded shapes: %v", err)
	}
	if err := server.New(shapeCatalog).Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
