// Package router implements deterministic orthogonal routing on directed,
// shareable grid lanes.
package router

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Rect struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

type Side string

const (
	North Side = "north"
	East  Side = "east"
	South Side = "south"
	West  Side = "west"
)

type Request struct {
	ID         string
	SourceID   string
	TargetID   string
	SourceRect Rect
	TargetRect Rect
}

type ReservedRoute struct {
	ID      string
	LaneIDs []string
}

type Options struct {
	GridSize       float64
	Clearance      float64
	BendPenalty    int
	NewLaneCost    int
	SharedLaneCost int
	MaxSearchNodes int
}

type Problem struct {
	Obstacles []Rect
	Requests  []Request
	Reserved  []ReservedRoute
	Options   Options
}

type Route struct {
	ID           string   `json:"id"`
	SourceID     string   `json:"source_id"`
	TargetID     string   `json:"target_id"`
	SourceSide   Side     `json:"source_side"`
	TargetSide   Side     `json:"target_side"`
	SourceAnchor Point    `json:"source_anchor"`
	TargetAnchor Point    `json:"target_anchor"`
	Points       []Point  `json:"points"`
	LaneIDs      []string `json:"lanes"`
	SharedWith   []string `json:"shared_with,omitempty"`
	Cost         int      `json:"cost"`
}

type Result struct {
	Routes      []Route
	SharedLanes int
}

type gridPoint struct {
	X int
	Y int
}

type direction int8

const (
	dirNone direction = iota
	dirNorth
	dirEast
	dirSouth
	dirWest
)

type laneKey struct {
	A gridPoint
	B gridPoint
}

type laneUsage struct {
	direction int8
	edgeIDs   []string
}

type occupancy struct {
	lanes map[laneKey]*laneUsage
}

type normalizedOptions struct {
	gridSize       float64
	clearance      float64
	bendPenalty    int
	newLaneCost    int
	sharedLaneCost int
	maxSearchNodes int
}

type grid struct {
	options   normalizedOptions
	obstacles []Rect
	minX      int
	maxX      int
	minY      int
	maxY      int
}

type port struct {
	point gridPoint
	side  Side
	dir   direction
}

type candidate struct {
	points     []gridPoint
	sourceSide Side
	targetSide Side
	cost       int
}

// Solve routes every request atomically. It returns no partial result.
func Solve(problem Problem) (Result, error) {
	options, err := normalizeOptions(problem.Options)
	if err != nil {
		return Result{}, err
	}
	if len(problem.Requests) == 0 {
		return Result{}, fmt.Errorf("at least one routing request is required")
	}
	ids := make(map[string]bool, len(problem.Requests))
	for _, request := range problem.Requests {
		if request.ID == "" || request.SourceID == "" || request.TargetID == "" {
			return Result{}, fmt.Errorf("route ID, source ID, and target ID are required")
		}
		if request.SourceID == request.TargetID {
			return Result{}, fmt.Errorf("self-loop routing is not supported for %q", request.ID)
		}
		if ids[request.ID] {
			return Result{}, fmt.Errorf("duplicate route ID %q", request.ID)
		}
		ids[request.ID] = true
	}

	routingGrid := newGrid(problem.Obstacles, problem.Requests, options)
	usage := &occupancy{lanes: make(map[laneKey]*laneUsage)}
	for _, reserved := range problem.Reserved {
		for _, laneID := range reserved.LaneIDs {
			key, orientation, ok := parseLaneID(laneID, options.gridSize)
			if !ok {
				continue
			}
			if existing := usage.lanes[key]; existing != nil && existing.direction != orientation {
				return Result{}, fmt.Errorf("reserved lane %q has conflicting directions", laneID)
			}
			entry := usage.lanes[key]
			if entry == nil {
				entry = &laneUsage{direction: orientation}
				usage.lanes[key] = entry
			}
			entry.edgeIDs = appendUnique(entry.edgeIDs, reserved.ID)
		}
	}

	requests := append([]Request(nil), problem.Requests...)
	sort.SliceStable(requests, func(i, j int) bool {
		iDistance := rectDistance(requests[i].SourceRect, requests[i].TargetRect)
		jDistance := rectDistance(requests[j].SourceRect, requests[j].TargetRect)
		if iDistance != jDistance {
			return iDistance > jDistance
		}
		return false
	})

	routes := make([]Route, 0, len(requests))
	newRouteLanes := make(map[laneKey]bool)
	for _, request := range requests {
		best, err := routingGrid.route(request, usage)
		if err != nil {
			return Result{}, fmt.Errorf("route %q (%s -> %s): %w", request.ID, request.SourceID, request.TargetID, err)
		}
		modelPoints := gridPointsToModel(compressPoints(best.points), options.gridSize)
		route := Route{
			ID: request.ID, SourceID: request.SourceID, TargetID: request.TargetID,
			SourceSide: best.sourceSide, TargetSide: best.targetSide,
			SourceAnchor: anchorForPort(modelPoints[0], request.SourceRect, best.sourceSide),
			TargetAnchor: anchorForPort(modelPoints[len(modelPoints)-1], request.TargetRect, best.targetSide),
			Points:       modelPoints, Cost: best.cost,
		}
		for i := 1; i < len(best.points); i++ {
			key, orientation := makeLane(best.points[i-1], best.points[i])
			laneID := formatLaneID(key, orientation, options.gridSize)
			route.LaneIDs = append(route.LaneIDs, laneID)
			newRouteLanes[key] = true
			entry := usage.lanes[key]
			if entry == nil {
				entry = &laneUsage{direction: orientation}
				usage.lanes[key] = entry
			}
			entry.edgeIDs = appendUnique(entry.edgeIDs, request.ID)
		}
		routes = append(routes, route)
	}

	sharedLaneKeys := make(map[laneKey]bool)
	byID := make(map[string]*Route, len(routes))
	for i := range routes {
		byID[routes[i].ID] = &routes[i]
	}
	for key, entry := range usage.lanes {
		if len(entry.edgeIDs) < 2 || !newRouteLanes[key] {
			continue
		}
		sharedLaneKeys[key] = true
		for _, edgeID := range entry.edgeIDs {
			route := byID[edgeID]
			if route == nil {
				continue
			}
			for _, other := range entry.edgeIDs {
				if other != edgeID {
					route.SharedWith = appendUnique(route.SharedWith, other)
				}
			}
		}
	}
	for i := range routes {
		sort.Strings(routes[i].SharedWith)
	}
	// Restore caller request order for deterministic API output.
	order := make(map[string]int, len(problem.Requests))
	for i, request := range problem.Requests {
		order[request.ID] = i
	}
	sort.SliceStable(routes, func(i, j int) bool { return order[routes[i].ID] < order[routes[j].ID] })
	return Result{Routes: routes, SharedLanes: len(sharedLaneKeys)}, nil
}

