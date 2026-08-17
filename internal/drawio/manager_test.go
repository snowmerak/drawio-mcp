package drawio_test

import (
	"testing"

	"github.com/snowmerak/drawio-mcp/internal/drawio"
)

func TestManagerCloseProtectsUnsavedChanges(t *testing.T) {
	manager := drawio.NewManager()
	created := manager.Create("Close", "Page-1", "")
	if _, err := manager.Close(created.DocumentID, false); err == nil {
		t.Fatal("modified document was closed without force")
	}
	if _, err := manager.Inspect(created.DocumentID); err != nil {
		t.Fatalf("rejected close removed document: %v", err)
	}
	closed, err := manager.Close(created.DocumentID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !closed.Closed || !closed.DiscardedChanges {
		t.Fatalf("close result = %+v", closed)
	}
	if _, err := manager.Inspect(created.DocumentID); err == nil {
		t.Fatal("closed document is still available")
	}
}
