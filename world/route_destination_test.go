package world

import (
	"reflect"
	"testing"
)

func TestFindRouteAtDestinationReentersSameMapForDifferentComponent(t *testing.T) {
	out := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 0, WarpY: 0}
	back := Edge{Kind: EdgeWarp, From: 2, To: 1, WarpX: 0, WarpY: 0}
	g := &Graph{
		Edges: map[uint8][]Edge{
			1: {out},
			2: {back},
		},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1, 2}},
			2: {{1}},
		},
		exitComps: map[Edge][]int{
			out:  {1},
			back: {1},
		},
		entryComps: map[Edge][]int{
			out:  {1},
			back: {2},
		},
	}

	got, err := FindRouteAtDestination(g, 1, 1, 0, 0, 1, 0, nil)
	if err != nil {
		t.Fatalf("FindRouteAtDestination: %v", err)
	}
	want := []Edge{out, back}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("route = %#v, want %#v", got, want)
	}

	got, err = FindRouteAtDestination(g, 1, 1, 1, 0, 1, 0, nil)
	if err != nil {
		t.Fatalf("same-component FindRouteAtDestination: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("same-component route = %#v, want empty", got)
	}
}