func normalizeOptions(input Options) (normalizedOptions, error) {
	result := normalizedOptions{
		gridSize: input.GridSize, clearance: input.Clearance, bendPenalty: input.BendPenalty,
		newLaneCost: input.NewLaneCost, sharedLaneCost: input.SharedLaneCost, maxSearchNodes: input.MaxSearchNodes,
	}
	if result.gridSize == 0 {
		result.gridSize = 20
	}
	if result.clearance == 0 {
		result.clearance = result.gridSize
	}
	if result.bendPenalty == 0 {
		result.bendPenalty = 20
	}
	if result.newLaneCost == 0 {
		result.newLaneCost = 10
	}
	if result.sharedLaneCost == 0 {
		result.sharedLaneCost = 3
	}
	if result.maxSearchNodes == 0 {
		result.maxSearchNodes = 150000
	}
	if !finite(result.gridSize) || result.gridSize <= 0 {
		return normalizedOptions{}, fmt.Errorf("grid_size must be a positive finite number")
	}
	if !finite(result.clearance) || result.clearance < 0 {
		return normalizedOptions{}, fmt.Errorf("clearance must be a non-negative finite number")
	}
	if result.bendPenalty < 0 || result.newLaneCost <= 0 || result.sharedLaneCost <= 0 {
		return normalizedOptions{}, fmt.Errorf("routing costs must be positive, except bend penalty may be zero")
	}
	if result.sharedLaneCost > result.newLaneCost {
		return normalizedOptions{}, fmt.Errorf("shared_lane_cost must not exceed new_lane_cost")
	}
	if result.maxSearchNodes < 100 {
		return normalizedOptions{}, fmt.Errorf("max_search_nodes must be at least 100")
	}
	return result, nil
}

