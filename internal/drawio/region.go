package drawio

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	RegionIntersects = "intersects"
	RegionContained  = "contained"

	EdgesNone       = "none"
	EdgesConnected  = "connected"
	EdgesIntersects = "intersects"
)

// Bounds is a rectangle in draw.io model coordinates.
type Bounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Point is a point in draw.io model coordinates.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// PageRef identifies the page returned by a regional query.
type PageRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RegionOptions controls semantic data returned for a rectangular area.
type RegionOptions struct {
	Match                string
	EdgeMode             string
	IncludeExternalNodes bool
	IncludeStyle         bool
	IncludeMetadata      bool
	Limit                int
}

// RegionNode is a vertex found in or connected to a queried region.
type RegionNode struct {
	ID       string            `json:"id"`
	ParentID string            `json:"parent_id,omitempty"`
	ShapeID  string            `json:"shape_id,omitempty"`
	Label    string            `json:"label,omitempty"`
	InRegion bool              `json:"in_region"`
	Bounds   Bounds            `json:"bounds"`
	Style    map[string]string `json:"style,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RegionEdge is an edge relevant to a queried region.
type RegionEdge struct {
	ID       string            `json:"id"`
	SourceID string            `json:"source_id,omitempty"`
	TargetID string            `json:"target_id,omitempty"`
	Label    string            `json:"label,omitempty"`
	Points   []Point           `json:"points,omitempty"`
	Style    map[string]string `json:"style,omitempty"`
}

// RegionResult contains semantic diagram data for a rectangular area.
type RegionResult struct {
	DocumentID string       `json:"document_id"`
	Page       PageRef      `json:"page"`
	Region     Bounds       `json:"region"`
	Nodes      []RegionNode `json:"nodes"`
	Edges      []RegionEdge `json:"edges"`
	Truncated  bool         `json:"truncated"`
}

type semanticCell struct {
	node     *Node
	wrapper  *Node
	id       string
	parentID string
	shapeID  string
	label    string
	style    string
	vertex   bool
	edge     bool
	sourceID string
	targetID string
	geometry *Node
}

type semanticPage struct {
	cells  []*semanticCell
	byID   map[string]*semanticCell
	bounds map[string]Bounds
	state  map[string]uint8
}

// InspectRegion returns nodes and edges relevant to a page rectangle.
func (d *Document) InspectRegion(pageRef string, region Bounds, options RegionOptions) (RegionResult, error) {
	if !finite(region.X) || !finite(region.Y) || !finite(region.Width) || !finite(region.Height) {
		return RegionResult{}, fmt.Errorf("region coordinates and dimensions must be finite numbers")
	}
	if region.Width <= 0 || region.Height <= 0 {
		return RegionResult{}, fmt.Errorf("region width and height must be greater than zero")
	}
	if options.Match == "" {
		options.Match = RegionIntersects
	}
	if options.Match != RegionIntersects && options.Match != RegionContained {
		return RegionResult{}, fmt.Errorf("match must be %q or %q", RegionIntersects, RegionContained)
	}
	if options.EdgeMode == "" {
		options.EdgeMode = EdgesConnected
	}
	if options.EdgeMode != EdgesNone && options.EdgeMode != EdgesConnected && options.EdgeMode != EdgesIntersects {
		return RegionResult{}, fmt.Errorf("edge_mode must be %q, %q, or %q", EdgesNone, EdgesConnected, EdgesIntersects)
	}
	if options.Limit <= 0 {
		options.Limit = 200
	} else if options.Limit > 2000 {
		options.Limit = 2000
	}

	page, err := d.page(pageRef)
	if err != nil {
		return RegionResult{}, err
	}
	semantic, err := buildSemanticPage(page)
	if err != nil {
		return RegionResult{}, err
	}
	result := RegionResult{
		DocumentID: d.ID,
		Page:       PageRef{ID: page.ID, Name: page.Name},
		Region:     region,
		Nodes:      []RegionNode{},
		Edges:      []RegionEdge{},
	}

	selected := make(map[string]bool)
	matched := 0
	for _, cell := range semantic.cells {
		if !cell.vertex || cell.id == "" {
			continue
		}
		bounds := semantic.bounds[cell.id]
		inRegion := intersects(bounds, region)
		if options.Match == RegionContained {
			inRegion = containedBy(bounds, region)
		}
		if !inRegion {
			continue
		}
		matched++
		if len(result.Nodes) >= options.Limit {
			continue
		}
		selected[cell.id] = true
		result.Nodes = append(result.Nodes, makeRegionNode(cell, bounds, true, options))
	}
	result.Truncated = matched > len(result.Nodes)

	if options.EdgeMode == EdgesNone {
		return result, nil
	}
	includedEdges := make([]*semanticCell, 0)
	for _, cell := range semantic.cells {
		if !cell.edge || cell.id == "" {
			continue
		}
		connected := selected[cell.sourceID] || selected[cell.targetID]
		points := semantic.edgePoints(cell)
		include := connected
		if options.EdgeMode == EdgesIntersects && !include {
			include = polylineIntersects(points, region)
		}
		if !include {
			continue
		}
		includedEdges = append(includedEdges, cell)
		result.Edges = append(result.Edges, RegionEdge{
			ID:       cell.id,
			SourceID: cell.sourceID,
			TargetID: cell.targetID,
			Label:    cell.label,
			Points:   points,
			Style:    styleForOutput(cell.style, options.IncludeStyle),
		})
	}

	if options.IncludeExternalNodes {
		for _, edge := range includedEdges {
			for _, id := range []string{edge.sourceID, edge.targetID} {
				if id == "" || selected[id] {
					continue
				}
				cell := semantic.byID[id]
				if cell == nil || !cell.vertex {
					continue
				}
				selected[id] = true
				result.Nodes = append(result.Nodes, makeRegionNode(cell, semantic.bounds[id], false, options))
			}
		}
	}
	return result, nil
}

func buildSemanticPage(page *Page) (*semanticPage, error) {
	root := page.Graph.child("root")
	if root == nil {
		return nil, fmt.Errorf("page %q has no mxGraphModel root", page.Name)
	}
	semantic := &semanticPage{
		byID:   make(map[string]*semanticCell),
		bounds: make(map[string]Bounds),
		state:  make(map[string]uint8),
	}
	collectSemanticCells(root, nil, semantic)
	for _, cell := range semantic.cells {
		if cell.id != "" && cell.vertex {
			semantic.resolveBounds(cell.id)
		}
	}
	return semantic, nil
}

func collectSemanticCells(node, wrapper *Node, semantic *semanticPage) {
	for _, child := range node.Children {
		switch child.Name.Local {
		case "mxCell":
			cell := newSemanticCell(child, wrapper)
			semantic.cells = append(semantic.cells, cell)
			if cell.id != "" {
				semantic.byID[cell.id] = cell
			}
		case "object", "UserObject":
			collectSemanticCells(child, child, semantic)
		default:
			collectSemanticCells(child, wrapper, semantic)
		}
	}
}

func newSemanticCell(node, wrapper *Node) *semanticCell {
	id := node.attr("id")
	label := node.attr("value")
	shapeID := node.attr("drawioMcpShape")
	if wrapper != nil {
		if id == "" {
			id = wrapper.attr("id")
		}
		if label == "" {
			label = wrapper.attr("label")
		}
		if shapeID == "" {
			shapeID = wrapper.attr("drawioMcpShape")
		}
	}
	return &semanticCell{
		node: node, wrapper: wrapper, id: id, parentID: node.attr("parent"),
		shapeID: shapeID, label: label, style: node.attr("style"),
		vertex: node.attr("vertex") == "1", edge: node.attr("edge") == "1",
		sourceID: node.attr("source"), targetID: node.attr("target"), geometry: node.child("mxGeometry"),
	}
}

func (s *semanticPage) resolveBounds(id string) Bounds {
	if s.state[id] == 2 {
		return s.bounds[id]
	}
	if s.state[id] == 1 {
		return Bounds{}
	}
	s.state[id] = 1
	cell := s.byID[id]
	if cell == nil || cell.geometry == nil {
		s.state[id] = 2
		return Bounds{}
	}
	geometry := cell.geometry
	bounds := Bounds{
		X: parseNumber(geometry.attr("x")), Y: parseNumber(geometry.attr("y")),
		Width: parseNumber(geometry.attr("width")), Height: parseNumber(geometry.attr("height")),
	}
	if parent := s.byID[cell.parentID]; parent != nil && parent.vertex {
		parentBounds := s.resolveBounds(parent.id)
		if geometry.attr("relative") == "1" {
			bounds.X = parentBounds.X + bounds.X*parentBounds.Width
			bounds.Y = parentBounds.Y + bounds.Y*parentBounds.Height
		} else {
			bounds.X += parentBounds.X
			bounds.Y += parentBounds.Y
		}
		if offset := geometryPoint(geometry, "offset"); offset != nil {
			bounds.X += offset.X
			bounds.Y += offset.Y
		}
	}
	s.bounds[id] = bounds
	s.state[id] = 2
	return bounds
}

func (s *semanticPage) edgePoints(edge *semanticCell) []Point {
	points := make([]Point, 0)
	if source := geometryPoint(edge.geometry, "sourcePoint"); source != nil {
		points = append(points, *source)
	} else if source := s.byID[edge.sourceID]; source != nil && source.vertex {
		points = append(points, center(s.bounds[source.id]))
	}
	if edge.geometry != nil {
		if array := edge.geometry.child("Array"); array != nil && array.attr("as") == "points" {
			for _, node := range array.Children {
				if node.Name.Local == "mxPoint" {
					points = append(points, Point{X: parseNumber(node.attr("x")), Y: parseNumber(node.attr("y"))})
				}
			}
		}
	}
	if target := geometryPoint(edge.geometry, "targetPoint"); target != nil {
		points = append(points, *target)
	} else if target := s.byID[edge.targetID]; target != nil && target.vertex {
		points = append(points, center(s.bounds[target.id]))
	}
	return points
}

func makeRegionNode(cell *semanticCell, bounds Bounds, inRegion bool, options RegionOptions) RegionNode {
	return RegionNode{
		ID: cell.id, ParentID: cell.parentID, ShapeID: cell.shapeID, Label: cell.label,
		InRegion: inRegion, Bounds: bounds,
		Style:    styleForOutput(cell.style, options.IncludeStyle),
		Metadata: metadataForOutput(cell, options.IncludeMetadata),
	}
}

func styleForOutput(style string, include bool) map[string]string {
	if !include || style == "" {
		return nil
	}
	result := make(map[string]string)
	for _, part := range strings.Split(style, ";") {
		if part == "" {
			continue
		}
		key, value, found := strings.Cut(part, "=")
		if !found {
			result[key] = ""
			continue
		}
		if key == "image" && strings.HasPrefix(value, "data:") {
			mime := strings.TrimPrefix(strings.SplitN(value, ",", 2)[0], "data:")
			result[key] = "embedded:" + mime
			continue
		}
		result[key] = value
	}
	return result
}

func metadataForOutput(cell *semanticCell, include bool) map[string]string {
	if !include {
		return nil
	}
	result := make(map[string]string)
	standard := map[string]bool{
		"id": true, "value": true, "label": true, "style": true, "parent": true,
		"vertex": true, "edge": true, "source": true, "target": true, "drawioMcpShape": true,
	}
	for _, source := range []*Node{cell.wrapper, cell.node} {
		if source == nil {
			continue
		}
		for _, attr := range source.Attrs {
			if !standard[attr.Name.Local] {
				result[attr.Name.Local] = attr.Value
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func geometryPoint(geometry *Node, kind string) *Point {
	if geometry == nil {
		return nil
	}
	for _, node := range geometry.Children {
		if node.Name.Local == "mxPoint" && node.attr("as") == kind {
			return &Point{X: parseNumber(node.attr("x")), Y: parseNumber(node.attr("y"))}
		}
	}
	return nil
}

func parseNumber(value string) float64 {
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed
}

func center(bounds Bounds) Point {
	return Point{X: bounds.X + bounds.Width/2, Y: bounds.Y + bounds.Height/2}
}

func intersects(a, b Bounds) bool {
	return a.X <= b.X+b.Width && a.X+a.Width >= b.X && a.Y <= b.Y+b.Height && a.Y+a.Height >= b.Y
}

func containedBy(inner, outer Bounds) bool {
	return inner.X >= outer.X && inner.Y >= outer.Y && inner.X+inner.Width <= outer.X+outer.Width && inner.Y+inner.Height <= outer.Y+outer.Height
}

func polylineIntersects(points []Point, region Bounds) bool {
	for _, point := range points {
		if pointInBounds(point, region) {
			return true
		}
	}
	for i := 1; i < len(points); i++ {
		if segmentIntersects(points[i-1], points[i], region) {
			return true
		}
	}
	return false
}

func pointInBounds(point Point, bounds Bounds) bool {
	return point.X >= bounds.X && point.X <= bounds.X+bounds.Width && point.Y >= bounds.Y && point.Y <= bounds.Y+bounds.Height
}

func segmentIntersects(start, end Point, bounds Bounds) bool {
	dx, dy := end.X-start.X, end.Y-start.Y
	p := []float64{-dx, dx, -dy, dy}
	q := []float64{start.X - bounds.X, bounds.X + bounds.Width - start.X, start.Y - bounds.Y, bounds.Y + bounds.Height - start.Y}
	low, high := 0.0, 1.0
	for i := range p {
		if p[i] == 0 {
			if q[i] < 0 {
				return false
			}
			continue
		}
		ratio := q[i] / p[i]
		if p[i] < 0 {
			low = math.Max(low, ratio)
		} else {
			high = math.Min(high, ratio)
		}
		if low > high {
			return false
		}
	}
	return true
}
