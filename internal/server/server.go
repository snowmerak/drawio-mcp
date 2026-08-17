package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/snowmerak/drawio-mcp/internal/catalog"
	"github.com/snowmerak/drawio-mcp/internal/drawio"
)

type Service struct {
	catalog   *catalog.Catalog
	documents *drawio.Manager
}

func New(shapeCatalog *catalog.Catalog) *mcp.Server {
	service := &Service{catalog: shapeCatalog, documents: drawio.NewManager()}
	server := mcp.NewServer(&mcp.Implementation{Name: "drawio-mcp", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_shapes", Description: "List bundled draw.io shapes, optionally filtered by source, category, or tags."}, service.listShapes)
	mcp.AddTool(server, &mcp.Tool{Name: "find_shapes", Description: "Find bundled draw.io shapes by stable ID, name, description, or tags."}, service.findShapes)
	mcp.AddTool(server, &mcp.Tool{Name: "create_diagram", Description: "Create a new in-memory draw.io document with one page."}, service.createDiagram)
	mcp.AddTool(server, &mcp.Tool{Name: "open_diagram", Description: "Open an existing compressed or uncompressed .drawio file for editing."}, service.openDiagram)
	mcp.AddTool(server, &mcp.Tool{Name: "save_diagram", Description: "Save an open draw.io document. The path may be omitted when the document already has one."}, service.saveDiagram)
	mcp.AddTool(server, &mcp.Tool{Name: "place_shape", Description: "Place a bundled shape on a page at the requested coordinates."}, service.placeShape)
	mcp.AddTool(server, &mcp.Tool{Name: "inspect_diagram", Description: "Inspect the pages and placed vertices in an open draw.io document."}, service.inspectDiagram)
	mcp.AddTool(server, &mcp.Tool{Name: "inspect_region", Description: "Inspect semantic nodes and relevant edges in a rectangular page region."}, service.inspectRegion)
	mcp.AddTool(server, &mcp.Tool{Name: "route_edges", Description: "Automatically create orthogonal edges on directed grid lanes; same-direction edges may share lanes and opposite-direction edges may not."}, service.routeEdges)
	return server
}

type shapeOutput struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Category    string   `json:"category"`
	Source      string   `json:"source"`
	Width       float64  `json:"width"`
	Height      float64  `json:"height"`
}

type shapesOutput struct {
	Shapes []shapeOutput `json:"shapes"`
	Count  int           `json:"count"`
}

type listShapesInput struct {
	Source   string   `json:"source,omitempty" jsonschema:"exact source filter such as default, saturday, or cloudflare"`
	Category string   `json:"category,omitempty" jsonschema:"exact category filter"`
	Tags     []string `json:"tags,omitempty" jsonschema:"tags that every result must contain"`
	Limit    int      `json:"limit,omitempty" jsonschema:"maximum number of results, from 1 to 200"`
}

func (s *Service) listShapes(_ context.Context, _ *mcp.CallToolRequest, input listShapesInput) (*mcp.CallToolResult, shapesOutput, error) {
	return nil, makeShapesOutput(s.catalog.List(input.Source, input.Category, input.Tags, input.Limit)), nil
}

type findShapesInput struct {
	Query    string   `json:"query" jsonschema:"shape ID, name, description, or tag text to search"`
	Source   string   `json:"source,omitempty" jsonschema:"exact source filter such as default, saturday, or cloudflare"`
	Category string   `json:"category,omitempty" jsonschema:"exact category filter"`
	Tags     []string `json:"tags,omitempty" jsonschema:"tags that every result must contain"`
	Limit    int      `json:"limit,omitempty" jsonschema:"maximum number of results, from 1 to 200"`
}

func (s *Service) findShapes(_ context.Context, _ *mcp.CallToolRequest, input findShapesInput) (*mcp.CallToolResult, shapesOutput, error) {
	return nil, makeShapesOutput(s.catalog.Find(input.Query, input.Source, input.Category, input.Tags, input.Limit)), nil
}

func makeShapesOutput(shapes []catalog.Shape) shapesOutput {
	output := shapesOutput{Shapes: make([]shapeOutput, len(shapes)), Count: len(shapes)}
	for i, shape := range shapes {
		output.Shapes[i] = shapeOutput{ID: shape.ID, Name: shape.Name, Description: shape.Description, Tags: shape.Tags, Category: shape.Category, Source: shape.Source, Width: shape.Width, Height: shape.Height}
	}
	return output
}

type createDiagramInput struct {
	Name     string `json:"name,omitempty" jsonschema:"document display name; defaults to Untitled"`
	PageName string `json:"page_name,omitempty" jsonschema:"first page name; defaults to Page-1"`
	Path     string `json:"path,omitempty" jsonschema:"optional future save path; no file is written until save_diagram"`
}

func (s *Service) createDiagram(_ context.Context, _ *mcp.CallToolRequest, input createDiagramInput) (*mcp.CallToolResult, drawio.Summary, error) {
	return nil, s.documents.Create(input.Name, input.PageName, input.Path), nil
}

type openDiagramInput struct {
	Path string `json:"path" jsonschema:"path to an existing .drawio or XML file"`
}

func (s *Service) openDiagram(_ context.Context, _ *mcp.CallToolRequest, input openDiagramInput) (*mcp.CallToolResult, drawio.Summary, error) {
	if input.Path == "" {
		return nil, drawio.Summary{}, fmt.Errorf("path is required")
	}
	result, err := s.documents.Open(input.Path)
	return nil, result, err
}

type saveDiagramInput struct {
	DocumentID string `json:"document_id" jsonschema:"ID returned by create_diagram or open_diagram"`
	Path       string `json:"path,omitempty" jsonschema:"new save path; omit to reuse the current document path"`
}

func (s *Service) saveDiagram(_ context.Context, _ *mcp.CallToolRequest, input saveDiagramInput) (*mcp.CallToolResult, drawio.Summary, error) {
	result, err := s.documents.Save(input.DocumentID, input.Path)
	return nil, result, err
}

type placeShapeInput struct {
	DocumentID string  `json:"document_id" jsonschema:"ID returned by create_diagram or open_diagram"`
	Page       string  `json:"page,omitempty" jsonschema:"page ID or name; omit to use the first page"`
	ShapeID    string  `json:"shape_id" jsonschema:"stable shape ID returned by list_shapes or find_shapes"`
	X          float64 `json:"x" jsonschema:"left position in draw.io model units"`
	Y          float64 `json:"y" jsonschema:"top position in draw.io model units"`
	Width      float64 `json:"width,omitempty" jsonschema:"width; zero uses the shape default"`
	Height     float64 `json:"height,omitempty" jsonschema:"height; zero uses the shape default"`
	Label      string  `json:"label,omitempty" jsonschema:"optional shape label"`
}

type placeShapeOutput struct {
	DocumentID string             `json:"document_id"`
	Page       string             `json:"page"`
	Cell       drawio.CellSummary `json:"cell"`
}

func (s *Service) placeShape(_ context.Context, _ *mcp.CallToolRequest, input placeShapeInput) (*mcp.CallToolResult, placeShapeOutput, error) {
	shape, ok := s.catalog.Get(input.ShapeID)
	if !ok {
		return nil, placeShapeOutput{}, fmt.Errorf("shape %q not found; call find_shapes first", input.ShapeID)
	}
	cell, err := s.documents.Place(input.DocumentID, input.Page, shape, input.X, input.Y, input.Width, input.Height, input.Label)
	return nil, placeShapeOutput{DocumentID: input.DocumentID, Page: input.Page, Cell: cell}, err
}

type inspectDiagramInput struct {
	DocumentID string `json:"document_id" jsonschema:"ID returned by create_diagram or open_diagram"`
}

func (s *Service) inspectDiagram(_ context.Context, _ *mcp.CallToolRequest, input inspectDiagramInput) (*mcp.CallToolResult, drawio.Summary, error) {
	result, err := s.documents.Inspect(input.DocumentID)
	return nil, result, err
}

type inspectRegionInput struct {
	DocumentID           string  `json:"document_id" jsonschema:"ID returned by create_diagram or open_diagram"`
	Page                 string  `json:"page,omitempty" jsonschema:"page ID or name; omit to use the first page"`
	X                    float64 `json:"x" jsonschema:"left edge in draw.io model coordinates"`
	Y                    float64 `json:"y" jsonschema:"top edge in draw.io model coordinates"`
	Width                float64 `json:"width" jsonschema:"positive region width in draw.io model coordinates"`
	Height               float64 `json:"height" jsonschema:"positive region height in draw.io model coordinates"`
	Match                string  `json:"match,omitempty" jsonschema:"node match mode: intersects or contained; defaults to intersects"`
	EdgeMode             string  `json:"edge_mode,omitempty" jsonschema:"edge mode: none, connected, or intersects; defaults to connected"`
	IncludeExternalNodes bool    `json:"include_external_nodes,omitempty" jsonschema:"include summarized endpoints outside the region for returned edges"`
	IncludeStyle         bool    `json:"include_style,omitempty" jsonschema:"include parsed styles with embedded image payloads omitted"`
	IncludeMetadata      bool    `json:"include_metadata,omitempty" jsonschema:"include custom mxCell and object attributes"`
	Limit                int     `json:"limit,omitempty" jsonschema:"maximum in-region nodes; defaults to 200 and is capped at 2000"`
}

func (s *Service) inspectRegion(_ context.Context, _ *mcp.CallToolRequest, input inspectRegionInput) (*mcp.CallToolResult, drawio.RegionResult, error) {
	result, err := s.documents.InspectRegion(input.DocumentID, input.Page, drawio.Bounds{
		X: input.X, Y: input.Y, Width: input.Width, Height: input.Height,
	}, drawio.RegionOptions{
		Match: input.Match, EdgeMode: input.EdgeMode,
		IncludeExternalNodes: input.IncludeExternalNodes,
		IncludeStyle:         input.IncludeStyle, IncludeMetadata: input.IncludeMetadata,
		Limit: input.Limit,
	})
	return nil, result, err
}

type routeConnectionInput struct {
	SourceID string `json:"source_id" jsonschema:"source vertex ID returned by inspect_diagram or inspect_region"`
	TargetID string `json:"target_id" jsonschema:"target vertex ID returned by inspect_diagram or inspect_region"`
	Label    string `json:"label,omitempty" jsonschema:"optional edge label"`
}

type routeEdgesInput struct {
	DocumentID     string                 `json:"document_id" jsonschema:"ID returned by create_diagram or open_diagram"`
	Page           string                 `json:"page,omitempty" jsonschema:"page ID or name; omit to use the first page"`
	Connections    []routeConnectionInput `json:"connections" jsonschema:"directed source-to-target connections routed atomically"`
	GridSize       float64                `json:"grid_size,omitempty" jsonschema:"lane grid size in model units; defaults to 20"`
	Clearance      float64                `json:"clearance,omitempty" jsonschema:"shape clearance in model units; defaults to grid_size"`
	BendPenalty    int                    `json:"bend_penalty,omitempty" jsonschema:"additional cost for each turn; defaults to 20"`
	NewLaneCost    int                    `json:"new_lane_cost,omitempty" jsonschema:"cost for occupying an empty lane; defaults to 10"`
	SharedLaneCost int                    `json:"shared_lane_cost,omitempty" jsonschema:"cost for a same-direction occupied lane; defaults to 3 and must not exceed new_lane_cost"`
	MaxSearchNodes int                    `json:"max_search_nodes,omitempty" jsonschema:"A-star expansion cap per port pair; defaults to 150000"`
}

func (s *Service) routeEdges(_ context.Context, _ *mcp.CallToolRequest, input routeEdgesInput) (*mcp.CallToolResult, drawio.RouteEdgesResult, error) {
	connections := make([]drawio.RouteConnection, len(input.Connections))
	for i, connection := range input.Connections {
		connections[i] = drawio.RouteConnection{SourceID: connection.SourceID, TargetID: connection.TargetID, Label: connection.Label}
	}
	result, err := s.documents.RouteEdges(input.DocumentID, input.Page, connections, drawio.RouteOptions{
		GridSize: input.GridSize, Clearance: input.Clearance,
		BendPenalty: input.BendPenalty, NewLaneCost: input.NewLaneCost,
		SharedLaneCost: input.SharedLaneCost, MaxSearchNodes: input.MaxSearchNodes,
	})
	return nil, result, err
}
