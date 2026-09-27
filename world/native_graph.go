package world

import (
	"fmt"

	"github.com/maestroi/pokepilot/worldmodel"
)

// NativeEdge is the map-level edge equivalent of Edge for cartridges whose
// native map identity needs more than eight bits.
type NativeEdge struct {
	Kind   EdgeKind
	From   uint16
	To     uint16
	WarpX  uint8
	WarpY  uint8
	Dir    uint8
	Offset int8
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
				Kind: EdgeWarp, From: id, To: warp.DestMap, WarpX: warp.X, WarpY: warp.Y,
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
