package drawio_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/snowmerak/drawio-mcp/internal/catalog"
	"github.com/snowmerak/drawio-mcp/internal/drawio"
)

func TestMoveLabelAndDeleteShape(t *testing.T) {
	doc := drawio.New("Editing", "Page-1", "")
	shape := catalog.Shape{ID: "test.box", Name: "Box", Width: 40, Height: 40, Style: "whiteSpace=wrap;html=1;"}
	source, _ := doc.Place("", shape, 0, 0, 40, 40, "Source")
	target, _ := doc.Place("", shape, 240, 0, 40, 40, "Target")
	routed, err := doc.RouteEdges("", []drawio.RouteConnection{{SourceID: source.ID, TargetID: target.ID}}, drawio.RouteOptions{})
	if err != nil {
		t.Fatal(err)
	}

	moved, err := doc.MoveShape("", target.ID, 300, 100)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Cell.X != 300 || moved.Cell.Y != 100 {
		t.Fatalf("moved cell = %+v", moved.Cell)
	}
	labeled, err := doc.SetShapeLabel("", target.ID, "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	if labeled.Cell.Label != "Renamed" {
		t.Fatalf("labeled cell = %+v", labeled.Cell)
	}

	deleted, err := doc.DeleteShape("", target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted.DeletedShapeIDs) != 1 || deleted.DeletedShapeIDs[0] != target.ID {
		t.Fatalf("deleted shapes = %+v", deleted.DeletedShapeIDs)
	}
	if len(deleted.DeletedEdgeIDs) != 1 || deleted.DeletedEdgeIDs[0] != routed.Edges[0].ID {
		t.Fatalf("deleted edges = %+v", deleted.DeletedEdgeIDs)
	}
	inspected, err := doc.InspectRegion("", drawio.Bounds{X: -100, Y: -100, Width: 600, Height: 400}, drawio.RegionOptions{EdgeMode: drawio.EdgesConnected})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspected.Nodes) != 1 || inspected.Nodes[0].ID != source.ID || len(inspected.Edges) != 0 {
		t.Fatalf("document after delete = %+v", inspected)
	}
}

func TestEditNestedShapesAndDeleteGroup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested.drawio")
	contents := `<?xml version="1.0" encoding="UTF-8"?>
<mxfile><diagram id="page-1" name="Nested"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="group" value="Group" vertex="1" parent="1"><mxGeometry x="100" y="100" width="300" height="200" as="geometry"/></mxCell>
  <object id="nested" label="Old"><mxCell vertex="1" parent="group"><mxGeometry x="0.5" y="0.5" width="20" height="20" relative="1" as="geometry"><mxPoint x="5" y="-5" as="offset"/></mxGeometry></mxCell></object>
  <mxCell id="outside" value="Outside" vertex="1" parent="1"><mxGeometry x="500" y="100" width="40" height="40" as="geometry"/></mxCell>
  <mxCell id="edge" edge="1" source="nested" target="outside" parent="1"><mxGeometry relative="1" as="geometry"/></mxCell>
</root></mxGraphModel></diagram></mxfile>`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := drawio.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.MoveShape("Nested", "nested", 300, 220); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.SetShapeLabel("Nested", "nested", "New"); err != nil {
		t.Fatal(err)
	}
	region, err := doc.InspectRegion("Nested", drawio.Bounds{X: 295, Y: 215, Width: 30, Height: 30}, drawio.RegionOptions{Match: drawio.RegionContained, EdgeMode: drawio.EdgesNone})
	if err != nil {
		t.Fatal(err)
	}
	if len(region.Nodes) != 1 || region.Nodes[0].ID != "nested" || region.Nodes[0].Label != "New" || region.Nodes[0].Bounds.X != 300 || region.Nodes[0].Bounds.Y != 220 {
		t.Fatalf("edited nested shape = %+v", region.Nodes)
	}

	deleted, err := doc.DeleteShape("Nested", "group")
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted.DeletedShapeIDs) != 2 || len(deleted.DeletedEdgeIDs) != 1 || deleted.DeletedEdgeIDs[0] != "edge" {
		t.Fatalf("group deletion = %+v", deleted)
	}
	remaining, err := doc.InspectRegion("Nested", drawio.Bounds{X: 0, Y: 0, Width: 600, Height: 400}, drawio.RegionOptions{EdgeMode: drawio.EdgesConnected})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining.Nodes) != 1 || remaining.Nodes[0].ID != "outside" || len(remaining.Edges) != 0 {
		t.Fatalf("remaining document = %+v", remaining)
	}
}
