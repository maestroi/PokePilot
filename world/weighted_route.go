package world

import (
	"container/heap"

	gameruntime "github.com/maestroi/pokepilot/game"
)

// RouteCostPolicy prices a legal global route in shared travel-cost units.
// MoveCost prices one tile of local movement; MapTransitionCost accounts for
// the fixed overhead of crossing a warp/connection. Semantic actions can be
// priced either by transition id or by the capabilities they require.
//
// FallbackMapTransitionCost is used only when exact local geometry cannot be
// evaluated. It deliberately preserves the historical transition-count model
// instead of pretending an arbitrary tile is representative of a map-only
// goal.
type RouteCostPolicy struct {
	MoveCost                 int
	MapTransitionCost        int
	DefaultActionCost        int
	FallbackMapTransitionCost int
	AllowWater               bool
	TransitionCosts          map[string]int
	CapabilityActionCosts    map[gameruntime.CapabilityID]int
}

// DefaultRouteCostPolicy matches the coarse units already used by Red's local
// speedrun routing: movement is the base unit, while map/action overhead is
// finite so a slightly longer transition sequence can still beat a huge walk.
// The fallback transition cost intentionally retains the old fast-travel scale.
func DefaultRouteCostPolicy() RouteCostPolicy {
	return RouteCostPolicy{
		MoveCost:                  1,
		MapTransitionCost:         4,
		DefaultActionCost:         12,
		FallbackMapTransitionCost: 100,
	}
}

func (p RouteCostPolicy) normalized() RouteCostPolicy {
	if p.MoveCost <= 0 {
		p.MoveCost = 1
	}
	if p.MapTransitionCost <= 0 {
		p.MapTransitionCost = 4
	}
	if p.DefaultActionCost < 0 {
		p.DefaultActionCost = 0
	}
	if p.FallbackMapTransitionCost <= 0 {
		p.FallbackMapTransitionCost = 100
	}
	return p
}

func (p RouteCostPolicy) actionCost(t gameruntime.Transition) int {
	if t.Gate {
		return 0
	}
	if cost, ok := p.TransitionCosts[t.ID]; ok {
		return cost
	}
	cost, matched := 0, false
	for _, capability := range t.Requires {
		if c, ok := p.CapabilityActionCosts[capability]; ok {
			if !matched || c > cost {
				cost = c
			}
			matched = true
		}
	}
	if matched {
		return cost
	}
	return p.DefaultActionCost
}

// RouteCostResult is a capability-aware route plus its estimated travel cost.
// Exact is true when every local leg and final destination distance was priced
// from decoded collision geometry. Exact=false means Steps came from the
// established component-aware BFS and Cost is its conservative compatibility
// estimate.
type RouteCostResult struct {
	Steps []RouteStep
	Cost  int
	Exact bool
}

// FindWeightedRoutePlanAtDestinationWithCapabilities chooses among the same
// legal semantic/component-aware routes as
// FindRoutePlanAtDestinationWithCapabilities, but when decoded geometry is
// available it uses Dijkstra over concrete transition landings and prices the
// local walk to each exit.
//
// tx/ty < 0 means "arrive anywhere on the destination map". No canonical tile
// is invented for that case: reaching the map costs zero additional local
// distance. If exact local geometry cannot be computed, the function returns
// the established BFS route with Exact=false rather than failing a journey that
// the conservative router already knows is legal.
func FindWeightedRoutePlanAtDestinationWithCapabilities(
	g *Graph,
	from, to uint8,
	x, y, tx, ty int,
	blockedHere map[Edge]bool,
	prereqs RoutePrerequisites,
	policy RouteCostPolicy,
) (RouteCostResult, error) {
	if g == nil {
		return RouteCostResult{}, ErrNoRoute
	}
	policy = policy.normalized()

	fallback, err := FindRoutePlanAtDestinationWithCapabilities(
		g, from, to, x, y, tx, ty, blockedHere, prereqs,
	)
	if err != nil {
		return RouteCostResult{}, err
	}
	fallbackResult := RouteCostResult{
		Steps: fallback,
		Cost:  fallbackRouteCost(fallback, from, to, x, y, tx, ty, policy),
		Exact: false,
	}

	if exact, ok := findExactWeightedRoute(
		g, from, to, x, y, tx, ty, blockedHere, prereqs, policy,
	); ok {
		return exact, nil
	}
	return fallbackResult, nil
}

