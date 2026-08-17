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
	if len(tools.Tools) != 9 {
		t.Fatalf("tool count = %d, want 9", len(tools.Tools))
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

	created, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "create_diagram",
		Arguments: map[string]any{"name": "Region Test", "page_name": "Page-1"},
	})
	if err != nil || created.IsError {
		t.Fatalf("create_diagram: result=%+v err=%v", created, err)
	}
	createdData := created.StructuredContent.(map[string]any)
	documentID := createdData["document_id"].(string)
	pages := createdData["pages"].([]any)
	pageID := pages[0].(map[string]any)["id"].(string)

	placed, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "place_shape",
		Arguments: map[string]any{
			"document_id": documentID, "page": pageID, "shape_id": "default.rectangle",
			"x": 100, "y": 80, "label": "Inside",
		},
	})
	if err != nil || placed.IsError {
		t.Fatalf("place_shape: result=%+v err=%v", placed, err)
	}
	placedData := placed.StructuredContent.(map[string]any)
	sourceCellID := placedData["cell"].(map[string]any)["id"].(string)

	region, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "inspect_region",
		Arguments: map[string]any{
			"document_id": documentID, "page": pageID,
			"x": 90, "y": 70, "width": 150, "height": 100,
			"match": "intersects", "edge_mode": "none", "include_style": true,
		},
	})
	if err != nil || region.IsError {
		t.Fatalf("inspect_region: result=%+v err=%v", region, err)
	}
	regionData := region.StructuredContent.(map[string]any)
	nodes := regionData["nodes"].([]any)
	if len(nodes) != 1 || nodes[0].(map[string]any)["label"] != "Inside" {
		t.Fatalf("unexpected inspect_region result: %#v", regionData)
	}

	second, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "place_shape",
		Arguments: map[string]any{
			"document_id": documentID, "page": pageID, "shape_id": "default.rectangle",
			"x": 400, "y": 80, "label": "Target",
		},
	})
	if err != nil || second.IsError {
		t.Fatalf("place second shape: result=%+v err=%v", second, err)
	}
	secondData := second.StructuredContent.(map[string]any)
	targetCellID := secondData["cell"].(map[string]any)["id"].(string)

	routed, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "route_edges",
		Arguments: map[string]any{
			"document_id": documentID, "page": pageID, "grid_size": 20,
			"connections": []map[string]any{
				{"source_id": sourceCellID, "target_id": targetCellID, "label": "one"},
				{"source_id": sourceCellID, "target_id": targetCellID, "label": "two"},
			},
		},
	})
	if err != nil || routed.IsError {
		t.Fatalf("route_edges: result=%+v err=%v", routed, err)
	}
	routedData := routed.StructuredContent.(map[string]any)
	edges := routedData["edges"].([]any)
	sharedLanes := routedData["shared_lanes"].(float64)
	if len(edges) != 2 || sharedLanes < 1 {
		t.Fatalf("unexpected route_edges result: %#v", routedData)
	}
}
