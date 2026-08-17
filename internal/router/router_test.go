package router

import (
	"reflect"
	"testing"
)

func TestSameDirectionRoutesShareLanes(t *testing.T) {
	source := Rect{X: 0, Y: 0, Width: 40, Height: 40}
	target := Rect{X: 240, Y: 0, Width: 40, Height: 40}
	result, err := Solve(Problem{
		Obstacles: []Rect{source, target},
		Requests: []Request{
			{ID: "edge-1", SourceID: "a", TargetID: "b", SourceRect: source, TargetRect: target},
			{ID: "edge-2", SourceID: "a", TargetID: "b", SourceRect: source, TargetRect: target},
		},
		Options: Options{GridSize: 20, Clearance: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Routes) != 2 || result.SharedLanes == 0 {
		t.Fatalf("expected shared lanes: %+v", result)
	}
	if !contains(result.Routes[0].SharedWith, "edge-2") || !contains(result.Routes[1].SharedWith, "edge-1") {
		t.Fatalf("shared route metadata is incomplete: %+v", result.Routes)
	}
	assertLaneAlignedAnchors(t, result.Routes[0], source, target)
}

func TestOppositeDirectionCannotReserveSameLane(t *testing.T) {
	source := Rect{X: 0, Y: 0, Width: 40, Height: 40}
	target := Rect{X: 240, Y: 0, Width: 40, Height: 40}
	first, err := Solve(Problem{
		Obstacles: []Rect{source, target},
		Requests:  []Request{{ID: "forward", SourceID: "a", TargetID: "b", SourceRect: source, TargetRect: target}},
		Options:   Options{GridSize: 20, Clearance: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := Solve(Problem{
		Obstacles: []Rect{source, target},
		Requests:  []Request{{ID: "reverse", SourceID: "b", TargetID: "a", SourceRect: target, TargetRect: source}},
		Reserved:  []ReservedRoute{{ID: "forward", LaneIDs: first.Routes[0].LaneIDs}},
		Options:   Options{GridSize: 20, Clearance: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	forwardLanes := laneDirections(first.Routes[0].LaneIDs, 20)
	for key, direction := range laneDirections(reverse.Routes[0].LaneIDs, 20) {
		if existing, ok := forwardLanes[key]; ok && existing != direction {
			t.Fatalf("opposite route reused lane %v in reverse direction", key)
		}
	}
}

func TestRouterAvoidsObstacleAndIsDeterministic(t *testing.T) {
	source := Rect{X: 0, Y: 80, Width: 40, Height: 40}
	target := Rect{X: 320, Y: 80, Width: 40, Height: 40}
	blocker := Rect{X: 140, Y: 40, Width: 80, Height: 120}
	problem := Problem{
		Obstacles: []Rect{source, target, blocker},
		Requests:  []Request{{ID: "around", SourceID: "a", TargetID: "b", SourceRect: source, TargetRect: target}},
		Options:   Options{GridSize: 20, Clearance: 20},
	}
	first, err := Solve(problem)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Solve(problem)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("router is not deterministic:\nfirst=%+v\nsecond=%+v", first, second)
	}
	inflatedBlocker := inflate(blocker, 20)
	points := first.Routes[0].Points
	for i := 1; i < len(points); i++ {
		if modelSegmentCrosses(points[i-1], points[i], inflatedBlocker) {
			t.Fatalf("route crosses obstacle: %+v", points)
		}
	}
}

func laneDirections(ids []string, gridSize float64) map[laneKey]int8 {
	result := make(map[laneKey]int8)
	for _, id := range ids {
		key, direction, ok := parseLaneID(id, gridSize)
		if ok {
			result[key] = direction
		}
	}
	return result
}

func modelSegmentCrosses(a, b Point, obstacle Rect) bool {
	if a.Y == b.Y && a.Y > obstacle.Y && a.Y < obstacle.Y+obstacle.Height {
		return min(a.X, b.X) < obstacle.X+obstacle.Width && max(a.X, b.X) > obstacle.X
	}
	if a.X == b.X && a.X > obstacle.X && a.X < obstacle.X+obstacle.Width {
		return min(a.Y, b.Y) < obstacle.Y+obstacle.Height && max(a.Y, b.Y) > obstacle.Y
	}
	return false
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func assertLaneAlignedAnchors(t *testing.T, route Route, source, target Rect) {
	t.Helper()
	if len(route.Points) < 2 {
		t.Fatalf("route has too few points: %+v", route)
	}
	first, last := route.Points[0], route.Points[len(route.Points)-1]
	if route.SourceSide == East || route.SourceSide == West {
		sourceAnchorY := source.Y + route.SourceAnchor.Y*source.Height
		if first.Y != sourceAnchorY {
			t.Fatalf("source anchor is not lane-aligned: %+v", route)
		}
	} else {
		sourceAnchorX := source.X + route.SourceAnchor.X*source.Width
		if first.X != sourceAnchorX {
			t.Fatalf("source anchor is not lane-aligned: %+v", route)
		}
	}
	if route.TargetSide == East || route.TargetSide == West {
		targetAnchorY := target.Y + route.TargetAnchor.Y*target.Height
		if last.Y != targetAnchorY {
			t.Fatalf("target anchor is not lane-aligned: %+v", route)
		}
	} else {
		targetAnchorX := target.X + route.TargetAnchor.X*target.Width
		if last.X != targetAnchorX {
			t.Fatalf("target anchor is not lane-aligned: %+v", route)
		}
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
