package worldverify

import "testing"

func TestReachabilityManifestClassifiesUnreachableMaps(t *testing.T) {
	snapshot := Snapshot{
		Game:      "fixture",
		StartMaps: []MapID{"start"},
		Maps: []Map{
			{ID: "start"},
			{ID: "required"},
			{ID: "optional", Label: "OPTIONAL_ROOM"},
			{ID: "dead", Label: "UNUSED_ROOM"},
			{ID: "scripted", Label: "SCRIPT_ROOM"},
			{ID: "mystery", Label: "MYSTERY_ROOM"},
		},
		Edges: []Edge{{ID: "start-required", From: "start", To: "required"}},
		MapExpectations: []MapExpectation{
			{Map: "required", Class: ReachabilityRequired, Reason: "completion path"},
			{Map: "optional", Class: ReachabilityOptional, Reason: "side room"},
			{Map: "dead", Class: ReachabilityExpectedUnreachable, Reason: "unused data"},
			{Map: "scripted", Class: ReachabilityStoryStateDependent, Reason: "script transfer"},
		},
	}

	report := Verify(snapshot, Options{})
	if report.HasErrors() {
		t.Fatalf("unexpected errors: %+v", report.Findings)
	}
	if report.WarningCount() != 1 {
		t.Fatalf("warnings = %d, want 1: %+v", report.WarningCount(), report.Findings)
	}
	if report.Stats.FullUnreachableMaps != 4 || report.Stats.OptionalUnreachableMaps != 1 ||
		report.Stats.ExpectedUnreachableMaps != 1 || report.Stats.StoryStateDependentUnreachableMaps != 1 ||
		report.Stats.SuspiciousUnreachableMaps != 1 {
		t.Fatalf("unexpected reachability stats: %+v", report.Stats)
	}
	if len(report.UnreachableMaps) != 4 {
		t.Fatalf("unreachable maps = %+v, want 4", report.UnreachableMaps)
	}
	foundMystery := false
	for _, unreachable := range report.UnreachableMaps {
		if unreachable.Map == "mystery" {
			foundMystery = true
			if unreachable.Class != ReachabilitySuspicious {
				t.Fatalf("mystery class = %q, want suspicious", unreachable.Class)
			}
		}
	}
	if !foundMystery {
		t.Fatal("mystery map missing from reachability report")
	}
}

func TestRequiredUnreachableMapIsError(t *testing.T) {
	report := Verify(Snapshot{
		StartMaps: []MapID{"start"},
		Maps:      []Map{{ID: "start"}, {ID: "boss"}},
		MapExpectations: []MapExpectation{{
			Map: "boss", Class: ReachabilityRequired, Reason: "final boss",
		}},
	}, Options{})
	if !report.HasErrors() || report.Stats.RequiredUnreachableMaps != 1 {
		t.Fatalf("required unreachable was not an error: stats=%+v findings=%+v", report.Stats, report.Findings)
	}
}

func TestExpectedUnreachableMapBecomingReachableWarns(t *testing.T) {
	report := Verify(Snapshot{
		StartMaps: []MapID{"start"},
		Maps:      []Map{{ID: "start"}, {ID: "dead"}},
		Edges:     []Edge{{ID: "oops", From: "start", To: "dead"}},
		MapExpectations: []MapExpectation{{
			Map: "dead", Class: ReachabilityExpectedUnreachable, Reason: "known dead data",
		}},
	}, Options{})
	if report.WarningCount() != 1 {
		t.Fatalf("warnings = %d, want manifest-drift warning: %+v", report.WarningCount(), report.Findings)
	}
	if report.Findings[0].Code != "expected_unreachable_map_reachable" {
		t.Fatalf("warning code = %q, want expected_unreachable_map_reachable", report.Findings[0].Code)
	}
}
