package world

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

// ErrNoRoute reports that no sequence of warps/connections links the maps.
var ErrNoRoute = errors.New("world: no route")

// FindRoute returns the edges to traverse, in order, to get from map
// `from` to map `to`. It returns an empty slice when from == to.
//
// It is a breadth-first search over Graph.Edges, so the returned route has
// the fewest map transitions. Edges of a map are explored in slice order,
// so the same call always returns the same route. Tile-level pathfinding is
// not done here; that happens per leg at execution time.
func FindRoute(g *Graph, from, to uint8) ([]Edge, error) {
	return FindRouteAvoiding(g, from, to, nil)
}

// FindRouteAvoiding is FindRoute with the legs the caller has discovered it
// cannot take FROM WHERE IT STANDS RIGHT NOW excluded from the first hop.
//
// It exists because the map graph knows which maps TOUCH, not which are
// walkable between: Route 2 (0x0D) connects Viridian to Pewter in one hop,
// but a ledge splits it across its width into two bands, and the walk that
// hop implies is impossible from the southern one. Only the tile-level
// pathfinder discovers that, and only at execution time, so a caller that
// hits such a leg reports it here and asks again.
//
// TWO PROPERTIES, both measured the hard way:
//
// `blockedHere` applies ONLY to the first hop. Unwalkability is never a
// property of an edge alone — the Route 2 -> Pewter connection is
// impossible from the south band and perfectly walkable from the north one.
// Excluding it everywhere makes the only real route to Pewter unplannable,
// because that route ENDS on it. A caller re-plans from where it failed, so
// the first hop is the only place its report can honestly apply.
//
// When component data is available, the search state is the map PLUS the
// walkable component the previous transition landed in. This is the semantic
// state routing actually cares about: leaving a plaza through a building and
// returning to the same component changed nothing, so it cannot be used to
// clear a first-hop preference; leaving Route 2 through Viridian Forest and
// re-entering its other component IS a new state and remains legal. If
// component data is unavailable, the search falls back to the old edge-keyed
// identity rather than inventing geometry.
//
// Edge is comparable, so the caller's set is a plain map[Edge]bool.
func FindRouteAvoiding(g *Graph, from, to uint8, blockedHere map[Edge]bool) ([]Edge, error) {
	return findRoute(g, from, to, blockedHere, nil, nil, nil, nil)
}

// FindRouteAt is FindRouteAvoiding with the player's position on `from` known:
// the first hop is constrained to exits reachable from (x, y), i.e. in the
// same walkable component. Use it when the start map has disconnected
// components (Route 2, the gate maps) and the caller knows which one it stands
// in; the component the player is in is the only honest first-hop constraint.
func FindRouteAt(g *Graph, from, to uint8, x, y int, blockedHere map[Edge]bool) ([]Edge, error) {
	return findRoute(g, from, to, blockedHere, componentSetAt(g, from, x, y), nil, nil, nil)
}

// FindRouteAtDestination is FindRouteAt with the destination tile known too.
// On component-aware graphs, reaching the destination MAP is not sufficient:
// the incoming edge must land in the walkable component that contains (tx,ty).
// If from == to but the player and target are in different components, this
// deliberately searches a cycle that leaves and re-enters the map through a
// component that can actually reach the target.
func FindRouteAtDestination(g *Graph, from, to uint8, x, y, tx, ty int, blockedHere map[Edge]bool) ([]Edge, error) {
	return findRouteAtDestinationAllowingSemantic(g, from, to, x, y, tx, ty, blockedHere, nil, nil)
}

// findRouteAtDestinationAllowingSemantic is the component-aware planner with
// two semantic privileges. skipCanExit edges may be taken even when ordinary
// walking cannot reach their exit port (PortBypass / FROM-side actions such as
// Surf or a Cut tree on this map). relaxLanding edges discard the static
// destination landing component after the hop, so a TO-side pivot (Route 9's
// tree) can continue past a split that pristine ROM collision still sees.
//
// PivotOnly annotations belong in relaxLanding only: their obstacle lives on
// the adjacent map, so inventing FROM-side port reachability strands players
// in dead pockets (Cerulean Badge House north exit) that planned "east to
// Route 9" with Cut while standing on an unreachable component.
func findRouteAtDestinationAllowingSemantic(g *Graph, from, to uint8, x, y, tx, ty int, blockedHere map[Edge]bool, skipCanExit, relaxLanding map[Edge]bool) ([]Edge, error) {
	first := componentSetAt(g, from, x, y)
	target := standingComponentAt(g, to, tx, ty)
	if !g.componentAware || len(target) == 0 {
		// Unknown destination tile: map-level arrival is enough. A missing
		// START component must not take this branch — that is not evidence
		// the requested dest tile is unreachable, and dropping target made
		// every landing on `to` look like success (Cerulean (19,28) is
		// unwalkable in the ROM grid, so "go to Route 4 (10,10)" accepted
		// the east-seam landing).
		return findRoute(g, from, to, blockedHere, first, nil, skipCanExit, relaxLanding)
	}
	return findRoute(g, from, to, blockedHere, first, target, skipCanExit, relaxLanding)
}

