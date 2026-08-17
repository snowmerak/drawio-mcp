package drawio_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/snowmerak/drawio-mcp/internal/catalog"
	"github.com/snowmerak/drawio-mcp/internal/drawio"
	templates "github.com/snowmerak/drawio-mcp/template"
)

func TestCreatePlaceSaveAndOpen(t *testing.T) {
	shapeCatalog, err := catalog.Load(templates.FS)
	if err != nil {
		t.Fatal(err)
	}
	shape, ok := shapeCatalog.Get("cloudflare.wrangler")
	if !ok {
		t.Fatal("cloudflare.wrangler is missing")
	}
	doc := drawio.New("Example", "Architecture", "")
	cell, err := doc.Place("", shape, 120, 80, 0, 0, "Worker CLI")
	if err != nil {
		t.Fatal(err)
	}
	if cell.Width != shape.Width || cell.Height != shape.Height {
		t.Fatalf("default dimensions not applied: %+v", cell)
	}
	path := filepath.Join(t.TempDir(), "example.drawio")
	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("saved file is empty")
	}
	reopened, err := drawio.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	summary := reopened.Inspect()
	if len(summary.Pages) != 1 || len(summary.Pages[0].Cells) != 1 {
		t.Fatalf("unexpected reopened document: %+v", summary)
	}
	if got := summary.Pages[0].Cells[0].ShapeID; got != "cloudflare.wrangler" {
		t.Fatalf("shape ID = %q", got)
	}
}