func fallbackRouteCost(steps []RouteStep, from, to uint8, x, y, tx, ty int, policy RouteCostPolicy) int {
	cost := len(steps) * policy.FallbackMapTransitionCost
	for _, step := range steps {
		if step.Transition != nil {
			cost += policy.actionCost(*step.Transition)
		}
	}
	if from == to && tx >= 0 && ty >= 0 && x >= 0 && y >= 0 {
		cost += (iabs(x-tx) + iabs(y-ty)) * policy.MoveCost
	}
	return cost
}

type weightedSemanticView struct {
	usable        *Graph
	executable    map[Edge]gameruntime.Transition
	skipCanExit   map[Edge]bool
	relaxLanding  map[Edge]bool
}

func buildWeightedSemanticView(g *Graph, prereqs RoutePrerequisites) weightedSemanticView {
	view := weightedSemanticView{
		usable:       g,
		executable:   make(map[Edge]gameruntime.Transition),
		skipCanExit:  make(map[Edge]bool),
		relaxLanding: make(map[Edge]bool),
	}
	if g == nil || len(prereqs.Transitions) == 0 {
		return view
	}

	hardDenied := make(map[Edge]gameruntime.TransitionBlockage)
	classify := func(edge Edge, transition gameruntime.Transition) {
		switch {
		case transition.Gate:
		case transition.PortBypass && transition.PivotOnly:
			view.skipCanExit[edge] = true
		case transition.PortBypass:
			view.skipCanExit[edge] = true
			view.relaxLanding[edge] = true
		case transition.PivotOnly:
			view.relaxLanding[edge] = true
		default:
			view.skipCanExit[edge] = true
			view.relaxLanding[edge] = true
		}
	}

	for edge, transition := range prereqs.Transitions {
		if _, ok := gameruntime.EvaluateTransition(transition, prereqs.Capabilities); !ok {
			if !transition.PivotOnly {
				hardDenied[edge] = gameruntime.TransitionBlockage{Transition: transition}
			}
			continue
		}
		view.executable[edge] = transition
		classify(edge, transition)
	}
	if len(hardDenied) > 0 {
		view.usable = graphWithoutSemanticEdges(g, hardDenied)
	}
	return view
}

type routeOccupancy [4]uint64

func (o routeOccupancy) has(mapID uint8) bool {
	slot, bit := int(mapID)/64, uint(mapID%64)
	return o[slot]&(uint64(1)<<bit) != 0
}

func (o routeOccupancy) with(mapID uint8) routeOccupancy {
	slot, bit := int(mapID)/64, uint(mapID%64)
	o[slot] |= uint64(1) << bit
	return o
}

type weightedRouteNode struct {
	mapID uint8
	x, y int
	known bool
	entry []int
	via   Edge

	occupied routeOccupancy
	cost     int
	prev     int
	step     RouteStep
}

type weightedRouteKey struct {
	mapID      uint8
	x, y       int
	known      bool
	components string
	via        Edge
	occupied   routeOccupancy
}

func weightedNodeKey(n weightedRouteNode) weightedRouteKey {
	return weightedRouteKey{
		mapID:      n.mapID,
		x:          n.x,
		y:          n.y,
		known:      n.known,
		components: componentSetKey(n.entry),
		via:        n.via,
		occupied:   n.occupied,
	}
}

type weightedQueueItem struct {
	node int
	cost int
	seq  int
}

type weightedRouteQueue []*weightedQueueItem

func (q weightedRouteQueue) Len() int { return len(q) }
func (q weightedRouteQueue) Less(i, j int) bool {
	if q[i].cost != q[j].cost {
		return q[i].cost < q[j].cost
	}
	return q[i].seq < q[j].seq
}
func (q weightedRouteQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *weightedRouteQueue) Push(v any)   { *q = append(*q, v.(*weightedQueueItem)) }
func (q *weightedRouteQueue) Pop() any {
	old := *q
	n := len(old)
	v := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return v
}

