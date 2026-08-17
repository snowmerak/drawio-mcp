package drawio

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/snowmerak/drawio-mcp/internal/catalog"
)

// Document is an editable draw.io file held by the MCP server.
type Document struct {
	ID       string
	Path     string
	Name     string
	Pages    []*Page
	Modified bool
}

// Page is one diagram page backed by an mxGraphModel.
type Page struct {
	ID    string
	Name  string
	Graph *Node
}

// CellSummary is a compact description of a vertex on a page.
type CellSummary struct {
	ID      string  `json:"id"`
	ShapeID string  `json:"shape_id,omitempty"`
	Label   string  `json:"label,omitempty"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Width   float64 `json:"width"`
	Height  float64 `json:"height"`
}

// PageSummary describes a page and its vertices.
type PageSummary struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	Cells []CellSummary `json:"cells"`
}

// Summary describes the current editable document.
type Summary struct {
	DocumentID string        `json:"document_id"`
	Name       string        `json:"name"`
	Path       string        `json:"path,omitempty"`
	Modified   bool          `json:"modified"`
	Pages      []PageSummary `json:"pages"`
}

// New creates a one-page, in-memory draw.io document.
func New(name, pageName, path string) *Document {
	if strings.TrimSpace(name) == "" {
		name = "Untitled"
	}
	if strings.TrimSpace(pageName) == "" {
		pageName = "Page-1"
	}
	root := element("root")
	root.Children = append(root.Children,
		element("mxCell", "id", "0"),
		element("mxCell", "id", "1", "parent", "0"),
	)
	graph := element("mxGraphModel",
		"dx", "1200", "dy", "800", "grid", "1", "gridSize", "10",
		"guides", "1", "tooltips", "1", "connect", "1", "arrows", "1",
		"fold", "1", "page", "1", "pageScale", "1", "pageWidth", "827",
		"pageHeight", "1169", "math", "0", "shadow", "0",
	)
	graph.Children = append(graph.Children, root)
	return &Document{
		ID:       newID("doc"),
		Path:     path,
		Name:     name,
		Modified: true,
		Pages: []*Page{{
			ID:    newID("page"),
			Name:  pageName,
			Graph: graph,
		}},
	}
}

// Open reads compressed or uncompressed draw.io XML.
func Open(path string) (*Document, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return nil, fmt.Errorf("read draw.io file: %w", err)
	}
	root, err := parseNode(data)
	if err != nil {
		return nil, fmt.Errorf("parse draw.io file: %w", err)
	}
	doc := &Document{ID: newID("doc"), Path: absolute, Name: strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute))}
	switch root.Name.Local {
	case "mxfile":
		for _, diagram := range root.Children {
			if diagram.Name.Local != "diagram" {
				continue
			}
			graph, err := graphFromDiagram(diagram)
			if err != nil {
				return nil, fmt.Errorf("open page %q: %w", diagram.attr("name"), err)
			}
			id := diagram.attr("id")
			if id == "" {
				id = newID("page")
			}
			name := diagram.attr("name")
			if name == "" {
				name = fmt.Sprintf("Page-%d", len(doc.Pages)+1)
			}
			doc.Pages = append(doc.Pages, &Page{ID: id, Name: name, Graph: graph})
		}
	case "mxGraphModel":
		doc.Pages = append(doc.Pages, &Page{ID: newID("page"), Name: "Page-1", Graph: root})
	default:
		return nil, fmt.Errorf("unsupported root element %q", root.Name.Local)
	}
	if len(doc.Pages) == 0 {
		return nil, fmt.Errorf("draw.io file contains no diagram pages")
	}
	return doc, nil
}

func graphFromDiagram(diagram *Node) (*Node, error) {
	if graph := diagram.child("mxGraphModel"); graph != nil {
		return graph, nil
	}
	content := compactText(diagram.Text)
	if content == "" {
		return nil, fmt.Errorf("diagram has no mxGraphModel")
	}
	decoded, err := decodeCompressed(content)
	if err != nil {
		return nil, fmt.Errorf("decode compressed diagram: %w", err)
	}
	graph, err := parseNode(decoded)
	if err != nil {
		return nil, fmt.Errorf("parse decoded mxGraphModel: %w", err)
	}
	if graph.Name.Local != "mxGraphModel" {
		return nil, fmt.Errorf("decoded root is %q, not mxGraphModel", graph.Name.Local)
	}
	return graph, nil
}

func decodeCompressed(data string) ([]byte, error) {
	binary, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}
	reader := flate.NewReader(bytes.NewReader(binary))
	inflated, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	decoded, err := url.PathUnescape(string(inflated))
	if err != nil {
		return nil, err
	}
	return []byte(decoded), nil
}

// Save writes the document as an uncompressed, editable mxfile.
func (d *Document) Save(path string) error {
	if strings.TrimSpace(path) == "" {
		path = d.Path
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("a save path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve save path: %w", err)
	}
	mxfile := element("mxfile",
		"host", "drawio-mcp", "agent", "drawio-mcp",
		"modified", time.Now().UTC().Format(time.RFC3339), "version", "1.0",
	)
	for _, page := range d.Pages {
		diagram := element("diagram", "id", page.ID, "name", page.Name)
		diagram.Children = append(diagram.Children, page.Graph)
		mxfile.Children = append(mxfile.Children, diagram)
	}
	var output bytes.Buffer
	output.WriteString(xml.Header)
	encoder := xml.NewEncoder(&output)
	encoder.Indent("", "  ")
	if err := encoder.Encode(mxfile); err != nil {
		return fmt.Errorf("encode draw.io file: %w", err)
	}
	if err := encoder.Flush(); err != nil {
		return fmt.Errorf("flush draw.io file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		return fmt.Errorf("create save directory: %w", err)
	}
	if err := os.WriteFile(absolute, output.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write draw.io file: %w", err)
	}
	d.Path = absolute
	d.Modified = false
	return nil
}

// Place adds a catalog shape as an mxCell vertex.
func (d *Document) Place(pageRef string, shape catalog.Shape, x, y, width, height float64, label string) (CellSummary, error) {
	page, err := d.page(pageRef)
	if err != nil {
		return CellSummary{}, err
	}
	if !finite(x) || !finite(y) || !finite(width) || !finite(height) {
		return CellSummary{}, fmt.Errorf("coordinates and dimensions must be finite numbers")
	}
	if width == 0 {
		width = shape.Width
	}
	if height == 0 {
		height = shape.Height
	}
	if width <= 0 || height <= 0 {
		return CellSummary{}, fmt.Errorf("width and height must be greater than zero")
	}
	root := page.Graph.child("root")
	if root == nil {
		return CellSummary{}, fmt.Errorf("page %q has no mxGraphModel root", page.Name)
	}
	value := label
	if value == "" {
		value = shape.Value
	}
	cellID := newID("cell")
	cell := element("mxCell",
		"id", cellID, "value", value, "style", shape.Style,
		"vertex", "1", "parent", "1", "drawioMcpShape", shape.ID,
	)
	geometry := element("mxGeometry",
		"x", formatFloat(x), "y", formatFloat(y),
		"width", formatFloat(width), "height", formatFloat(height), "as", "geometry",
	)
	cell.Children = append(cell.Children, geometry)
	root.Children = append(root.Children, cell)
	d.Modified = true
	return CellSummary{ID: cellID, ShapeID: shape.ID, Label: value, X: x, Y: y, Width: width, Height: height}, nil
}

// Inspect returns pages and all vertex cells.
func (d *Document) Inspect() Summary {
	result := Summary{DocumentID: d.ID, Name: d.Name, Path: d.Path, Modified: d.Modified}
	for _, page := range d.Pages {
		pageSummary := PageSummary{ID: page.ID, Name: page.Name, Cells: []CellSummary{}}
		if root := page.Graph.child("root"); root != nil {
			for _, cell := range root.Children {
				if cell.Name.Local != "mxCell" || cell.attr("vertex") != "1" {
					continue
				}
				geometry := cell.child("mxGeometry")
				summary := CellSummary{ID: cell.attr("id"), ShapeID: cell.attr("drawioMcpShape"), Label: cell.attr("value")}
				if geometry != nil {
					summary.X, _ = strconv.ParseFloat(geometry.attr("x"), 64)
					summary.Y, _ = strconv.ParseFloat(geometry.attr("y"), 64)
					summary.Width, _ = strconv.ParseFloat(geometry.attr("width"), 64)
					summary.Height, _ = strconv.ParseFloat(geometry.attr("height"), 64)
				}
				pageSummary.Cells = append(pageSummary.Cells, summary)
			}
		}
		result.Pages = append(result.Pages, pageSummary)
	}
	return result
}

func (d *Document) page(ref string) (*Page, error) {
	if ref == "" && len(d.Pages) > 0 {
		return d.Pages[0], nil
	}
	for _, page := range d.Pages {
		if page.ID == ref || page.Name == ref {
			return page, nil
		}
	}
	return nil, fmt.Errorf("page %q not found", ref)
}

func newID(prefix string) string {
	data := make([]byte, 8)
	if _, err := rand.Read(data); err != nil {
		return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return prefix + "-" + hex.EncodeToString(data)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
