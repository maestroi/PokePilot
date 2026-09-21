package worldverify

import "testing"

func TestVerifyAcceptsValidComponentGraphAndEnumeratesCapabilities(t *testing.T) {
	snapshot := Snapshot{
		Game: "fixture",
		Maps: []Map{
			{ID: "start", Width: 2, Height: 1, GeometryKnown: true, Components: []int{1}},
			{ID: "island", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}},
		},
		Edges: []Edge{{
			ID:         "surf",
			Kind:       EdgeConnection,
			From:       "start",
			To:         "island",
			Exit:       Port{Known: true, Components: []int{1}},
			Entry:      Port{Known: true, Components: []int{1}},
			Transition: &Transition{ID: "fixture:surf", Requires: []CapabilityID{"can_surf"}},
		}},
		StartMaps:    []MapID{"start"},
		RequiredMaps: []MapID{"island"},
	}

	report := Verify(snapshot, Options{})
	if report.HasErrors() {
		t.Fatalf("unexpected errors: %+v", report.Findings)
	}
	if report.Stats.Capabilities != 1 || report.Stats.CapabilityStatesChecked != 2 || !report.Stats.ExhaustiveCapabilities {
		t.Fatalf("unexpected capability stats: %+v", report.Stats)
	}
	if report.Stats.FullReachableMaps != 2 {
		t.Fatalf("full reachable maps=%d, want 2", report.Stats.FullReachableMaps)
	}
}

func TestVerifyMutationMatrix(t *testing.T) {
	valid := func() Snapshot {
		return Snapshot{
			Maps: []Map{
				{ID: "a", Width: 2, Height: 2, GeometryKnown: true, Components: []int{1}},
				{ID: "b", Width: 2, Height: 2, GeometryKnown: true, Components: []int{1}},
			},
			Edges: []Edge{{
				ID:    "a-b",
				Kind:  EdgeWarp,
				From:  "a",
				To:    "b",
				Exit:  Port{Known: true, Components: []int{1}, Point: &Point{X: 1, Y: 1}},
				Entry: Port{Known: true, Components: []int{1}, Point: &Point{X: 0, Y: 0}},
			}},
			StartMaps: []MapID{"a"},
		}
	}

	tests := []struct {
		name   string
		code   string
		mutate func(*Snapshot)
	}{
		{
			name:   "unknown destination",
			code:   "unknown_edge_destination",
			mutate: func(s *Snapshot) { s.Edges[0].To = "missing" },
		},
		{
			name:   "bad component",
			code:   "unknown_port_component",
			mutate: func(s *Snapshot) { s.Edges[0].Entry.Components = []int{99} },
		},
		{
			name:   "warp out of bounds",
			code:   "point_out_of_bounds",
			mutate: func(s *Snapshot) { s.Edges[0].Exit.Point = &Point{X: 2, Y: 0} },
		},
		{
			name:   "bad border band",
			code:   "invalid_border_span",
			mutate: func(s *Snapshot) { s.Edges[0].BorderSpan = &Span{Start: 0, End: 2, Limit: 2} },
		},
		{
			name:   "invalid transition mode",
			code:   "invalid_transition_mode",
			mutate: func(s *Snapshot) { s.Edges[0].Transition = &Transition{ID: "bad", Gate: true, PivotOnly: true} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := valid()
			tt.mutate(&snapshot)
			report := Verify(snapshot, Options{})
			if !hasFinding(report, tt.code, SeverityError) {
				t.Fatalf("missing %s in %+v", tt.code, report.Findings)
			}
		})
	}
}

