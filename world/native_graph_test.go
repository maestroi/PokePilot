package world

import (
	"fmt"
	"sort"
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

type fakeNativeTopologyProvider map[uint16]worldmodel.NativeMapHeader

func (p fakeNativeTopologyProvider) MapIDs() []uint16 {
	ids := make([]uint16, 0, len(p))
	for id := range p {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (p fakeNativeTopologyProvider) ParseMap(id uint16) (worldmodel.NativeMapHeader, error) {
	header, ok := p[id]
	if !ok {
		return worldmodel.NativeMapHeader{}, fmt.Errorf("unknown map %#04x", id)
	}
	return header, nil
}

func TestBuildNativeGraphPreservesMapGroupIdentity(t *testing.T) {
	const (
		groupOneMapOne uint16 = 0x0101
		groupTwoMapOne uint16 = 0x0201
	)
	provider := fakeNativeTopologyProvider{
		groupOneMapOne: {
			ID: groupOneMapOne,
			Connections: []worldmodel.NativeConnection{{
				Dir:    dirEast,
				MapID:  groupTwoMapOne,
				Offset: -3,
			}},
		},
		groupTwoMapOne: {ID: groupTwoMapOne},
	}

	graph, err := BuildNativeGraph(provider)
	if err != nil {
		t.Fatalf("BuildNativeGraph: %v", err)
	}
	if len(graph.Edges) != 2 {
		t.Fatalf("graph has %d map ids, want 2", len(graph.Edges))
	}
	edges := graph.Edges[groupOneMapOne]
	if len(edges) != 1 {
		t.Fatalf("map %#04x has %d edges, want 1", groupOneMapOne, len(edges))
	}
	if got := edges[0].To; got != groupTwoMapOne {
		t.Fatalf("edge destination = %#04x, want %#04x", got, groupTwoMapOne)
	}
	if got := edges[0].Offset; got != -3 {
		t.Fatalf("edge offset = %d, want -3", got)
	}
}

func TestBuildNativeGraphOmitsUnsupportedDestination(t *testing.T) {
	const (
		supported   uint16 = 0x1804
		unsupported uint16 = 0x1801
	)
	provider := fakeNativeTopologyProvider{
		supported: {
			ID: supported,
			Connections: []worldmodel.NativeConnection{{
				Dir:   dirEast,
				MapID: unsupported,
			}},
		},
	}
	graph, err := BuildNativeGraph(provider)
	if err != nil {
		t.Fatalf("BuildNativeGraph: %v", err)
	}
	if got := len(graph.Edges[supported]); got != 0 {
		t.Fatalf("partial provider leaked %d edge(s) to unsupported map", got)
	}
}

func TestFindNativeRoute(t *testing.T) {
	const (
		a uint16 = 0x1807
		b uint16 = 0x1806
		c uint16 = 0x1804
	)
	provider := fakeNativeTopologyProvider{
		a: {ID: a, Warps: []worldmodel.NativeWarp{{DestMap: b}}},
		b: {ID: b, Warps: []worldmodel.NativeWarp{{DestMap: c}}},
		c: {ID: c},
	}
	graph, err := BuildNativeGraph(provider)
	if err != nil {
		t.Fatalf("BuildNativeGraph: %v", err)
	}
	route, err := FindNativeRoute(graph, a, c)
	if err != nil {
		t.Fatalf("FindNativeRoute: %v", err)
	}
	if len(route) != 2 || route[0].To != b || route[1].To != c {
		t.Fatalf("route = %#v, want %#04x -> %#04x", route, b, c)
	}
}

func TestFindNativeRouteFromReentersThroughOtherEntry(t *testing.T) {
	// Map 2 has two stairs in from 1; only entry 1 can reach the stairs to 3.
	up0 := NativeEdge{Kind: EdgeWarp, From: 1, To: 2, DestWarp: 0}
	up1 := NativeEdge{Kind: EdgeWarp, From: 1, To: 2, DestWarp: 1}
	down := NativeEdge{Kind: EdgeWarp, From: 2, To: 1}
	top := NativeEdge{Kind: EdgeWarp, From: 2, To: 3}
	g := &NativeGraph{Edges: map[uint16][]NativeEdge{1: {up0, up1}, 2: {down, top}, 3: nil}}

	bad := map[NativeUnreachable]bool{{Map: 2, Entry: 0, Edge: top}: true}
	route, err := FindNativeRouteFrom(g, 2, 0, 3, bad)
	if err != nil || len(route) != 3 || route[0] != down || route[1] != up1 || route[2] != top {
		t.Fatalf("route = %v, %v; want down, re-enter via entry 1, top", route, err)
	}
}

// A goal tile can sit in a different walkable component of the player's own
// map. FindNativeRouteToEntry routes map -> other map -> back in through the
// one entry whose landing reaches it, rather than reporting an empty route.
func TestFindNativeRouteToEntryReentersThroughAcceptedWarp(t *testing.T) {
	const a, b uint16 = 0x0301, 0x0302
	graph := &NativeGraph{Edges: map[uint16][]NativeEdge{
		a: {{Kind: EdgeWarp, From: a, To: b, WarpX: 1, WarpY: 1, DestWarp: 0}},
		b: {
			{Kind: EdgeWarp, From: b, To: a, WarpX: 2, WarpY: 2, DestWarp: 0},
			{Kind: EdgeWarp, From: b, To: a, WarpX: 3, WarpY: 3, DestWarp: 1},
		},
	}}
	if route, err := FindNativeRouteFrom(graph, a, 0, a, nil); err != nil || len(route) != 0 {
		t.Fatalf("same-map route = %v, %v; want empty", route, err)
	}
	onlyLanding1 := func(e NativeEdge) bool { return e.Entry() == 1 }
	route, err := FindNativeRouteToEntry(graph, a, 0, a, onlyLanding1, nil)
	if err != nil || len(route) != 2 || route[0].To != b || route[1].To != a || route[1].DestWarp != 1 {
		t.Fatalf("route = %+v, %v; want a->b then b->a via warp 1", route, err)
	}
	// A warp proven dead from this entry is skipped, so no accepted route remains.
	bad := map[NativeUnreachable]bool{{Map: b, Entry: 0, Edge: graph.Edges[b][1]}: true}
	if _, err := FindNativeRouteToEntry(graph, a, 0, a, onlyLanding1, bad); err == nil {
		t.Fatal("route through a dead edge unexpectedly found")
	}
}
