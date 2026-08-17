// Command drawio-example generates an example file through the drawio MCP tools.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type shapeResult struct {
	Shapes []struct {
		ID string `json:"id"`
	} `json:"shapes"`
}

type documentResult struct {
	DocumentID string `json:"document_id"`
	Pages      []struct {
		ID    string `json:"id"`
		Cells []struct {
			ID string `json:"id"`
		} `json:"cells"`
	} `json:"pages"`
}

type placement struct {
	Query  string
	Label  string
	X      float64
	Y      float64
	Width  float64
	Height float64
}

func main() {
	serverPath := flag.String("server", filepath.FromSlash("build/drawio-mcp.exe"), "path to the drawio MCP server binary")
	outputPath := flag.String("output", filepath.FromSlash("examples/generated-architecture.drawio"), "output .drawio path")
	flag.Parse()

	absoluteServer, err := filepath.Abs(*serverPath)
	if err != nil {
		log.Fatal(err)
	}
	absoluteOutput, err := filepath.Abs(*outputPath)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "drawio-example", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(absoluteServer)}, nil)
	if err != nil {
		log.Fatalf("connect to drawio MCP server: %v", err)
	}
	defer session.Close()

	var document documentResult
	call(ctx, session, "create_diagram", map[string]any{
		"name":      "Generated Architecture",
		"page_name": "Architecture",
		"path":      absoluteOutput,
	}, &document)
	if document.DocumentID == "" || len(document.Pages) == 0 {
		log.Fatalf("create_diagram returned an incomplete document: %+v", document)
	}

	placements := []placement{
		{Query: "default.note", Label: "Generated entirely through drawio-mcp", X: 290, Y: 30, Width: 430, Height: 70},
		{Query: "default.cloud", Label: "Users / Internet", X: 40, Y: 180, Width: 140, Height: 90},
		{Query: "cloudflare.cloud-internet-multi", Label: "Cloudflare Edge", X: 250, Y: 180, Width: 96, Height: 96},
		{Query: "cloudflare.workers-unbound", Label: "Worker", X: 450, Y: 180, Width: 96, Height: 96},
		{Query: "saturday.golang", Label: "Go API", X: 650, Y: 180, Width: 96, Height: 96},
		{Query: "default.cylinder", Label: "Database", X: 850, Y: 165, Width: 100, Height: 120},
	}

	for _, item := range placements {
		shapeID := findExactShape(ctx, session, item.Query)
		call(ctx, session, "place_shape", map[string]any{
			"document_id": document.DocumentID,
			"page":        document.Pages[0].ID,
			"shape_id":    shapeID,
			"x":           item.X,
			"y":           item.Y,
			"width":       item.Width,
			"height":      item.Height,
			"label":       item.Label,
		}, nil)
	}

	call(ctx, session, "save_diagram", map[string]any{
		"document_id": document.DocumentID,
		"path":        absoluteOutput,
	}, nil)

	var reopened documentResult
	call(ctx, session, "open_diagram", map[string]any{"path": absoluteOutput}, &reopened)
	var inspected documentResult
	call(ctx, session, "inspect_diagram", map[string]any{"document_id": reopened.DocumentID}, &inspected)
	if len(inspected.Pages) != 1 || len(inspected.Pages[0].Cells) != len(placements) {
		log.Fatalf("inspect_diagram returned an unexpected document: %+v", inspected)
	}
	fmt.Printf("%s (%d shapes)\n", absoluteOutput, len(inspected.Pages[0].Cells))
}

func findExactShape(ctx context.Context, session *mcp.ClientSession, id string) string {
	var result shapeResult
	call(ctx, session, "find_shapes", map[string]any{"query": id, "limit": 10}, &result)
	for _, shape := range result.Shapes {
		if shape.ID == id {
			return shape.ID
		}
	}
	log.Fatalf("find_shapes did not return %q", id)
	return ""
}

func call(ctx context.Context, session *mcp.ClientSession, name string, arguments any, output any) {
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		log.Fatalf("call %s: %v", name, err)
	}
	if result.IsError {
		log.Fatalf("call %s: %v", name, result.GetError())
	}
	if output == nil {
		return
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		log.Fatalf("marshal %s result: %v", name, err)
	}
	if err := json.Unmarshal(data, output); err != nil {
		log.Fatalf("decode %s result: %v", name, err)
	}
}