func findExactWeightedRoute(
	g *Graph,
	from, to uint8,
	x, y, tx, ty int,
	blockedHere map[Edge]bool,
	prereqs RoutePrerequisites,
	policy RouteCostPolicy,
) (RouteCostResult, bool) {
	if x < 0 || y < 0 {
		return RouteCostResult{}, false
	}
	view := buildWeightedSemanticView(g, prereqs)
	geometry := newRouteGeometry(g, policy.AllowWater)

	start := weightedRouteNode{
		mapID:    from,
		x:        x,
		y:        y,
		known:    true,
		entry:    componentSetAt(g, from, x, y),
		occupied: (routeOccupancy{}).with(from),
		prev:     -1,
	}
	nodes := []weightedRouteNode{start}
	best := map[weightedRouteKey]int{weightedNodeKey(start): 0}
	open := &weightedRouteQueue{}
	heap.Init(open)
	seq := 0
	heap.Push(open, &weightedQueueItem{node: 0, cost: 0, seq: seq})

	const maxWeightedRouteStates = 50000
	expanded := 0

	for open.Len() > 0 {
		item := heap.Pop(open).(*weightedQueueItem)
		cur := nodes[item.node]
		if best[weightedNodeKey(cur)] != item.cost {
			continue
		}

		if cur.mapID == to {
			finalDistance := 0
			if tx >= 0 && ty >= 0 {
				if !cur.known {
					goto expand
				}
				d, ok := geometry.distance(cur.mapID, cur.x, cur.y, tx, ty, false)
				if !ok {
					goto expand
				}
				finalDistance = d
			}
			return RouteCostResult{
				Steps: reconstructWeightedRoute(nodes, item.node),
				Cost:  cur.cost + finalDistance*policy.MoveCost,
				Exact: true,
			}, true
		}

	expand:
		expanded++
		if expanded > maxWeightedRouteStates {
			return RouteCostResult{}, false
		}

		for _, edge := range view.usable.Edges[cur.mapID] {
			if cur.prev < 0 && blockedHere[edge] {
				continue
			}
			if g.componentAware && len(g.exitComps[edge]) == 0 &&
				!(view.skipCanExit[edge] && view.relaxLanding[edge]) {
				continue
			}
			if !view.skipCanExit[edge] && !canExit(g, edge, cur.entry) {
				continue
			}
			if !cur.known {
				continue
			}

			port, localDistance, ok := geometry.bestPort(cur.mapID, cur.x, cur.y, edge)
			if !ok {
				continue
			}

			nextEntry := g.entryComps[edge]
			if view.relaxLanding[edge] && !cur.occupied.has(edge.To) {
				nextEntry = nil
			}

			next := weightedRouteNode{
				mapID:    edge.To,
				x:        port.entry.x,
				y:        port.entry.y,
				known:    port.entryKnown,
				entry:    nextEntry,
				via:      edge,
				occupied: cur.occupied.with(edge.To),
				prev:     item.node,
			}
			next.step.Edge = edge
			if transition, ok := view.executable[edge]; ok {
				t := transition
				next.step.Transition = &t
			}

			next.cost = cur.cost + localDistance*policy.MoveCost + policy.MapTransitionCost
			if next.step.Transition != nil {
				next.cost += policy.actionCost(*next.step.Transition)
			}

			key := weightedNodeKey(next)
			if old, seen := best[key]; seen && old <= next.cost {
				continue
			}
			best[key] = next.cost
			nodes = append(nodes, next)
			seq++
			heap.Push(open, &weightedQueueItem{node: len(nodes) - 1, cost: next.cost, seq: seq})
		}
	}
	return RouteCostResult{}, false
}

func reconstructWeightedRoute(nodes []weightedRouteNode, at int) []RouteStep {
	var reverse []RouteStep
	for at >= 0 && nodes[at].prev >= 0 {
		reverse = append(reverse, nodes[at].step)
		at = nodes[at].prev
	}
	out := make([]RouteStep, len(reverse))
	for i := range reverse {
		out[len(reverse)-1-i] = reverse[i]
	}
	return out
}

type routePoint struct {
	x, y int
}

type routePortCandidate struct {
	exit       routePoint
	entry      routePoint
	entryKnown bool
}

type routeGridKey struct {
	mapID uint8
	mode  TraversalMode
}

type routeDistanceKey struct {
	mapID          uint8
	sx, sy, dx, dy int
	adjacent       bool
}

type routeGeometry struct {
	g          *Graph
	allowWater bool
	grids      map[routeGridKey]*Grid
	gridMiss   map[routeGridKey]bool
	distances  map[routeDistanceKey]int
	distMiss   map[routeDistanceKey]bool
}

func newRouteGeometry(g *Graph, allowWater bool) *routeGeometry {
	return &routeGeometry{
		g:          g,
		allowWater: allowWater,
		grids:      make(map[routeGridKey]*Grid),
		gridMiss:   make(map[routeGridKey]bool),
		distances:  make(map[routeDistanceKey]int),
		distMiss:   make(map[routeDistanceKey]bool),
	}
}

