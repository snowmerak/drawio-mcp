package drawio_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/snowmerak/drawio-mcp/internal/catalog"
	"github.com/snowmerak/drawio-mcp/internal/drawio"
)

func TestRouteEdgesSharesAndRestoresLaneOccupancy(t *testing.T) {
	doc := drawio.New("Routing", "Page-1", "")
	shape := catalog.Shape{ID: "test.box", Name: "Box", Width: 40, Height: 40, Style: "whiteSpace=wrap;html=1;"}
	source, err := doc.Place("", shape, 0, 0, 40, 40, "Source")
	if err != nil {
		t.Fatal(err)
	}
	target, err := doc.Place("", shape, 240, 0, 40, 40, "Target")
	if err != nil {
		t.Fatal(err)
	}

	first, err := doc.RouteEdges("", []drawio.RouteConnection{
		{SourceID: source.ID, TargetID: target.ID},
		{SourceID: source.ID, TargetID: target.ID},
	}, drawio.RouteOptions{GridSize: 20, Clearance: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Edges) != 2 || first.SharedLanes == 0 {
		t.Fatalf("expected shared routed edges: %+v", first)
	}
	if len(first.Edges[0].Points) < 2 || len(first.Edges[0].Lanes) == 0 {
		t.Fatalf("route has no waypoints or lanes: %+v", first.Edges[0])
	}
	if !containsString(first.Edges[0].SharedWith, first.Edges[1].ID) {
		t.Fatalf("first edge does not report sharing: %+v", first.Edges)
	}

	path := filepath.Join(t.TempDir(), "routed.drawio")
	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := drawio.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	third, err := reopened.RouteEdges("", []drawio.RouteConnection{{
		SourceID: source.ID, TargetID: target.ID,
	}}, drawio.RouteOptions{GridSize: 20, Clearance: 20})
	if err != nil {
		t.Fatal(err)
	}
	if third.SharedLanes == 0 || len(third.Edges) != 1 {
		t.Fatalf("saved occupancy was not restored: %+v", third)
	}
	if !containsString(third.Edges[0].SharedWith, first.Edges[0].ID) && !containsString(third.Edges[0].SharedWith, first.Edges[1].ID) {
		t.Fatalf("new edge did not share with a saved edge: %+v", third.Edges[0])
	}

	inspected, err := reopened.InspectRegion("", drawio.Bounds{X: -50, Y: -50, Width: 400, Height: 200}, drawio.RegionOptions{
		EdgeMode: drawio.EdgesConnected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspected.Edges) != 3 {
		t.Fatalf("routed edge count = %d, want 3", len(inspected.Edges))
	}
}

func TestRouteEdgesLabeledConnectionsUseExclusiveLanes(t *testing.T) {
	doc := drawio.New("Labels", "Page-1", "")
	shape := catalog.Shape{ID: "test.box", Name: "Box", Width: 40, Height: 40, Style: "whiteSpace=wrap;html=1;"}
	source, _ := doc.Place("", shape, 0, 0, 40, 40, "Source")
	target, _ := doc.Place("", shape, 240, 0, 40, 40, "Target")
	labeled, err := doc.RouteEdges("", []drawio.RouteConnection{
		{SourceID: source.ID, TargetID: target.ID, Label: "request"},
		{SourceID: source.ID, TargetID: target.ID, Label: "forward"},
	}, drawio.RouteOptions{GridSize: 20, Clearance: 20})
	if err != nil {
		t.Fatal(err)
	}
	if labeled.SharedLanes != 0 || routedEdgesShareLane(labeled.Edges[0], labeled.Edges[1]) {
		t.Fatalf("labeled edges shared lanes: %+v", labeled)
	}

	path := filepath.Join(t.TempDir(), "labeled.drawio")
	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := drawio.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	later, err := reopened.RouteEdges("", []drawio.RouteConnection{{SourceID: source.ID, TargetID: target.ID}}, drawio.RouteOptions{GridSize: 20, Clearance: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range labeled.Edges {
		if routedEdgesShareLane(edge, later.Edges[0]) {
			t.Fatalf("later edge shared restored labeled lanes: labeled=%+v later=%+v", edge, later.Edges[0])
		}
	}
}

func TestRouteEdgesOppositeDirectionDoesNotReuseLaneInReverse(t *testing.T) {
	doc := drawio.New("Opposite", "Page-1", "")
	shape := catalog.Shape{ID: "test.box", Name: "Box", Width: 40, Height: 40, Style: "whiteSpace=wrap;html=1;"}
	left, _ := doc.Place("", shape, 0, 0, 40, 40, "Left")
	right, _ := doc.Place("", shape, 240, 0, 40, 40, "Right")
	forward, err := doc.RouteEdges("", []drawio.RouteConnection{{SourceID: left.ID, TargetID: right.ID}}, drawio.RouteOptions{GridSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := doc.RouteEdges("", []drawio.RouteConnection{{SourceID: right.ID, TargetID: left.ID}}, drawio.RouteOptions{GridSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	forwardDirections := laneDirections(forward.Edges[0].Lanes)
	for _, lane := range reverse.Edges[0].Lanes {
		base, direction := splitLaneDirection(lane)
		if existing, ok := forwardDirections[base]; ok && existing != direction {
			t.Fatalf("opposite edge reused %q in reverse", base)
		}
	}
}

func TestRouteEdgesIsAtomicOnInvalidConnection(t *testing.T) {
	doc := drawio.New("Atomic", "Page-1", "")
	shape := catalog.Shape{ID: "test.box", Name: "Box", Width: 40, Height: 40, Style: "whiteSpace=wrap;html=1;"}
	source, _ := doc.Place("", shape, 0, 0, 40, 40, "Source")
	target, _ := doc.Place("", shape, 240, 0, 40, 40, "Target")
	_, err := doc.RouteEdges("", []drawio.RouteConnection{
		{SourceID: source.ID, TargetID: target.ID},
		{SourceID: source.ID, TargetID: "missing"},
	}, drawio.RouteOptions{})
	if err == nil {
		t.Fatal("invalid batch was accepted")
	}
	inspected, inspectErr := doc.InspectRegion("", drawio.Bounds{X: -100, Y: -100, Width: 500, Height: 300}, drawio.RegionOptions{EdgeMode: drawio.EdgesConnected})
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	if len(inspected.Edges) != 0 {
		t.Fatalf("failed batch added partial edges: %+v", inspected.Edges)
	}
}

func laneDirections(lanes []string) map[string]string {
	result := make(map[string]string)
	for _, lane := range lanes {
		base, direction := splitLaneDirection(lane)
		result[base] = direction
	}
	return result
}

func splitLaneDirection(lane string) (string, string) {
	index := strings.LastIndex(lane, ":")
	if index < 0 {
		return lane, ""
	}
	return lane[:index], lane[index+1:]
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func routedEdgesShareLane(a, b drawio.RoutedEdge) bool {
	lanes := make(map[string]bool, len(a.Lanes))
	for _, lane := range a.Lanes {
		lanes[lane] = true
	}
	for _, lane := range b.Lanes {
		if lanes[lane] {
			return true
		}
	}
	return false
}
