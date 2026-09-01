package world

import (
	"errors"
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
// The search state is the EDGE just taken, not the map stood on. Keying on
// the map yields simple paths only, and the walkable way to Pewter re-enters
// Route 2 after a detour through Viridian Forest. Taking one edge twice can
// never help, so barring repeated edges still terminates and still returns
// the fewest transitions.
//
// Edge is comparable, so the caller's set is a plain map[Edge]bool.
func FindRouteAvoiding(g *Graph, from, to uint8, blockedHere map[Edge]bool) ([]Edge, error) {
	return findRoute(g, from, to, blockedHere, nil)
}

// FindRouteAt is FindRouteAvoiding with the player's position on `from` known:
// the first hop is constrained to exits reachable from (x, y), i.e. in the
// same walkable component. Use it when the start map has disconnected
// components (Route 2, the gate maps) and the caller knows which one it stands
// in; the component the player is in is the only honest first-hop constraint.
func FindRouteAt(g *Graph, from, to uint8, x, y int, blockedHere map[Edge]bool) ([]Edge, error) {
	var first []int
	if g.componentAware {
		if c := g.comps[from]; c != nil && y >= 0 && y < len(c) && x >= 0 && x < len(c[0]) && c[y][x] != 0 {
			first = []int{c[y][x]}
		}
	}
	return findRoute(g, from, to, blockedHere, first)
}

func findRoute(g *Graph, from, to uint8, blockedHere map[Edge]bool, first []int) ([]Edge, error) {
	if from == to {
		return []Edge{}, nil
	}
	// node.prev indexes back into nodes, or -1 for a first hop.
	type node struct {
		edge Edge
		prev int
	}
	var nodes []node
	seen := make(map[Edge]bool)
	expand := func(cur uint8, prev int, entry []int) {
		for _, e := range g.Edges[cur] {
			if seen[e] || (prev < 0 && blockedHere[e]) {
				continue
			}
			if !canExit(g, e, entry) {
				continue
			}
			seen[e] = true
			nodes = append(nodes, node{edge: e, prev: prev})
		}
	}
	expand(from, -1, first)
	for i := 0; i < len(nodes); i++ {
		if nodes[i].edge.To == to {
			var route []Edge
			for j := i; j >= 0; j = nodes[j].prev {
				route = append([]Edge{nodes[j].edge}, route...)
			}
			return route, nil
		}
		expand(nodes[i].edge.To, i, g.entryComps[nodes[i].edge])
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
