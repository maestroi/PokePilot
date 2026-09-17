package world

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

func TestValidationSnapshotTreatsPortBypassAsPostActionGeometry(t *testing.T) {
	edge := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
	g := &Graph{
		componentAware: true,
		Edges:          map[uint8][]Edge{1: {edge}, 2: nil},
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{1}},
		},
		exitComps:  map[Edge][]int{edge: nil},
		entryComps: map[Edge][]int{edge: nil},
		tiles:      map[uint8]dim{1: {w: 1, h: 1}, 2: {w: 1, h: 1}},
	}
	transitions := map[Edge]gameruntime.Transition{
		edge: {ID: "fixture:surf", Requires: []gameruntime.CapabilityID{"can_surf"}, PortBypass: true},
	}

	snapshot := ValidationSnapshot(g, transitions)
	if len(snapshot.Edges) != 1 {
		t.Fatalf("snapshot edges=%d, want 1", len(snapshot.Edges))
	}
	got := snapshot.Edges[0]
	if got.Transition == nil || !got.Transition.PortBypass {
		t.Fatalf("transition did not preserve port bypass: %+v", got.Transition)
	}
	if got.Exit.Known || got.Entry.Known {
		t.Fatalf("port-bypass action kept pristine ports authoritative: exit=%+v entry=%+v", got.Exit, got.Entry)
	}

	report := VerifyGraph(g, transitions)
	if report.HasErrors() || report.WarningCount() != 0 {
		t.Fatalf("expected clean verification for action-owned dead port, got %+v", report.Findings)
	}
}
