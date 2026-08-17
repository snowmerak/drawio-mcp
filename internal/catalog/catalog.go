package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Shape is the normalized representation of a built-in or library shape.
type Shape struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Category    string   `json:"category"`
	Source      string   `json:"source"`
	Width       float64  `json:"width"`
	Height      float64  `json:"height"`
	Style       string   `json:"-"`
	Value       string   `json:"-"`
}

type libraryItem struct {
	XML    string  `json:"xml"`
	Data   string  `json:"data"`
	Width  float64 `json:"w"`
	Height float64 `json:"h"`
	Title  string  `json:"title"`
	Aspect string  `json:"aspect"`
}

type graphModel struct {
	Root struct {
		Cells []graphCell `xml:"mxCell"`
	} `xml:"root"`
}

type graphCell struct {
	ID       string `xml:"id,attr"`
	Value    string `xml:"value,attr"`
	Style    string `xml:"style,attr"`
	Vertex   string `xml:"vertex,attr"`
	Geometry struct {
		Width  string `xml:"width,attr"`
		Height string `xml:"height,attr"`
	} `xml:"mxGeometry"`
}

// Catalog is an immutable, searchable shape collection.
type Catalog struct {
	shapes []Shape
	byID   map[string]int
}

// Load creates a catalog from the built-in shapes and all mxlibrary XML files.
func Load(libraries fs.FS) (*Catalog, error) {
	shapes := append([]Shape(nil), defaultShapes...)
	paths, err := fs.Glob(libraries, "*.xml")
	if err != nil {
		return nil, fmt.Errorf("find embedded libraries: %w", err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		loaded, err := loadLibrary(libraries, path)
		if err != nil {
			return nil, err
		}
		shapes = append(shapes, loaded...)
	}

	byID := make(map[string]int, len(shapes))
	for i := range shapes {
		base := shapes[i].ID
		if _, exists := byID[base]; exists {
			hash := sha256.Sum256([]byte(shapes[i].Source + "\x00" + shapes[i].Name + "\x00" + shapes[i].Style))
			shapes[i].ID = base + "-" + hex.EncodeToString(hash[:4])
		}
		byID[shapes[i].ID] = i
	}
	return &Catalog{shapes: shapes, byID: byID}, nil
}

func loadLibrary(fsys fs.FS, path string) ([]Shape, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("read embedded library %q: %w", path, err)
	}
	var container struct {
		Text string `xml:",chardata"`
	}
	if err := xml.Unmarshal(data, &container); err != nil {
		return nil, fmt.Errorf("parse embedded library %q: %w", path, err)
	}
	var items []libraryItem
	if err := json.Unmarshal([]byte(strings.TrimSpace(container.Text)), &items); err != nil {
		return nil, fmt.Errorf("decode embedded library %q: %w", path, err)
	}

	source := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	result := make([]Shape, 0, len(items))
	for i, item := range items {
		name := strings.TrimSpace(item.Title)
		if name == "" {
			name = fmt.Sprintf("shape-%d", i+1)
		}
		shape := Shape{
			ID:       source + "." + slug(name),
			Name:     name,
			Tags:     tagsFor(name, source),
			Category: source,
			Source:   source,
			Width:    item.Width,
			Height:   item.Height,
		}
		if shape.Width <= 0 {
			shape.Width = 80
		}
		if shape.Height <= 0 {
			shape.Height = 80
		}

		switch {
		case item.Data != "":
			shape.Style = "shape=image;verticalLabelPosition=bottom;verticalAlign=top;imageAspect=0;aspect=fixed;image=" + item.Data
		case item.XML != "":
			style, value, width, height, err := extractLibraryCell(item.XML)
			if err != nil {
				return nil, fmt.Errorf("parse shape %q in %q: %w", name, path, err)
			}
			shape.Style, shape.Value = style, value
			if item.Width <= 0 && width > 0 {
				shape.Width = width
			}
			if item.Height <= 0 && height > 0 {
				shape.Height = height
			}
		default:
			continue
		}
		result = append(result, shape)
	}
	return result, nil
}