func newGrid(obstacles []Rect, requests []Request, options normalizedOptions) *grid {
	expanded := make([]Rect, 0, len(obstacles))
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, obstacle := range obstacles {
		if obstacle.Width <= 0 || obstacle.Height <= 0 {
			continue
		}
		rect := inflate(obstacle, options.clearance)
		expanded = append(expanded, rect)
		minX, minY = math.Min(minX, rect.X), math.Min(minY, rect.Y)
		maxX, maxY = math.Max(maxX, rect.X+rect.Width), math.Max(maxY, rect.Y+rect.Height)
	}
	for _, request := range requests {
		for _, rect := range []Rect{request.SourceRect, request.TargetRect} {
			minX, minY = math.Min(minX, rect.X), math.Min(minY, rect.Y)
			maxX, maxY = math.Max(maxX, rect.X+rect.Width), math.Max(maxY, rect.Y+rect.Height)
		}
	}
	if math.IsInf(minX, 1) {
		minX, minY, maxX, maxY = 0, 0, options.gridSize*10, options.gridSize*10
	}
	margin := math.Max(options.gridSize*8, options.clearance*2+options.gridSize*2)
	return &grid{
		options: options, obstacles: expanded,
		minX: int(math.Floor((minX - margin) / options.gridSize)),
		maxX: int(math.Ceil((maxX + margin) / options.gridSize)),
		minY: int(math.Floor((minY - margin) / options.gridSize)),
		maxY: int(math.Ceil((maxY + margin) / options.gridSize)),
	}
}

func (g *grid) route(request Request, usage *occupancy) (candidate, error) {
	sourcePorts := g.ports(request.SourceRect)
	targetPorts := g.ports(request.TargetRect)
	best := candidate{cost: math.MaxInt}
	for _, source := range sourcePorts {
		if g.blockedPoint(source.point) {
			continue
		}
		for _, target := range targetPorts {
			if g.blockedPoint(target.point) || source.point == target.point {
				continue
			}
			points, cost, finalDirection, ok := g.aStar(source, target, usage)
			if !ok {
				continue
			}
			if finalDirection != opposite(target.dir) {
				cost += g.options.bendPenalty
			}
			preference := portPreference(request.SourceRect, request.TargetRect, source.side, target.side)
			cost += preference
			if cost < best.cost || (cost == best.cost && candidateLess(source.side, target.side, best)) {
				best = candidate{points: points, sourceSide: source.side, targetSide: target.side, cost: cost}
			}
		}
	}
	if len(best.points) == 0 {
		return candidate{}, fmt.Errorf("no route found within the search grid")
	}
	return best, nil
}

func (g *grid) ports(rect Rect) []port {
	gridSize := g.options.gridSize
	centerX := rect.X + rect.Width/2
	centerY := rect.Y + rect.Height/2
	return []port{
		{point: gridPoint{X: snapNearest(centerX, gridSize), Y: snapFloor(rect.Y-g.options.clearance, gridSize)}, side: North, dir: dirNorth},
		{point: gridPoint{X: snapCeil(rect.X+rect.Width+g.options.clearance, gridSize), Y: snapNearest(centerY, gridSize)}, side: East, dir: dirEast},
		{point: gridPoint{X: snapNearest(centerX, gridSize), Y: snapCeil(rect.Y+rect.Height+g.options.clearance, gridSize)}, side: South, dir: dirSouth},
		{point: gridPoint{X: snapFloor(rect.X-g.options.clearance, gridSize), Y: snapNearest(centerY, gridSize)}, side: West, dir: dirWest},
	}
}

type searchState struct {
	point gridPoint
	dir   direction
}

type queueItem struct {
	state searchState
	g     int
	f     int
	seq   int
	index int
}

type priorityQueue []*queueItem

func (q priorityQueue) Len() int { return len(q) }
func (q priorityQueue) Less(i, j int) bool {
	if q[i].f != q[j].f {
		return q[i].f < q[j].f
	}
	if q[i].g != q[j].g {
		return q[i].g < q[j].g
	}
	return q[i].seq < q[j].seq
}
func (q priorityQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i]; q[i].index, q[j].index = i, j }
func (q *priorityQueue) Push(value any) {
	item := value.(*queueItem)
	item.index = len(*q)
	*q = append(*q, item)
}
func (q *priorityQueue) Pop() any {
	old := *q
	item := old[len(old)-1]
	*q = old[:len(old)-1]
	return item
}

