package drawio_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/snowmerak/drawio-mcp/internal/drawio"
)

func TestInspectRegionResolvesGroupsObjectsAndEdges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "semantic.drawio")
	contents := `<?xml version="1.0" encoding="UTF-8"?>
<mxfile><diagram id="page-1" name="Architecture"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="group" value="Service Group" vertex="1" parent="1"><mxGeometry x="100" y="100" width="300" height="200" as="geometry"/></mxCell>
  <object id="nested" label="Nested API" owner="team-a"><mxCell style="rounded=1;image=data:image/svg+xml,PHN2Zw==;" vertex="1" parent="group"><mxGeometry x="20" y="30" width="80" height="40" as="geometry"/></mxCell></object>
  <mxCell id="relative" value="Relative Port" vertex="1" parent="group"><mxGeometry x="0.5" y="0.5" width="20" height="20" relative="1" as="geometry"><mxPoint x="5" y="-5" as="offset"/></mxGeometry></mxCell>
  <mxCell id="outside" value="Database" style="shape=cylinder3;" vertex="1" parent="1"><mxGeometry x="500" y="130" width="80" height="80" as="geometry"/></mxCell>
  <mxCell id="edge-1" value="query" edge="1" source="nested" target="outside" parent="1" style="edgeStyle=orthogonalEdgeStyle;"><mxGeometry relative="1" as="geometry"><Array as="points"><mxPoint x="300" y="150"/></Array></mxGeometry></mxCell>
</root></mxGraphModel></diagram></mxfile>`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := drawio.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	result, err := doc.InspectRegion("Architecture", drawio.Bounds{X: 110, Y: 120, Width: 120, Height: 100}, drawio.RegionOptions{
		Match: drawio.RegionContained, EdgeMode: drawio.EdgesConnected,
		IncludeExternalNodes: true, IncludeStyle: true, IncludeMetadata: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("nodes = %+v", result.Nodes)
	}
	nested := result.Nodes[0]
	if nested.ID != "nested" || !nested.InRegion || nested.Bounds.X != 120 || nested.Bounds.Y != 130 {
		t.Fatalf("nested node = %+v", nested)
	}
	if nested.Metadata["owner"] != "team-a" {
		t.Fatalf("nested metadata = %+v", nested.Metadata)
	}
	if nested.Style["image"] != "embedded:image/svg+xml" {
		t.Fatalf("embedded style was not sanitized: %+v", nested.Style)
	}
	if result.Nodes[1].ID != "outside" || result.Nodes[1].InRegion {
		t.Fatalf("external node = %+v", result.Nodes[1])
	}
	if len(result.Edges) != 1 || len(result.Edges[0].Points) != 3 {
		t.Fatalf("edges = %+v", result.Edges)
	}

	crossing, err := doc.InspectRegion("Architecture", drawio.Bounds{X: 420, Y: 145, Width: 40, Height: 20}, drawio.RegionOptions{
		EdgeMode: drawio.EdgesIntersects,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(crossing.Nodes) != 0 || len(crossing.Edges) != 1 || crossing.Edges[0].ID != "edge-1" {
		t.Fatalf("crossing edge query = %+v", crossing)
	}

	relative, err := doc.InspectRegion("Architecture", drawio.Bounds{X: 250, Y: 190, Width: 30, Height: 30}, drawio.RegionOptions{
		Match: drawio.RegionContained, EdgeMode: drawio.EdgesNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(relative.Nodes) != 1 || relative.Nodes[0].ID != "relative" || relative.Nodes[0].Bounds.X != 255 || relative.Nodes[0].Bounds.Y != 195 {
		t.Fatalf("relative geometry query = %+v", relative)
	}
}

func TestInspectRegionRejectsInvalidOptions(t *testing.T) {
	doc := drawio.New("Invalid", "Page-1", "")
	_, err := doc.InspectRegion("", drawio.Bounds{Width: 10, Height: 10}, drawio.RegionOptions{EdgeMode: "invalid"})
	if err == nil {
		t.Fatal("invalid edge mode was accepted")
	}
}