func TestVerifyInactiveStaticEdgeIsCountedNotReported(t *testing.T) {
	snapshot := Snapshot{
		Maps: []Map{
			{ID: "a", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}},
			{ID: "b", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}},
		},
		Edges: []Edge{{
			ID:    "inactive",
			From:  "a",
			To:    "b",
			Exit:  Port{Known: true},
			Entry: Port{Known: true},
		}},
	}

	report := Verify(snapshot, Options{})
	if report.HasErrors() || report.WarningCount() != 0 {
		t.Fatalf("inactive static topology is not a finding: %+v", report.Findings)
	}
	if report.Stats.InactiveStaticEdges != 1 {
		t.Fatalf("inactive static edges=%d, want 1", report.Stats.InactiveStaticEdges)
	}
}

func TestVerifySemanticDeadPortIsVisibleButNotAutomaticallyFatal(t *testing.T) {
	snapshot := Snapshot{
		Maps: []Map{
			{ID: "shore", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}},
			{ID: "water", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}},
		},
		Edges: []Edge{{
			ID:         "surf-action",
			From:       "shore",
			To:         "water",
			Exit:       Port{Known: true},
			Entry:      Port{Known: true, Components: []int{1}},
			Transition: &Transition{ID: "surf", Requires: []CapabilityID{"can_surf"}},
		}},
	}

	report := Verify(snapshot, Options{})
	if report.HasErrors() {
		t.Fatalf("semantic action on pristine non-walkable geometry must not be assumed invalid: %+v", report.Findings)
	}
	if !hasFinding(report, "semantic_dead_exit_port", SeverityWarning) {
		t.Fatalf("expected semantic dead-port warning, got %+v", report.Findings)
	}
	if report.Stats.SemanticDeadPortEdges != 1 {
		t.Fatalf("semantic dead-port edges=%d, want 1", report.Stats.SemanticDeadPortEdges)
	}
}

func TestVerifyRequiredMapMustBeReachableWithFullCapabilities(t *testing.T) {
	snapshot := Snapshot{
		Maps: []Map{
			{ID: "a", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}},
			{ID: "b", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}},
		},
		StartMaps:    []MapID{"a"},
		RequiredMaps: []MapID{"b"},
	}
	report := Verify(snapshot, Options{})
	if !hasFinding(report, "required_map_unreachable", SeverityError) {
		t.Fatalf("expected required_map_unreachable, got %+v", report.Findings)
	}
}

func TestVerifyFallsBackToBoundaryCapabilityStates(t *testing.T) {
	var requires []CapabilityID
	for i := 0; i < 5; i++ {
		requires = append(requires, CapabilityID(string(rune('a'+i))))
	}
	snapshot := Snapshot{
		Maps: []Map{{ID: "a", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}}},
		Edges: []Edge{{
			ID:         "loop",
			From:       "a",
			To:         "a",
			Exit:       Port{Known: true, Components: []int{1}},
			Entry:      Port{Known: true, Components: []int{1}},
			Transition: &Transition{ID: "many", Requires: requires},
		}},
		StartMaps: []MapID{"a"},
	}
	report := Verify(snapshot, Options{MaxExhaustiveCapabilities: 3})
	if report.Stats.ExhaustiveCapabilities {
		t.Fatal("expected bounded non-exhaustive capability exploration")
	}
	if report.Stats.CapabilityStatesChecked != 12 { // empty + full + 5 singles + 5 full-minus-one
		t.Fatalf("states=%d, want 12", report.Stats.CapabilityStatesChecked)
	}
}

func hasFinding(report Report, code string, severity Severity) bool {
	for _, finding := range report.Findings {
		if finding.Code == code && finding.Severity == severity {
			return true
		}
	}
	return false
}

