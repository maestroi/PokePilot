package world

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/worldmodel"
	"github.com/maestroi/pokepilot/worldverify"
)

func TestVerifyGraphCountsPhantomOrdinaryEdgeAsInactive(t *testing.T) {
	edge := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast, BandScoped: true, BandStart: 0, BandEnd: 0}
	g := &Graph{
		Edges:          map[uint8][]Edge{1: {edge}, 2: nil},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{1}},
		},
		exitComps:  map[Edge][]int{edge: nil},
		entryComps: map[Edge][]int{edge: {1}},
		tiles:      map[uint8]dim{1: {w: 1, h: 1}, 2: {w: 1, h: 1}},
	}

	report := VerifyGraph(g, nil, 1)
	if report.HasErrors() {
		t.Fatalf("inactive ordinary edge must not be an error: %+v", report.Findings)
	}
	if reportHasFinding(report, "dead_exit_port", worldverify.SeverityError) ||
		reportHasFinding(report, "dead_entry_port", worldverify.SeverityError) {
		t.Fatalf("inactive ordinary edge was promoted to a geometry error: %+v", report.Findings)
	}
	// With map 1 declared as the start, map 2 is independently and correctly
	// reported as an unexplained full-capability reachability gap. That finding
	// is about the map manifest, not the inactive edge classification exercised
	// by this test.
	if !reportHasFinding(report, "unclassified_unreachable_map", worldverify.SeverityWarning) {
		t.Fatalf("expected separate reachability warning for map 2, got %+v", report.Findings)
	}
	if report.Stats.InactiveStaticEdges != 1 {
		t.Fatalf("inactive static edges=%d, want 1", report.Stats.InactiveStaticEdges)
	}
}

func TestVerifyGraphKeepsSemanticGeometryMismatchVisible(t *testing.T) {
	edge := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast, BandScoped: true, BandStart: 0, BandEnd: 0}
	g := &Graph{
		Edges:          map[uint8][]Edge{1: {edge}, 2: nil},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{1}},
		},
		exitComps:  map[Edge][]int{edge: nil},
		entryComps: map[Edge][]int{edge: {1}},
		tiles:      map[uint8]dim{1: {w: 1, h: 1}, 2: {w: 1, h: 1}},
	}
	transitions := map[Edge]gameruntime.Transition{
		edge: {ID: "fixture:surf", Requires: []gameruntime.CapabilityID{"can_surf"}},
	}

	report := VerifyGraph(g, transitions, 1)
	if report.HasErrors() {
		t.Fatalf("semantic action may create collision traversal: %+v", report.Findings)
	}
	if !reportHasFinding(report, "semantic_dead_exit_port", worldverify.SeverityWarning) {
		t.Fatalf("expected semantic_dead_exit_port warning, got %+v", report.Findings)
	}
	if report.Stats.Capabilities != 1 || report.Stats.CapabilityStatesChecked != 2 {
		t.Fatalf("unexpected capability exploration: %+v", report.Stats)
	}
	if report.Stats.SemanticDeadPortEdges != 1 {
		t.Fatalf("semantic dead-port edges=%d, want 1", report.Stats.SemanticDeadPortEdges)
	}
}

func TestValidationSnapshotPreservesConnectionBandLimit(t *testing.T) {
	edge := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirSouth, BandScoped: true, BandStart: 1, BandEnd: 3}
	g := &Graph{
		Edges:          map[uint8][]Edge{1: {edge}, 2: nil},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1, 1}, {1, 1}},
			2: {{1, 1}, {1, 1}},
		},
		exitComps:  map[Edge][]int{edge: {1}},
		entryComps: map[Edge][]int{edge: {1}},
		tiles:      map[uint8]dim{1: {w: 2, h: 2}, 2: {w: 2, h: 2}},
	}

	report := VerifyGraph(g, nil, 1)
	if !reportHasFinding(report, "invalid_border_span", worldverify.SeverityError) {
		t.Fatalf("expected invalid_border_span, got %+v", report.Findings)
	}
}

