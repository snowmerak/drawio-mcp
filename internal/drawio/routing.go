package drawio

import (
	"fmt"
	"strings"

	"github.com/snowmerak/drawio-mcp/internal/router"
)

// RouteConnection requests one directed connection between two vertices.
type RouteConnection struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	Label    string `json:"label,omitempty"`
}

// RouteOptions configures the orthogonal lane router.
type RouteOptions struct {
	GridSize       float64
	Clearance      float64
	BendPenalty    int
	NewLaneCost    int
	SharedLaneCost int
	MaxSearchNodes int
}

// RoutedEdge describes a created draw.io edge and its occupied lanes.
type RoutedEdge struct {
	ID         string   `json:"id"`
	SourceID   string   `json:"source_id"`
	TargetID   string   `json:"target_id"`
	Label      string   `json:"label,omitempty"`
	SourceSide string   `json:"source_side"`
	TargetSide string   `json:"target_side"`
	Points     []Point  `json:"points"`
	Lanes      []string `json:"lanes"`
	SharedWith []string `json:"shared_with,omitempty"`
	Cost       int      `json:"cost"`
}

// RouteEdgesResult contains all edges created by one atomic routing call.
type RouteEdgesResult struct {
	DocumentID  string       `json:"document_id"`
	Page        PageRef      `json:"page"`
	Edges       []RoutedEdge `json:"edges"`
	SharedLanes int          `json:"shared_lanes"`
}

type pendingConnection struct {
	id         string
	connection RouteConnection
}

