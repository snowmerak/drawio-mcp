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

func (m *Manager) Place(documentID, pageRef string, shape catalog.Shape, x, y, width, height float64, label string) (CellSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, err := m.document(documentID)
	if err != nil {
		return CellSummary{}, err
	}
	return doc.Place(pageRef, shape, x, y, width, height, label)
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

func (m *Manager) document(id string) (*Document, error) {
	doc, ok := m.docs[id]
	if !ok {
		return nil, fmt.Errorf("document %q is not open", id)
	}
	return doc, nil
}