func (g *grid) aStar(source, target port, usage *occupancy) ([]gridPoint, int, direction, bool) {
	start := searchState{point: source.point, dir: source.dir}
	queue := &priorityQueue{}
	heap.Init(queue)
	sequence := 0
	heap.Push(queue, &queueItem{state: start, g: 0, f: heuristic(source.point, target.point, g.options.sharedLaneCost), seq: sequence})
	distance := map[searchState]int{start: 0}
	previous := make(map[searchState]searchState)
	expanded := 0
	directions := []struct {
		dx, dy int
		dir    direction
	}{{0, -1, dirNorth}, {1, 0, dirEast}, {0, 1, dirSouth}, {-1, 0, dirWest}}

	for queue.Len() > 0 && expanded < g.options.maxSearchNodes {
		item := heap.Pop(queue).(*queueItem)
		if current, ok := distance[item.state]; !ok || current != item.g {
			continue
		}
		expanded++
		if item.state.point == target.point {
			return reconstruct(previous, item.state), item.g, item.state.dir, true
		}
		for _, move := range directions {
			nextPoint := gridPoint{X: item.state.point.X + move.dx, Y: item.state.point.Y + move.dy}
			if !g.inBounds(nextPoint) || (nextPoint != target.point && g.blockedPoint(nextPoint)) || g.blockedSegment(item.state.point, nextPoint) {
				continue
			}
			key, orientation := makeLane(item.state.point, nextPoint)
			stepCost := g.options.newLaneCost
			if occupied := usage.lanes[key]; occupied != nil {
				if occupied.direction != orientation {
					continue
				}
				stepCost = g.options.sharedLaneCost
			}
			if item.state.dir != dirNone && item.state.dir != move.dir {
				stepCost += g.options.bendPenalty
			}
			next := searchState{point: nextPoint, dir: move.dir}
			newDistance := item.g + stepCost
			if old, ok := distance[next]; ok && old <= newDistance {
				continue
			}
			distance[next] = newDistance
			previous[next] = item.state
			sequence++
			heap.Push(queue, &queueItem{
				state: next, g: newDistance,
				f: newDistance + heuristic(nextPoint, target.point, g.options.sharedLaneCost), seq: sequence,
			})
		}
	}
	return nil, 0, dirNone, false
}

func (g *grid) blockedPoint(point gridPoint) bool {
	x, y := float64(point.X)*g.options.gridSize, float64(point.Y)*g.options.gridSize
	for _, obstacle := range g.obstacles {
		if x > obstacle.X && x < obstacle.X+obstacle.Width && y > obstacle.Y && y < obstacle.Y+obstacle.Height {
			return true
		}
	}
	return false
}

func (g *grid) blockedSegment(a, b gridPoint) bool {
	ax, ay := float64(a.X)*g.options.gridSize, float64(a.Y)*g.options.gridSize
	bx, by := float64(b.X)*g.options.gridSize, float64(b.Y)*g.options.gridSize
	for _, obstacle := range g.obstacles {
		if ay == by && ay > obstacle.Y && ay < obstacle.Y+obstacle.Height {
			left, right := math.Min(ax, bx), math.Max(ax, bx)
			if left < obstacle.X+obstacle.Width && right > obstacle.X {
				return true
			}
		}
		if ax == bx && ax > obstacle.X && ax < obstacle.X+obstacle.Width {
			top, bottom := math.Min(ay, by), math.Max(ay, by)
			if top < obstacle.Y+obstacle.Height && bottom > obstacle.Y {
				return true
			}
		}
	}
	return false
}

func (g *grid) inBounds(point gridPoint) bool {
	return point.X >= g.minX && point.X <= g.maxX && point.Y >= g.minY && point.Y <= g.maxY
}

func reconstruct(previous map[searchState]searchState, state searchState) []gridPoint {
	points := []gridPoint{state.point}
	for {
		parent, ok := previous[state]
		if !ok {
			break
		}
		state = parent
		points = append(points, state.point)
	}
	for left, right := 0, len(points)-1; left < right; left, right = left+1, right-1 {
		points[left], points[right] = points[right], points[left]
	}
	return points
}

func compressPoints(points []gridPoint) []gridPoint {
	if len(points) < 3 {
		return append([]gridPoint(nil), points...)
	}
	result := []gridPoint{points[0]}
	for i := 1; i < len(points)-1; i++ {
		before, current, after := points[i-1], points[i], points[i+1]
		if (before.X == current.X && current.X == after.X) || (before.Y == current.Y && current.Y == after.Y) {
			continue
		}
		result = append(result, current)
	}
	return append(result, points[len(points)-1])
}

func makeLane(from, to gridPoint) (laneKey, int8) {
	if pointLess(to, from) {
		return laneKey{A: to, B: from}, -1
	}
	return laneKey{A: from, B: to}, 1
}

func pointLess(a, b gridPoint) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	return a.Y < b.Y
}

func formatLaneID(key laneKey, orientation int8, gridSize float64) string {
	direction := "+"
	if orientation < 0 {
		direction = "-"
	}
	if key.A.Y == key.B.Y {
		return fmt.Sprintf("H:%s:%s:%s:%s", number(float64(key.A.Y)*gridSize), number(float64(key.A.X)*gridSize), number(float64(key.B.X)*gridSize), direction)
	}
	return fmt.Sprintf("V:%s:%s:%s:%s", number(float64(key.A.X)*gridSize), number(float64(key.A.Y)*gridSize), number(float64(key.B.Y)*gridSize), direction)
}

