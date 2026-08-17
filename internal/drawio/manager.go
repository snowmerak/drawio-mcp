package drawio

import (
	"fmt"
	"sync"

	"github.com/snowmerak/drawio-mcp/internal/catalog"
)

// Manager owns the documents opened during the MCP server process lifetime.
type Manager struct {
	mu   sync.RWMutex
	docs map[string]*Document
}

// CloseResult confirms that an in-memory document was released.
type CloseResult struct {
	DocumentID       string `json:"document_id"`
	Path             string `json:"path,omitempty"`
	Closed           bool   `json:"closed"`
	DiscardedChanges bool   `json:"discarded_changes"`
}

func NewManager() *Manager {
	return &Manager{docs: make(map[string]*Document)}
}

func (m *Manager) Create(name, pageName, path string) Summary {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc := New(name, pageName, path)
	m.docs[doc.ID] = doc
	return doc.Inspect()
}

func (m *Manager) Open(path string) (Summary, error) {
	doc, err := Open(path)
	if err != nil {
		return Summary{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.docs[doc.ID] = doc
	return doc.Inspect(), nil
}

func (m *Manager) Save(documentID, path string) (Summary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, err := m.document(documentID)
	if err != nil {
		return Summary{}, err
	}
	if err := doc.Save(path); err != nil {
		return Summary{}, err
	}
	return doc.Inspect(), nil
}

// Close releases a document from the manager. Modified documents require an
// explicit force flag so an accidental close cannot silently discard edits.
func (m *Manager) Close(documentID string, force bool) (CloseResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, err := m.document(documentID)
	if err != nil {
		return CloseResult{}, err
	}
	if doc.Modified && !force {
		return CloseResult{}, fmt.Errorf("document %q has unsaved changes; save it or set force to true", documentID)
	}
	result := CloseResult{
		DocumentID:       doc.ID,
		Path:             doc.Path,
		Closed:           true,
		DiscardedChanges: doc.Modified,
	}
	delete(m.docs, documentID)
	return result, nil
}

func (m *Manager) Place(documentID, pageRef string, shape catalog.Shape, x, y, width, height float64, label string) (CellSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, err := m.document(documentID)
	if err != nil {
		return CellSummary{}, err
	}
	return doc.Place(pageRef, shape, x, y, width, height, label)
}

func (m *Manager) MoveShape(documentID, pageRef, shapeID string, x, y float64) (ShapeEditResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, err := m.document(documentID)
	if err != nil {
		return ShapeEditResult{}, err
	}
	return doc.MoveShape(pageRef, shapeID, x, y)
}

func (m *Manager) SetShapeLabel(documentID, pageRef, shapeID, label string) (ShapeEditResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, err := m.document(documentID)
	if err != nil {
		return ShapeEditResult{}, err
	}
	return doc.SetShapeLabel(pageRef, shapeID, label)
}

func (m *Manager) DeleteShape(documentID, pageRef, shapeID string) (DeleteShapeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, err := m.document(documentID)
	if err != nil {
		return DeleteShapeResult{}, err
	}
	return doc.DeleteShape(pageRef, shapeID)
}

func (m *Manager) Inspect(documentID string) (Summary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	doc, err := m.document(documentID)
	if err != nil {
		return Summary{}, err
	}
	return doc.Inspect(), nil
}

func (m *Manager) InspectRegion(documentID, pageRef string, region Bounds, options RegionOptions) (RegionResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	doc, err := m.document(documentID)
	if err != nil {
		return RegionResult{}, err
	}
	return doc.InspectRegion(pageRef, region, options)
}

func (m *Manager) RouteEdges(documentID, pageRef string, connections []RouteConnection, options RouteOptions) (RouteEdgesResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, err := m.document(documentID)
	if err != nil {
		return RouteEdgesResult{}, err
	}
	return doc.RouteEdges(pageRef, connections, options)
}

func (m *Manager) document(id string) (*Document, error) {
	doc, ok := m.docs[id]
	if !ok {
		return nil, fmt.Errorf("document %q is not open", id)
	}
	return doc, nil
}
