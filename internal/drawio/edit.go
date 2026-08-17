package drawio

import (
	"fmt"
	"sort"
)

// ShapeEditResult describes a shape after an edit.
type ShapeEditResult struct {
	DocumentID string      `json:"document_id"`
	Page       PageRef     `json:"page"`
	Cell       CellSummary `json:"cell"`
}

// DeleteShapeResult describes the vertices and connected edges removed by a
// shape deletion. Deleting a group also deletes its descendant vertices.
type DeleteShapeResult struct {
	DocumentID      string   `json:"document_id"`
	Page            PageRef  `json:"page"`
	DeletedShapeIDs []string `json:"deleted_shape_ids"`
	DeletedEdgeIDs  []string `json:"deleted_edge_ids"`
}

// MoveShape moves a vertex so its top-left corner is at an absolute page
// coordinate. Nested and relative geometry is converted back to parent-local
// coordinates while preserving its representation.
func (d *Document) MoveShape(pageRef, shapeID string, x, y float64) (ShapeEditResult, error) {
	if !finite(x) || !finite(y) {
		return ShapeEditResult{}, fmt.Errorf("coordinates must be finite numbers")
	}
	page, semantic, cell, err := d.editableShape(pageRef, shapeID)
	if err != nil {
		return ShapeEditResult{}, err
	}
	geometry := cell.geometry
	localX, localY := x, y
	if parent := semantic.byID[cell.parentID]; parent != nil && parent.vertex {
		parentBounds := semantic.bounds[parent.id]
		localX, localY = x-parentBounds.X, y-parentBounds.Y
		if geometry.attr("relative") == "1" {
			if parentBounds.Width <= 0 || parentBounds.Height <= 0 {
				return ShapeEditResult{}, fmt.Errorf("parent shape %q has no positive geometry", parent.id)
			}
			localX /= parentBounds.Width
			localY /= parentBounds.Height
			if offset := geometryPointNode(geometry, "offset"); offset != nil {
				offset.setAttr("x", "0")
				offset.setAttr("y", "0")
			}
		}
	}
	geometry.setAttr("x", formatFloat(localX))
	geometry.setAttr("y", formatFloat(localY))
	d.Modified = true
	bounds := semantic.bounds[cell.id]
	bounds.X, bounds.Y = x, y
	return ShapeEditResult{
		DocumentID: d.ID,
		Page:       PageRef{ID: page.ID, Name: page.Name},
		Cell:       cellSummary(cell, bounds),
	}, nil
}

// SetShapeLabel changes the visible label of a vertex. For object/UserObject
// wrappers both the wrapper label and inner mxCell value are kept in sync.
func (d *Document) SetShapeLabel(pageRef, shapeID, label string) (ShapeEditResult, error) {
	page, semantic, cell, err := d.editableShape(pageRef, shapeID)
	if err != nil {
		return ShapeEditResult{}, err
	}
	cell.node.setAttr("value", label)
	if cell.wrapper != nil {
		cell.wrapper.setAttr("label", label)
	}
	d.Modified = true
	cell.label = label
	return ShapeEditResult{
		DocumentID: d.ID,
		Page:       PageRef{ID: page.ID, Name: page.Name},
		Cell:       cellSummary(cell, semantic.bounds[cell.id]),
	}, nil
}

// DeleteShape removes a vertex, all of its descendant vertices, and every edge
// connected to any removed vertex so the document has no dangling endpoints.
func (d *Document) DeleteShape(pageRef, shapeID string) (DeleteShapeResult, error) {
	page, semantic, _, err := d.editableShape(pageRef, shapeID)
	if err != nil {
		return DeleteShapeResult{}, err
	}
	deletedIDs := map[string]bool{shapeID: true}
	for changed := true; changed; {
		changed = false
		for _, cell := range semantic.cells {
			if cell.vertex && cell.id != "" && !deletedIDs[cell.id] && deletedIDs[cell.parentID] {
				deletedIDs[cell.id] = true
				changed = true
			}
		}
	}

	remove := make(map[*Node]bool)
	result := DeleteShapeResult{
		DocumentID: d.ID,
		Page:       PageRef{ID: page.ID, Name: page.Name},
	}
	for _, cell := range semantic.cells {
		switch {
		case cell.vertex && deletedIDs[cell.id]:
			result.DeletedShapeIDs = append(result.DeletedShapeIDs, cell.id)
			remove[removalNode(cell)] = true
		case cell.edge && (deletedIDs[cell.sourceID] || deletedIDs[cell.targetID]):
			result.DeletedEdgeIDs = append(result.DeletedEdgeIDs, cell.id)
			remove[removalNode(cell)] = true
		}
	}
	sort.Strings(result.DeletedShapeIDs)
	sort.Strings(result.DeletedEdgeIDs)
	removeNodes(page.Graph, remove)
	d.Modified = true
	return result, nil
}

func (d *Document) editableShape(pageRef, shapeID string) (*Page, *semanticPage, *semanticCell, error) {
	if shapeID == "" {
		return nil, nil, nil, fmt.Errorf("shape_id is required")
	}
	page, err := d.page(pageRef)
	if err != nil {
		return nil, nil, nil, err
	}
	semantic, err := buildSemanticPage(page)
	if err != nil {
		return nil, nil, nil, err
	}
	cell := semantic.byID[shapeID]
	if cell == nil || !cell.vertex {
		return nil, nil, nil, fmt.Errorf("shape %q not found on page %q", shapeID, page.Name)
	}
	if cell.geometry == nil {
		return nil, nil, nil, fmt.Errorf("shape %q has no geometry", shapeID)
	}
	return page, semantic, cell, nil
}

func cellSummary(cell *semanticCell, bounds Bounds) CellSummary {
	return CellSummary{
		ID: cell.id, ShapeID: cell.shapeID, Label: cell.label,
		X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height,
	}
}

func removalNode(cell *semanticCell) *Node {
	if cell.wrapper != nil {
		return cell.wrapper
	}
	return cell.node
}

func geometryPointNode(geometry *Node, kind string) *Node {
	for _, child := range geometry.Children {
		if child.Name.Local == "mxPoint" && child.attr("as") == kind {
			return child
		}
	}
	return nil
}

func removeNodes(parent *Node, remove map[*Node]bool) {
	children := parent.Children[:0]
	for _, child := range parent.Children {
		if remove[child] {
			continue
		}
		removeNodes(child, remove)
		children = append(children, child)
	}
	parent.Children = children
}