func componentSetAt(g *Graph, mapID uint8, x, y int) []int {
	return g.expandComponents(mapID, standingComponentAt(g, mapID, x, y))
}

// EdgeEntrySharesComponentWith reports whether crossing e lands in the same
// walkable component as (x,y) on e.To. known is false when the graph has no
// component evidence for either side; callers should preserve their previous
// conservative behavior in that case rather than inventing topology.
//
// This is intentionally narrower than exposing component ids. A component id
// is graph-internal bookkeeping; callers such as GoTo only need to distinguish
// "return to territory already visited" from "same map, new walking region".
func (g *Graph) EdgeEntrySharesComponentWith(e Edge, x, y int) (same, known bool) {
	if g == nil || !g.componentAware {
		return false, false
	}
	entry := g.entryComps[e]
	at := componentSetAt(g, e.To, x, y)
	if len(entry) == 0 || len(at) == 0 {
		return false, false
	}
	return shareComp(entry, at), true
}

// standingComponentAt reports the walkable component(s) at (x,y) on mapID.
// A tile the player is physically standing on is never itself a warp tile's
// component: componentsWithBlocked excludes warp tiles from the flood so a
// teleporter can't falsely bridge two rooms it connects. But route search
// treats an empty result as "unknown, don't filter" (canExit), so standing
// exactly on a warp tile — a gym's exit door, a stair, a checkpoint that
// resumed mid-warp — used to silently disable first-hop reachability
// filtering and let the router offer every same-map warp edge as if it were
// walkable from here, however far behind a wall its pad actually sits
// (Saffron Gym's warp maze, standing on the exit door at (8,17)). Graph-build
// time already solves this for a warp's own port component via
// tileOrNeighbourComps; apply the same neighbor fallback here so a live
// position gets the same answer a statically-known warp tile would.
// isWarpTile reports whether (x,y) is a warp source tile on mapID, the only
// reason componentsWithBlocked would leave a walkable tile at component 0.
func isWarpTile(g *Graph, mapID uint8, x, y int) bool {
	for _, w := range g.warps[mapID] {
		if int(w.X) == x && int(w.Y) == y {
			return true
		}
	}
	return false
}

func standingComponentAt(g *Graph, mapID uint8, x, y int) []int {
	if !g.componentAware {
		return nil
	}
	c := g.comps[mapID]
	if c == nil || y < 0 || y >= len(c) || x < 0 || x >= len(c[y]) {
		return nil
	}
	if v := c[y][x]; v != 0 {
		return []int{v}
	}
	if !isWarpTile(g, mapID, x, y) {
		// Zero here means genuinely unwalkable (a wall, padding on a
		// connection border) rather than an excluded warp tile: stay
		// "unknown" rather than borrowing a neighbor's component, or a
		// padding tile would look like part of the walkable room beside it.
		return nil
	}
	var out []int
	seen := map[int]bool{}
	for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		nx, ny := x+d[0], y+d[1]
		if ny < 0 || ny >= len(c) || nx < 0 || nx >= len(c[ny]) {
			continue
		}
		if v := c[ny][nx]; v != 0 && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

type routeStateKey struct {
	mapID      uint8
	components string
	via        Edge
	byEdge     bool
}

// routeStateIdentity names the state reached after crossing via. A known
// component set is stronger than the transition that got there: two different
// building cycles that both land on the same plaza component are the same
// routing state. When the graph has no component evidence for the landing, use
// via as the conservative fallback and preserve the previous edge-keyed search.
func routeStateIdentity(g *Graph, mapID uint8, entry []int, via Edge) routeStateKey {
	if g.componentAware && len(entry) > 0 {
		return routeStateKey{mapID: mapID, components: componentSetKey(entry)}
	}
	return routeStateKey{mapID: mapID, via: via, byEdge: true}
}

func componentSetKey(in []int) string {
	if len(in) == 0 {
		return ""
	}
	v := append([]int(nil), in...)
	sort.Ints(v)
	var b strings.Builder
	for i, c := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(c))
	}
	return b.String()
}

