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

type regionResult struct {
	Nodes []struct {
		ID string `json:"id"`
	} `json:"nodes"`
	Edges []struct {
		ID string `json:"id"`
	} `json:"edges"`
}

type placeResult struct {
	Cell struct {
		ID string `json:"id"`
	} `json:"cell"`
}

type routeResult struct {
	SharedLanes int `json:"shared_lanes"`
	Edges       []struct {
		ID string `json:"id"`
	} `json:"edges"`
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

	cellIDs := make([]string, len(placements))
	for i, item := range placements {
		shapeID := findExactShape(ctx, session, item.Query)
		var placed placeResult
		call(ctx, session, "place_shape", map[string]any{
			"document_id": document.DocumentID,
			"page":        document.Pages[0].ID,
			"shape_id":    shapeID,
			"x":           item.X,
			"y":           item.Y,
			"width":       item.Width,
			"height":      item.Height,
			"label":       item.Label,
		}, &placed)
		cellIDs[i] = placed.Cell.ID
	}

	connections := []map[string]any{
		{"source_id": cellIDs[1], "target_id": cellIDs[2]},
		{"source_id": cellIDs[2], "target_id": cellIDs[3]},
		{"source_id": cellIDs[3], "target_id": cellIDs[4]},
		{"source_id": cellIDs[4], "target_id": cellIDs[5]},
		{"source_id": cellIDs[1], "target_id": cellIDs[4], "label": "request"},
		{"source_id": cellIDs[2], "target_id": cellIDs[5], "label": "forward"},
	}
	var routed routeResult
	call(ctx, session, "route_edges", map[string]any{
		"document_id": document.DocumentID,
		"page":        document.Pages[0].ID,
		"connections": connections,
		"grid_size":   20,
		"clearance":   20,
	}, &routed)
	if len(routed.Edges) != len(connections) {
		log.Fatalf("route_edges returned an incomplete batch: %+v", routed)
	}

	call(ctx, session, "save_diagram", map[string]any{
		"document_id": document.DocumentID,
		"path":        absoluteOutput,
	}, nil)

	var reopened documentResult
	call(ctx, session, "open_diagram", map[string]any{"path": absoluteOutput}, &reopened)
	var inspected regionResult
	call(ctx, session, "inspect_region", map[string]any{
		"document_id": reopened.DocumentID,
		"x":           0, "y": 0, "width": 1000, "height": 400,
		"match": "contained", "edge_mode": "connected",
	}, &inspected)
	if len(inspected.Nodes) != len(placements) || len(inspected.Edges) != len(connections) {
		log.Fatalf("inspect_region returned an unexpected document: %+v", inspected)
	}
	fmt.Printf("%s (%d shapes, %d routed edges, %d shared lanes)\n", absoluteOutput, len(inspected.Nodes), len(inspected.Edges), routed.SharedLanes)
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
