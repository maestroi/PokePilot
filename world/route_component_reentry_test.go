package world

import "testing"

func TestEdgeEntrySharesComponentWithDistinguishesFreshRegion(t *testing.T) {
	const target = uint8(2)
	same := Edge{Kind: EdgeWarp, From: 1, To: target, WarpX: 1, WarpY: 1}
	fresh := Edge{Kind: EdgeWarp, From: 1, To: target, WarpX: 2, WarpY: 1}
	g := &Graph{
		componentAware: true,
		comps: map[uint8][][]int{
			target: {{1, 1, 2, 2}},
		},
		entryComps: map[Edge][]int{
			same:  {1},
			fresh: {2},
		},
	}

	if got, known := g.EdgeEntrySharesComponentWith(same, 0, 0); !known || !got {
		t.Fatalf("same-component entry = (%v,%v), want (true,true)", got, known)
	}
	if got, known := g.EdgeEntrySharesComponentWith(fresh, 0, 0); !known || got {
		t.Fatalf("fresh-component entry = (%v,%v), want (false,true)", got, known)
	}

	legacy := &Graph{}
	if got, known := legacy.EdgeEntrySharesComponentWith(same, 0, 0); known || got {
		t.Fatalf("component-unknown graph = (%v,%v), want (false,false)", got, known)
	}
}