func parseLaneID(value string, gridSize float64) (laneKey, int8, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 5 || (parts[0] != "H" && parts[0] != "V") {
		return laneKey{}, 0, false
	}
	a, errA := strconv.ParseFloat(parts[1], 64)
	b, errB := strconv.ParseFloat(parts[2], 64)
	c, errC := strconv.ParseFloat(parts[3], 64)
	if errA != nil || errB != nil || errC != nil {
		return laneKey{}, 0, false
	}
	orientation := int8(1)
	if parts[4] == "-" {
		orientation = -1
	} else if parts[4] != "+" {
		return laneKey{}, 0, false
	}
	var from, to gridPoint
	if parts[0] == "H" {
		from, to = gridPoint{X: snapNearest(b, gridSize), Y: snapNearest(a, gridSize)}, gridPoint{X: snapNearest(c, gridSize), Y: snapNearest(a, gridSize)}
	} else {
		from, to = gridPoint{X: snapNearest(a, gridSize), Y: snapNearest(b, gridSize)}, gridPoint{X: snapNearest(a, gridSize), Y: snapNearest(c, gridSize)}
	}
	key, normalized := makeLane(from, to)
	return key, orientation * normalized, true
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func gridPointsToModel(points []gridPoint, gridSize float64) []Point {
	result := make([]Point, len(points))
	for i, point := range points {
		result[i] = Point{X: float64(point.X) * gridSize, Y: float64(point.Y) * gridSize}
	}
	return result
}

// anchorForPort returns a normalized mxGraph connection point on the selected
// shape side, aligned with the lane instead of forced to the side center.
func anchorForPort(port Point, rect Rect, side Side) Point {
	switch side {
	case East:
		return Point{X: 1, Y: clampUnit((port.Y - rect.Y) / rect.Height)}
	case West:
		return Point{X: 0, Y: clampUnit((port.Y - rect.Y) / rect.Height)}
	case North:
		return Point{X: clampUnit((port.X - rect.X) / rect.Width), Y: 0}
	case South:
		return Point{X: clampUnit((port.X - rect.X) / rect.Width), Y: 1}
	default:
		return Point{X: 0.5, Y: 0.5}
	}
}

func clampUnit(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func inflate(rect Rect, amount float64) Rect {
	return Rect{X: rect.X - amount, Y: rect.Y - amount, Width: rect.Width + amount*2, Height: rect.Height + amount*2}
}

func rectDistance(a, b Rect) float64 {
	return math.Abs((a.X+a.Width/2)-(b.X+b.Width/2)) + math.Abs((a.Y+a.Height/2)-(b.Y+b.Height/2))
}

func portPreference(source, target Rect, sourceSide, targetSide Side) int {
	dx := (target.X + target.Width/2) - (source.X + source.Width/2)
	dy := (target.Y + target.Height/2) - (source.Y + source.Height/2)
	preferredSource, preferredTarget := East, West
	if math.Abs(dy) > math.Abs(dx) {
		if dy >= 0 {
			preferredSource, preferredTarget = South, North
		} else {
			preferredSource, preferredTarget = North, South
		}
	} else if dx < 0 {
		preferredSource, preferredTarget = West, East
	}
	penalty := 0
	if sourceSide != preferredSource {
		penalty += 5
	}
	if targetSide != preferredTarget {
		penalty += 5
	}
	return penalty
}

func candidateLess(source, target Side, best candidate) bool {
	value := string(source) + ":" + string(target)
	bestValue := string(best.sourceSide) + ":" + string(best.targetSide)
	return value < bestValue
}

func heuristic(a, b gridPoint, stepCost int) int {
	return (absInt(a.X-b.X) + absInt(a.Y-b.Y)) * stepCost
}

func opposite(value direction) direction {
	switch value {
	case dirNorth:
		return dirSouth
	case dirEast:
		return dirWest
	case dirSouth:
		return dirNorth
	case dirWest:
		return dirEast
	default:
		return dirNone
	}
}

func snapNearest(value, gridSize float64) int { return int(math.Round(value / gridSize)) }
func snapFloor(value, gridSize float64) int   { return int(math.Floor(value / gridSize)) }
func snapCeil(value, gridSize float64) int    { return int(math.Ceil(value / gridSize)) }

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func number(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }
func finite(value float64) bool   { return !math.IsNaN(value) && !math.IsInf(value, 0) }
