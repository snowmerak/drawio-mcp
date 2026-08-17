package drawio

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCompressedDiagram(t *testing.T) {
	model := `<mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/><mxCell id="2" value="Hello" style="whiteSpace=wrap;html=1;" vertex="1" parent="1"><mxGeometry x="20" y="30" width="120" height="60" as="geometry"/></mxCell></root></mxGraphModel>`
	encoded := strings.ReplaceAll(url.QueryEscape(model), "+", "%20")
	var compressed bytes.Buffer
	writer, err := flate.NewWriter(&compressed, flate.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(encoded)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	payload := base64.StdEncoding.EncodeToString(compressed.Bytes())
	path := filepath.Join(t.TempDir(), "compressed.drawio")
	contents := `<mxfile><diagram id="page-1" name="Page-1">` + payload + `</diagram></mxfile>`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	summary := doc.Inspect()
	if len(summary.Pages) != 1 || len(summary.Pages[0].Cells) != 1 {
		t.Fatalf("unexpected compressed document: %+v", summary)
	}
	if summary.Pages[0].Cells[0].Label != "Hello" {
		t.Fatalf("unexpected cell: %+v", summary.Pages[0].Cells[0])
	}
}