func findRoute(g *Graph, from, to uint8, blockedHere map[Edge]bool, first, target []int, skipCanExit, relaxLanding map[Edge]bool) ([]Edge, error) {
	if from == to && (len(target) == 0 || shareComp(first, target)) {
		return []Edge{}, nil
	}
	// node.prev indexes back into nodes, or -1 for a first hop. entry is the
	// walkable component set on edge.To after taking edge (nil after a
	// relaxLanding hop, so later canExit on that map may use every port).
	type node struct {
		edge  Edge
		prev  int
		entry []int
	}
	var nodes []node
	seen := make(map[routeStateKey]bool)
	// A PivotOnly hop may unlock exits on a map the search has not stood on
	// yet. Re-entering a map already occupied in this search must use the
	// physical landing: otherwise leave-and-return becomes a teleport onto
	// every component of the origin (Cerulean -> Route 9 -> Cerulean).
	occupied := map[uint8]bool{from: true}
	if g.componentAware && len(first) > 0 {
		seen[routeStateIdentity(g, from, first, Edge{})] = true
	}
	expand := func(cur uint8, prev int, entry []int) {
		for _, e := range g.Edges[cur] {
			if prev < 0 && blockedHere[e] {
				continue
			}
			// PortBypass / FROM-side actions may be selected even when ordinary
			// walking cannot reach the exit port. PivotOnly destinations still
			// require canExit: their obstacle is on the adjacent map.
			//
			// PortBypass must never promote a phantom connection band — one
			// whose exit port has no walkable tile — into a real hop. Those
			// bands still receive the same transition annotation as their
			// walkable siblings (Route 9's east seam is split into six bands,
			// five of them empty), and skipping canExit for them used to land
			// with a nil component set. A nil landing is "unconstrained," so
			// the next hop could invent a far-side exit on the destination map
			// (Route 10 south to Lavender without Rock Tunnel). MEASURED on
			// farm run-2ccw7p3rpnvu4129l1dkhc7ayh: with can_cut, FindRoutePlan
			// returned Route9 -cut-> Route10 -south-> Lavender, then GoTo
			// bounced through Rock Tunnel until navigation_stalled.
			//
			// Surf is the exception: its PortBypass privilege is
			// skipCanExit+relaxLanding (not PivotOnly), and water shores have
			// empty land exitComps by construction. Rejecting them made every
			// southern-sea / Route 21 Surf approach unroutable
			// (triage:ff3ad54bb21d3a2d). PivotOnly+PortBypass Cut phantoms only
			// set skipCanExit and stay rejected here.
			if g.componentAware && len(g.exitComps[e]) == 0 && !(skipCanExit[e] && relaxLanding[e]) {
				continue
			}
			if !skipCanExit[e] && !canExit(g, e, entry) {
				continue
			}
			nextEntry := g.entryComps[e]
			if relaxLanding[e] && !occupied[e.To] {
				// The static graph's landing component for e.To was computed from
				// pristine ROM collision. A semantic pivot (Cut, Surf, a switch...)
				// can permanently rewrite that map's tile collision at the exact
				// spot it lands (VermilionGymSetDoorTile, a cut tree), so the
				// precomputed component is not authoritative once the action is
				// taken. Treat the landing as unconstrained, same as a caller who
				// does not know its component (canExit already treats nil this
				// way); the live map, rebuilt fresh once the walker actually
				// stands there, is what execution trusts anyway.
				nextEntry = nil
			}
			key := routeStateIdentity(g, e.To, nextEntry, e)
			if seen[key] {
				continue
			}
			seen[key] = true
			occupied[e.To] = true
			nodes = append(nodes, node{edge: e, prev: prev, entry: nextEntry})
		}
	}
	expand(from, -1, first)
	for i := 0; i < len(nodes); i++ {
		// nodes[i].entry == nil means a semantic pivot deliberately discarded
		// the static landing component (see expand above): "unknown" must not
		// read as "elsewhere," the same rule canExit already applies for an
		// edge whose entry component isn't known.
		if nodes[i].edge.To == to && (len(target) == 0 || nodes[i].entry == nil || shareComp(nodes[i].entry, target)) {
			var route []Edge
			for j := i; j >= 0; j = nodes[j].prev {
				route = append([]Edge{nodes[j].edge}, route...)
			}
			return route, nil
		}
		expand(nodes[i].edge.To, i, nodes[i].entry)
	}
	return nil, ErrNoRoute
}

// canExit reports whether edge e can be taken from the entry component set on
// e.From. A graph built by BuildGraph is component-aware: an edge is usable
// only if its exit port is walkable, and, when the entry component is known
// (entry != nil), only if the exit port shares a component with it. A
// hand-built graph (componentAware false) imposes no constraint, so its
// routes are exactly what the old search produced.
func canExit(g *Graph, e Edge, entry []int) bool {
	if !g.componentAware {
		return true
	}
	if g.comps[e.From] == nil {
		return true // no walkable grid for this map: no info, don't reject
	}
	exit := g.exitComps[e]
	if len(exit) == 0 {
		return false // exit port has no walkable tile: the leg is a phantom
	}
	if entry == nil {
		return true // walkable; the caller does not know which component
	}
	return shareComp(entry, exit)
}

// shareComp reports whether component sets a and b have a member in common.
func shareComp(a, b []int) bool {
	seen := make(map[int]bool, len(a))
	for _, c := range a {
		seen[c] = true
	}
	for _, c := range b {
		if seen[c] {
			return true
		}
	}
	return false
}