// RouteEdges computes and adds every requested connection atomically.
func (d *Document) RouteEdges(pageRef string, connections []RouteConnection, options RouteOptions) (RouteEdgesResult, error) {
	if len(connections) == 0 {
		return RouteEdgesResult{}, fmt.Errorf("at least one connection is required")
	}
	page, err := d.page(pageRef)
	if err != nil {
		return RouteEdgesResult{}, err
	}
	semantic, err := buildSemanticPage(page)
	if err != nil {
		return RouteEdgesResult{}, err
	}
	effectiveGrid := options.GridSize
	if effectiveGrid == 0 {
		effectiveGrid = 20
	}

	children := make(map[string]bool)
	for _, cell := range semantic.cells {
		if cell.vertex && cell.parentID != "" {
			if parent := semantic.byID[cell.parentID]; parent != nil && parent.vertex {
				children[parent.id] = true
			}
		}
	}
	obstacles := make([]router.Rect, 0)
	for _, cell := range semantic.cells {
		if !cell.vertex || cell.id == "" || children[cell.id] {
			continue
		}
		bounds := semantic.bounds[cell.id]
		if bounds.Width <= 0 || bounds.Height <= 0 {
			continue
		}
		obstacles = append(obstacles, routerRect(bounds))
	}

	pending := make([]pendingConnection, len(connections))
	requests := make([]router.Request, len(connections))
	for i, connection := range connections {
		if connection.SourceID == "" || connection.TargetID == "" {
			return RouteEdgesResult{}, fmt.Errorf("connection %d requires source_id and target_id", i)
		}
		source := semantic.byID[connection.SourceID]
		if source == nil || !source.vertex {
			return RouteEdgesResult{}, fmt.Errorf("source shape %q not found", connection.SourceID)
		}
		target := semantic.byID[connection.TargetID]
		if target == nil || !target.vertex {
			return RouteEdgesResult{}, fmt.Errorf("target shape %q not found", connection.TargetID)
		}
		sourceBounds, targetBounds := semantic.bounds[source.id], semantic.bounds[target.id]
		if sourceBounds.Width <= 0 || sourceBounds.Height <= 0 || targetBounds.Width <= 0 || targetBounds.Height <= 0 {
			return RouteEdgesResult{}, fmt.Errorf("connection %d has a shape without positive geometry", i)
		}
		edgeID := newID("edge")
		pending[i] = pendingConnection{id: edgeID, connection: connection}
		requests[i] = router.Request{
			ID: edgeID, SourceID: connection.SourceID, TargetID: connection.TargetID,
			SourceRect: routerRect(sourceBounds), TargetRect: routerRect(targetBounds),
		}
	}

	reserved := make([]router.ReservedRoute, 0)
	for _, cell := range semantic.cells {
		if !cell.edge || cell.id == "" || cell.node.attr("drawioMcpRouted") != "1" {
			continue
		}
		gridValue := parseNumber(cell.node.attr("drawioMcpGrid"))
		if gridValue != effectiveGrid {
			continue
		}
		lanes := splitNonEmpty(cell.node.attr("drawioMcpLanes"), "|")
		if len(lanes) > 0 {
			reserved = append(reserved, router.ReservedRoute{ID: cell.id, LaneIDs: lanes})
		}
	}

	routed, err := router.Solve(router.Problem{
		Obstacles: obstacles,
		Requests:  requests,
		Reserved:  reserved,
		Options: router.Options{
			GridSize: options.GridSize, Clearance: options.Clearance,
			BendPenalty: options.BendPenalty, NewLaneCost: options.NewLaneCost,
			SharedLaneCost: options.SharedLaneCost, MaxSearchNodes: options.MaxSearchNodes,
		},
	})
	if err != nil {
		return RouteEdgesResult{}, err
	}

	root := page.Graph.child("root")
	if root == nil {
		return RouteEdgesResult{}, fmt.Errorf("page %q has no mxGraphModel root", page.Name)
	}
	labels := make(map[string]string, len(pending))
	for _, item := range pending {
		labels[item.id] = item.connection.Label
	}
	result := RouteEdgesResult{
		DocumentID: d.ID, Page: PageRef{ID: page.ID, Name: page.Name},
		Edges: make([]RoutedEdge, 0, len(routed.Routes)), SharedLanes: routed.SharedLanes,
	}
	for _, route := range routed.Routes {
		style := routedEdgeStyle(route.SourceSide, route.TargetSide)
		cell := element("mxCell",
			"id", route.ID, "value", labels[route.ID], "style", style,
			"edge", "1", "parent", "1", "source", route.SourceID, "target", route.TargetID,
			"drawioMcpRouted", "1", "drawioMcpGrid", formatFloat(effectiveGrid),
			"drawioMcpLanes", strings.Join(route.LaneIDs, "|"),
		)
		geometry := element("mxGeometry", "relative", "1", "as", "geometry")
		points := element("Array", "as", "points")
		for _, point := range route.Points {
			points.Children = append(points.Children, element("mxPoint", "x", formatFloat(point.X), "y", formatFloat(point.Y)))
		}
		geometry.Children = append(geometry.Children, points)
		cell.Children = append(cell.Children, geometry)
		root.Children = append(root.Children, cell)

		outputPoints := make([]Point, len(route.Points))
		for i, point := range route.Points {
			outputPoints[i] = Point{X: point.X, Y: point.Y}
		}
		result.Edges = append(result.Edges, RoutedEdge{
			ID: route.ID, SourceID: route.SourceID, TargetID: route.TargetID, Label: labels[route.ID],
			SourceSide: string(route.SourceSide), TargetSide: string(route.TargetSide),
			Points: outputPoints, Lanes: route.LaneIDs, SharedWith: route.SharedWith, Cost: route.Cost,
		})
	}
	d.Modified = true
	return result, nil
}

func routerRect(bounds Bounds) router.Rect {
	return router.Rect{X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height}
}

func routedEdgeStyle(source, target router.Side) string {
	exitX, exitY := sideCoordinates(source)
	entryX, entryY := sideCoordinates(target)
	return "edgeStyle=orthogonalEdgeStyle;rounded=0;orthogonalLoop=1;jettySize=auto;html=1;" +
		"exitX=" + exitX + ";exitY=" + exitY + ";exitDx=0;exitDy=0;" +
		"entryX=" + entryX + ";entryY=" + entryY + ";entryDx=0;entryDy=0;"
}

func sideCoordinates(side router.Side) (string, string) {
	switch side {
	case router.North:
		return "0.5", "0"
	case router.East:
		return "1", "0.5"
	case router.South:
		return "0.5", "1"
	case router.West:
		return "0", "0.5"
	default:
		return "0.5", "0.5"
	}
}

func splitNonEmpty(value, separator string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, separator)
	result := parts[:0]
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
