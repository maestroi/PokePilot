package world

import (
	"errors"
)

// ErrNoRoute reports that no sequence of warps/connections links the maps.
var ErrNoRoute = errors.New("world: no route")

// FindRoute returns the edges to traverse, in order, to get from map
// `from` to map `to`. It returns an empty slice when from == to.
//
// It is a plain breadth-first search over Graph.Edges, so the returned
// route has the fewest map transitions. Edges of a map are explored in
// slice order, so the same call always returns the same route. Tile-level
// pathfinding is not done here; that happens per leg at execution time.
func FindRoute(g *Graph, from, to uint8) ([]Edge, error) {
	return FindRouteAvoiding(g, from, to, nil)
}

// FindRouteAvoiding is FindRoute with a set of edges excluded from the
// search. It exists because the map graph knows which maps TOUCH, not
// which are walkable between: Route 2 connects Viridian to Pewter in one
// hop, but the map is split across its width by ledges, so the walk that
// hop implies is impossible. Only the tile-level pathfinder can discover
// that, and only at execution time, so a caller that hits such a leg
// bans it and asks again — here, the warp chain through Viridian Forest.
//
// Edge is comparable, so the caller's set is a plain map[Edge]bool.
func FindRouteAvoiding(g *Graph, from, to uint8, blocked map[Edge]bool) ([]Edge, error) {
	if from == to {
		return []Edge{}, nil
	}
	// parent[m] is the edge first taken to reach m.
	parent := make(map[uint8]Edge)
	visited := map[uint8]bool{from: true}
	queue := []uint8{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range g.Edges[cur] {
			if visited[e.To] || blocked[e] {
				continue
			}
			visited[e.To] = true
			parent[e.To] = e
			if e.To == to {
				route := []Edge{e}
				for m := cur; m != from; m = parent[m].From {
					route = append([]Edge{parent[m]}, route...)
				}
				return route, nil
			}
			queue = append(queue, e.To)
		}
	}
	return nil, ErrNoRoute
}