func (r *routeGeometry) grid(mapID uint8, mode TraversalMode) (*Grid, bool) {
	key := routeGridKey{mapID: mapID, mode: mode}
	if grid, ok := r.grids[key]; ok {
		return grid, true
	}
	if r.gridMiss[key] || r.g == nil || r.g.provider == nil {
		return nil, false
	}
	spec, err := r.g.provider.Grid(mapID, nil, mode)
	if err != nil {
		r.gridMiss[key] = true
		return nil, false
	}
	grid, err := gridFromSpec(spec)
	if err != nil {
		r.gridMiss[key] = true
		return nil, false
	}
	r.grids[key] = grid
	return grid, true
}

func (r *routeGeometry) distance(mapID uint8, sx, sy, dx, dy int, adjacent bool) (int, bool) {
	key := routeDistanceKey{
		mapID: mapID, sx: sx, sy: sy, dx: dx, dy: dy, adjacent: adjacent,
	}
	if distance, ok := r.distances[key]; ok {
		return distance, true
	}
	if r.distMiss[key] {
		return 0, false
	}

	modes := []TraversalMode{TraversalLand}
	if r.allowWater {
		modes = append(modes, TraversalWater)
	}
	best, found := 0, false
	for _, mode := range modes {
		grid, ok := r.grid(mapID, mode)
		if !ok {
			continue
		}
		distance, ok := routeGridDistance(r.g, grid, sx, sy, dx, dy, adjacent)
		if !ok {
			continue
		}
		if !found || distance < best {
			best, found = distance, true
		}
	}
	if !found {
		r.distMiss[key] = true
		return 0, false
	}
	r.distances[key] = best
	return best, true
}

func routeGridDistance(g *Graph, grid *Grid, sx, sy, dx, dy int, adjacent bool) (int, bool) {
	if grid == nil || !grid.InBounds(sx, sy) || !grid.InBounds(dx, dy) {
		return 0, false
	}
	blocked := warpTileBlockers(g.warps[grid.MapID])
	delete(blocked, [2]int{sx, sy})
	delete(blocked, [2]int{dx, dy})

	if steps, err := FindPath(grid, sx, sy, dx, dy, blocked); err == nil {
		return routeStepDistance(steps), true
	}
	if !adjacent {
		return 0, false
	}
	steps, push, err := FindPathAdjacent(grid, sx, sy, dx, dy, blocked)
	if err != nil {
		return 0, false
	}
	return routeStepDistance(steps) + iabs(push.DX) + iabs(push.DY), true
}

func routeStepDistance(steps []Step) int {
	total := 0
	for _, step := range steps {
		total += iabs(step.DX) + iabs(step.DY)
	}
	return total
}

func (r *routeGeometry) bestPort(mapID uint8, sx, sy int, edge Edge) (routePortCandidate, int, bool) {
	ports := r.edgePorts(edge)
	var best routePortCandidate
	bestDistance, found := 0, false
	for _, port := range ports {
		distance, ok := r.distance(mapID, sx, sy, port.exit.x, port.exit.y, true)
		if !ok {
			continue
		}
		if !found || distance < bestDistance {
			best, bestDistance, found = port, distance, true
		}
	}
	return best, bestDistance, found
}

func (r *routeGeometry) edgePorts(edge Edge) []routePortCandidate {
	if r.g == nil {
		return nil
	}
	switch edge.Kind {
	case EdgeWarp:
		port := routePortCandidate{
			exit: routePoint{x: int(edge.WarpX), y: int(edge.WarpY)},
		}
		if x, y, ok := r.g.destWarpTile(edge); ok {
			port.entry = routePoint{x: x, y: y}
			port.entryKnown = true
		}
		return []routePortCandidate{port}

	case EdgeConnection:
		connection, ok := r.g.connections[edge]
		if !ok {
			return nil
		}
		src, srcOK := r.g.tiles[edge.From]
		dst, dstOK := r.g.tiles[edge.To]
		if !srcOK || !dstOK {
			return nil
		}
		n := src.w
		if edge.Dir >= dirWest {
			n = src.h
		}
		start, end := connectionBandRange(edge, n)
		if end < start {
			return nil
		}
		ports := make([]routePortCandidate, 0, end-start+1)
		for i := start; i <= end; i++ {
			sx, sy, tx, ty := r.g.connectionSeamTile(edge, connection, i)
			if sx < 0 || sy < 0 || sx >= src.w || sy >= src.h ||
				tx < 0 || ty < 0 || tx >= dst.w || ty >= dst.h {
				continue
			}
			ports = append(ports, routePortCandidate{
				exit:       routePoint{x: sx, y: sy},
				entry:      routePoint{x: tx, y: ty},
				entryKnown: true,
			})
		}
		return ports
	}
	return nil
}