func extractLibraryCell(data string) (string, string, float64, float64, error) {
	var model graphModel
	if err := xml.Unmarshal([]byte(data), &model); err != nil {
		return "", "", 0, 0, err
	}
	for _, cell := range model.Root.Cells {
		if cell.Vertex != "1" || cell.Style == "" {
			continue
		}
		width, _ := strconv.ParseFloat(cell.Geometry.Width, 64)
		height, _ := strconv.ParseFloat(cell.Geometry.Height, 64)
		return cell.Style, cell.Value, width, height, nil
	}
	return "", "", 0, 0, fmt.Errorf("library entry has no placeable vertex")
}

// Get returns a shape by its stable ID.
func (c *Catalog) Get(id string) (Shape, bool) {
	i, ok := c.byID[id]
	if !ok {
		return Shape{}, false
	}
	return c.shapes[i], true
}

// List filters shapes by source, category, and tags.
func (c *Catalog) List(source, category string, tags []string, limit int) []Shape {
	return c.search("", source, category, tags, limit)
}

// Find searches IDs, names, descriptions, and tags.
func (c *Catalog) Find(query, source, category string, tags []string, limit int) []Shape {
	return c.search(query, source, category, tags, limit)
}

func (c *Catalog) search(query, source, category string, tags []string, limit int) []Shape {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query = strings.ToLower(strings.TrimSpace(query))
	source = strings.ToLower(strings.TrimSpace(source))
	category = strings.ToLower(strings.TrimSpace(category))
	requiredTags := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag = strings.ToLower(strings.TrimSpace(tag)); tag != "" {
			requiredTags = append(requiredTags, tag)
		}
	}
	type match struct {
		shape Shape
		score int
	}
	matches := make([]match, 0)
	for _, shape := range c.shapes {
		if source != "" && strings.ToLower(shape.Source) != source {
			continue
		}
		if category != "" && strings.ToLower(shape.Category) != category {
			continue
		}
		if !hasAllTags(shape.Tags, requiredTags) {
			continue
		}
		score := scoreShape(shape, query)
		if query != "" && score == 0 {
			continue
		}
		matches = append(matches, match{shape: shape, score: score})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].shape.ID < matches[j].shape.ID
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	result := make([]Shape, len(matches))
	for i := range matches {
		result[i] = matches[i].shape
	}
	return result
}

func scoreShape(shape Shape, query string) int {
	if query == "" {
		return 1
	}
	id, name := strings.ToLower(shape.ID), strings.ToLower(shape.Name)
	switch {
	case id == query:
		return 1000
	case name == query:
		return 900
	case strings.HasPrefix(id, query):
		return 700
	case strings.HasPrefix(name, query):
		return 650
	}
	score := 0
	if strings.Contains(id, query) {
		score += 400
	}
	if strings.Contains(name, query) {
		score += 350
	}
	if strings.Contains(strings.ToLower(shape.Description), query) {
		score += 150
	}
	for _, tag := range shape.Tags {
		lower := strings.ToLower(tag)
		if lower == query {
			score += 300
		} else if strings.Contains(lower, query) {
			score += 100
		}
	}
	for _, token := range strings.Fields(query) {
		if strings.Contains(id+" "+name+" "+strings.Join(shape.Tags, " "), token) {
			score += 25
		}
	}
	return score
}

func hasAllTags(actual, required []string) bool {
	for _, wanted := range required {
		found := false
		for _, tag := range actual {
			if strings.EqualFold(tag, wanted) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func tagsFor(name, source string) []string {
	tags := []string{source}
	seen := map[string]bool{source: true}
	for _, token := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if token != "" && !seen[token] {
			tags = append(tags, token)
			seen[token] = true
		}
	}
	return tags
}

func slug(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
