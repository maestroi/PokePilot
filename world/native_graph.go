package world

import (
	"fmt"

	"github.com/maestroi/pokepilot/worldmodel"
)

// NativeEdge is the map-level edge equivalent of Edge for cartridges whose
// native map identity needs more than eight bits.
type NativeEdge struct {
	Kind  EdgeKind
	From  uint16
	To    uint16
	WarpX uint8
	WarpY uint8
	// DestWarp is the warp the player arrives on, for EdgeWarp.
	DestWarp uint8
	Dir      uint8
	Offset   int8
}

// NativeGraph is a topology-only graph keyed by native cartridge map ids.
// Tile/component geometry intentionally remains outside this first boundary.
type NativeGraph struct {
	Edges map[uint16][]NativeEdge
}

// BuildNativeGraph builds a map-level graph without narrowing native map ids.
// Edges that point outside the provider's enumerated slice are omitted: a
// partial adapter can expose an honest connected vertical slice without
// pretending unsupported maps are routable.
func BuildNativeGraph(provider worldmodel.NativeMapTopologyProvider) (*NativeGraph, error) {
	if provider == nil {
		return nil, fmt.Errorf("world: nil native map topology provider")
	}

	headers := make(map[uint16]worldmodel.NativeMapHeader)
	for _, id := range provider.MapIDs() {
		if _, duplicate := headers[id]; duplicate {
			return nil, fmt.Errorf("world: native map provider returned duplicate map %#04x", id)
		}
		header, err := provider.ParseMap(id)
		if err != nil {
			return nil, fmt.Errorf("world: parse native map %#04x: %w", id, err)
		}
		if header.ID != id {
			return nil, fmt.Errorf("world: native map %#04x parsed as %#04x", id, header.ID)
		}
		headers[id] = header
	}
	if len(headers) == 0 {
		return nil, fmt.Errorf("world: native map provider returned no maps")
	}

	graph := &NativeGraph{Edges: make(map[uint16][]NativeEdge, len(headers))}
	for id, header := range headers {
		graph.Edges[id] = nil
		for _, warp := range header.Warps {
			if warp.Inert {
				continue
			}
			if _, supported := headers[warp.DestMap]; !supported {
				continue
			}
			graph.Edges[id] = append(graph.Edges[id], NativeEdge{
				Kind: EdgeWarp, From: id, To: warp.DestMap, WarpX: warp.X, WarpY: warp.Y, DestWarp: warp.DestWarpID,
			})
		}
		for _, connection := range header.Connections {
			if _, supported := headers[connection.MapID]; !supported {
				continue
			}
			graph.Edges[id] = append(graph.Edges[id], NativeEdge{
				Kind: EdgeConnection, From: id, To: connection.MapID, Dir: connection.Dir, Offset: connection.Offset,
			})
		}
	}
	return graph, nil
}

// FindNativeRoute returns the fewest map transitions between native map ids.
// Tile-level reachability is intentionally not inferred here.
func FindNativeRoute(graph *NativeGraph, from, to uint16) ([]NativeEdge, error) {
	if graph == nil {
		return nil, fmt.Errorf("world: nil native graph")
	}
	if from == to {
		return []NativeEdge{}, nil
	}
	if _, ok := graph.Edges[from]; !ok {
		return nil, fmt.Errorf("%w: native start map %#04x is unavailable", ErrNoRoute, from)
	}
	if _, ok := graph.Edges[to]; !ok {
		return nil, fmt.Errorf("%w: native destination map %#04x is unavailable", ErrNoRoute, to)
	}

	type node struct {
		mapID uint16
		prev  int
		edge  NativeEdge
	}
	nodes := []node{{mapID: from, prev: -1}}
	seen := map[uint16]bool{from: true}

	for i := 0; i < len(nodes); i++ {
		for _, edge := range graph.Edges[nodes[i].mapID] {
			if seen[edge.To] {
				continue
			}
			seen[edge.To] = true
			nodes = append(nodes, node{mapID: edge.To, prev: i, edge: edge})
			next := len(nodes) - 1
			if edge.To != to {
				continue
			}
			var route []NativeEdge
			for j := next; nodes[j].prev >= 0; j = nodes[j].prev {
				route = append([]NativeEdge{nodes[j].edge}, route...)
			}
			return route, nil
		}
	}
	return nil, ErrNoRoute
}

// NativeEntryUnknown is the entry of a map the player was already standing on.
const NativeEntryUnknown = -1

// Entry names how an edge lands on its destination: the warp arrived on, or
// the seam direction. Two entries into one map can sit in different
// walkable components (Sprout Tower 2F), so a route is searched over
// (map, entry) rather than over maps alone.
func (e NativeEdge) Entry() int {
	if e.Kind == EdgeWarp {
		return int(e.DestWarp)
	}
	return -2 - int(e.Dir)
}

// NativeUnreachable records an edge whose approach had no walkable path from
// the component a given entry lands in.
type NativeUnreachable struct {
	Map   uint16
	Entry int
	Edge  NativeEdge
}

// FindNativeRouteFrom is FindNativeRoute for a player who entered `from`
// through `entry`, skipping edges already proven unreachable from that entry.
// The graph is topology-only, so such proofs come from live tile geometry;
// the route then leaves the map and re-enters through another entry.
func FindNativeRouteFrom(graph *NativeGraph, from uint16, entry int, to uint16, bad map[NativeUnreachable]bool) ([]NativeEdge, error) {
	if graph != nil && from == to {
		return []NativeEdge{}, nil
	}
	return FindNativeRouteToEntry(graph, from, entry, to, nil, bad)
}

// FindNativeRouteToEntry is FindNativeRouteFrom with a goal on HOW the
// destination map is entered: the route must end with an edge into `to` that
// entryOK accepts (nil accepts any). Unlike FindNativeRouteFrom it also routes
// from a map to itself, which is how a goal tile stranded in another walkable
// component of the player's own map is reached: leave, then re-enter through
// the entry whose landing connects to it.
func FindNativeRouteToEntry(graph *NativeGraph, from uint16, entry int, to uint16, entryOK func(NativeEdge) bool, bad map[NativeUnreachable]bool) ([]NativeEdge, error) {
	if graph == nil {
		return nil, fmt.Errorf("world: nil native graph")
	}
	if _, ok := graph.Edges[from]; !ok {
		return nil, fmt.Errorf("%w: native start map %#04x is unavailable", ErrNoRoute, from)
	}
	if _, ok := graph.Edges[to]; !ok {
		return nil, fmt.Errorf("%w: native destination map %#04x is unavailable", ErrNoRoute, to)
	}

	type state struct {
		mapID uint16
		entry int
	}
	type node struct {
		state state
		prev  int
		edge  NativeEdge
	}
	start := state{from, entry}
	nodes := []node{{state: start, prev: -1}}
	seen := map[state]bool{start: true}

	for i := 0; i < len(nodes); i++ {
		cur := nodes[i].state
		for _, edge := range graph.Edges[cur.mapID] {
			next := state{edge.To, edge.Entry()}
			if seen[next] || bad[NativeUnreachable{cur.mapID, cur.entry, edge}] {
				continue
			}
			seen[next] = true
			nodes = append(nodes, node{state: next, prev: i, edge: edge})
			if edge.To != to || (entryOK != nil && !entryOK(edge)) {
				continue
			}
			var route []NativeEdge
			for j := len(nodes) - 1; nodes[j].prev >= 0; j = nodes[j].prev {
				route = append([]NativeEdge{nodes[j].edge}, route...)
			}
			return route, nil
		}
	}
	return nil, ErrNoRoute
}