func reportHasFinding(report worldverify.Report, code string, severity worldverify.Severity) bool {
	for _, finding := range report.Findings {
		if finding.Code == code && finding.Severity == severity {
			return true
		}
	}
	return false
}


func TestVerifyGraphDetectsWarpLandingExecutorMismatch(t *testing.T) {
	edge := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 0, WarpY: 0}
	g := &Graph{
		Edges:          map[uint8][]Edge{1: {edge}, 2: nil},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{0, 1}},
			2: {{0, 2, 1}},
		},
		exitComps:  map[Edge][]int{edge: {1}},
		entryComps: map[Edge][]int{edge: {1}}, // stale graph claim; actual landing is component 2
		warps: map[uint8][]worldmodel.Warp{
			1: {{X: 0, Y: 0, DestWarpID: 0, DestMap: 2}},
			2: {{X: 0, Y: 0, DestWarpID: 0, DestMap: 1}},
		},
		tiles: map[uint8]dim{1: {w: 2, h: 1}, 2: {w: 3, h: 1}},
	}

	report := VerifyGraph(g, nil, 1)
	if !reportHasFinding(report, "executor_landing_not_advertised", worldverify.SeverityError) {
		t.Fatalf("expected warp executor landing mismatch, got %+v", report.Findings)
	}
}

func TestVerifyGraphDetectsConnectionBandLandingMismatch(t *testing.T) {
	edge := Edge{
		Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast,
		BandScoped: true, BandStart: 0, BandEnd: 0,
	}
	g := &Graph{
		Edges:          map[uint8][]Edge{1: {edge}, 2: nil},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{2, 1}},
		},
		exitComps:  map[Edge][]int{edge: {1}},
		entryComps: map[Edge][]int{edge: {1}}, // aggregate/stale claim; seam actually lands in 2
		tiles:      map[uint8]dim{1: {w: 1, h: 1}, 2: {w: 2, h: 1}},
		connections: map[Edge]worldmodel.Connection{
			edge: {Dir: dirEast, MapID: 2, Offset: 0},
		},
	}

	report := VerifyGraph(g, nil, 1)
	if !reportHasFinding(report, "executor_landing_not_advertised", worldverify.SeverityError) {
		t.Fatalf("expected connection-band executor landing mismatch, got %+v", report.Findings)
	}
}

func TestValidationSnapshotUsesProviderInertWarpSemanticsForExecution(t *testing.T) {
	// Regression shape from the measured Silph Co inert-teleporter failures:
	// an inaccessible warp-table entry is ordinary floor and must be neither a
	// graph edge nor an execution blocker. The portable provider carries the
	// same Inert bit derived from red/rom.IsInertWarp that runtime navigation
	// uses when building warpAvoidance/warpTarget.
	g, err := BuildGraph(fakeMapProvider{})
	if err != nil {
		t.Fatalf("BuildGraph(fake provider): %v", err)
	}
	snapshot := ValidationSnapshot(g, nil, 1)
	seenActive := false
	for _, edge := range snapshot.Edges {
		if edge.From != validationMapID(1) || edge.Kind != worldverify.EdgeWarp {
			continue
		}
		if edge.Exit.Point != nil && edge.Exit.Point.X == 1 && edge.Exit.Point.Y == 0 {
			t.Fatalf("inert warp leaked into validation snapshot: %+v", edge)
		}
		if edge.Exit.Point != nil && edge.Exit.Point.X == 0 && edge.Exit.Point.Y == 0 {
			seenActive = true
			if edge.Execution == nil || edge.Execution.Status != worldverify.ExecutionProven || len(edge.Execution.Paths) == 0 {
				t.Fatalf("active warp lacks executable proof: %+v", edge.Execution)
			}
		}
	}
	if !seenActive {
		t.Fatal("active warp missing from validation snapshot")
	}
}
