package catalog_test

import (
	"strings"
	"testing"

	"github.com/snowmerak/drawio-mcp/internal/catalog"
	templates "github.com/snowmerak/drawio-mcp/template"
)

func TestLoadEmbeddedCatalog(t *testing.T) {
	c, err := catalog.Load(templates.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"default.rectangle", "saturday.golang", "cloudflare.wrangler"} {
		shape, ok := c.Get(id)
		if !ok {
			t.Errorf("expected embedded shape %q", id)
			continue
		}
		if shape.Width <= 0 || shape.Height <= 0 || shape.Style == "" {
			t.Errorf("shape %q is not placeable: %+v", id, shape)
		}
	}
	results := c.Find("golang", "saturday", "", nil, 10)
	if len(results) == 0 || results[0].ID != "saturday.golang" {
		t.Fatalf("unexpected search results: %+v", results)
	}

	cloudflare, ok := c.Get("cloudflare.wrangler")
	if !ok {
		t.Fatal("cloudflare.wrangler is missing")
	}
	if strings.Contains(cloudflare.Style, ";base64,") {
		t.Fatalf("Cloudflare image contains an mxGraph style delimiter: %q", cloudflare.Style)
	}
	if !strings.Contains(cloudflare.Style, "image=data:image/svg+xml,PHN2") {
		t.Fatalf("Cloudflare image was not normalized: %q", cloudflare.Style)
	}
}