func TestVerifyRejectsGraphExitComponentWithoutExecutableApproach(t *testing.T) {
	snapshot := Snapshot{
		Maps: []Map{
			{ID: "a", Width: 3, Height: 1, GeometryKnown: true, Components: []int{1, 2}},
			{ID: "b", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}},
		},
		Edges: []Edge{{
			ID:    "a-b",
			Kind:  EdgeConnection,
			From:  "a",
			To:    "b",
			Exit:  Port{Known: true, Components: []int{1, 2}},
			Entry: Port{Known: true, Components: []int{1}},
			Execution: &ExecutionEvidence{
				Status: ExecutionProven,
				Paths: []ExecutionPath{{
					ExitComponent:  1,
					EntryComponent: 1,
					ExitPoint:      Point{X: 0, Y: 0},
					EntryPoint:     Point{X: 0, Y: 0},
				}},
			},
		}},
	}

	report := Verify(snapshot, Options{})
	if !hasFinding(report, "graph_executor_exit_mismatch", SeverityError) {
		t.Fatalf("expected graph_executor_exit_mismatch, got %+v", report.Findings)
	}
}

func TestVerifyRejectsExecutorLandingOutsideAdvertisedEntry(t *testing.T) {
	snapshot := Snapshot{
		Maps: []Map{
			{ID: "a", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}},
			{ID: "b", Width: 2, Height: 1, GeometryKnown: true, Components: []int{1, 2}},
		},
		Edges: []Edge{{
			ID:    "warp",
			Kind:  EdgeWarp,
			From:  "a",
			To:    "b",
			Exit:  Port{Known: true, Components: []int{1}},
			Entry: Port{Known: true, Components: []int{1}},
			Execution: &ExecutionEvidence{
				Status: ExecutionProven,
				Paths: []ExecutionPath{{
					ExitComponent:  1,
					EntryComponent: 2,
					ExitPoint:      Point{X: 0, Y: 0},
					EntryPoint:     Point{X: 1, Y: 0},
				}},
			},
		}},
	}

	report := Verify(snapshot, Options{})
	if !hasFinding(report, "executor_landing_not_advertised", SeverityError) {
		t.Fatalf("expected executor_landing_not_advertised, got %+v", report.Findings)
	}
}

func TestVerifyDynamicExecutionUnknownIsInformational(t *testing.T) {
	snapshot := Snapshot{
		Maps: []Map{
			{ID: "a", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}},
			{ID: "b", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}},
		},
		Edges: []Edge{{
			ID:    "surf",
			Kind:  EdgeConnection,
			From:  "a",
			To:    "b",
			Exit:  Port{Known: false},
			Entry: Port{Known: false},
			Execution: &ExecutionEvidence{
				Status: ExecutionDynamicUnknown,
				Reason: "requires live Surf topology",
			},
		}},
	}

	report := Verify(snapshot, Options{})
	if report.HasErrors() || report.WarningCount() != 0 {
		t.Fatalf("dynamic unknown should be informational only: %+v", report.Findings)
	}
	if !hasFinding(report, "dynamic_execution_unknown", SeverityInfo) {
		t.Fatalf("expected dynamic_execution_unknown info, got %+v", report.Findings)
	}
	if report.Stats.DynamicExecutionEdges != 1 {
		t.Fatalf("dynamic execution edges=%d, want 1", report.Stats.DynamicExecutionEdges)
	}
}

func TestVerifyReportsExpectedMapParseDiagnostic(t *testing.T) {
	snapshot := Snapshot{
		Maps: []Map{{ID: "01", Width: 1, Height: 1, GeometryKnown: true, Components: []int{1}}},
		MapParseDiagnostics: []MapParseDiagnostic{{
			Map:    "02",
			Error:  "truncated map header",
			Reason: "dead duplicate map excluded by adapter",
		}},
	}

	report := Verify(snapshot, Options{})
	if report.HasErrors() {
		t.Fatalf("expected parse diagnostic must be non-fatal: %+v", report.Findings)
	}
	if report.Stats.ExpectedMapParseFailures != 1 {
		t.Fatalf("expected parse failures=%d, want 1", report.Stats.ExpectedMapParseFailures)
	}
	if !hasFinding(report, "expected_map_parse_failure", SeverityWarning) {
		t.Fatalf("expected parse diagnostic missing from report: %+v", report.Findings)
	}
}
