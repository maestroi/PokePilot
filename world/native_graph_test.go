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
				Dir:   dirEast,
				MapID: groupTwoMapOne,
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
