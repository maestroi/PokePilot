package agent

import "testing"

// Keep the private ROM replay selectors in exact lockstep with capture
// boundaries. A renamed/missing source would otherwise surface only after
// an hours-long campaign when its first authentic fixture is needed.
func TestGSKantoReplayCatalogFollowsVerifiedCheckpointChain(t *testing.T) {
	if len(gsKantoReplaySpecs) != len(gsKantoCheckpointBoundaries)-1 {
		t.Fatalf("replays=%d, checkpoints=%d: one replay per adjacent checkpoint pair required",
			len(gsKantoReplaySpecs), len(gsKantoCheckpointBoundaries))
	}
	seen := make(map[string]bool)
	for i, replay := range gsKantoReplaySpecs {
		source := gsKantoCheckpointBoundaries[i]
		target := gsKantoCheckpointBoundaries[i+1]
		if replay.name == "" || seen[replay.name] {
			t.Fatalf("empty or duplicate Kanto replay selector %q", replay.name)
		}
		seen[replay.name] = true
		if replay.source != source.directory || replay.goal != target.progress ||
			target.previous != source.progress {
			t.Fatalf("replay %q skips or mismatches adjacent verified progress: source=%+v target=%+v",
				replay.name, source, target)
		}
		gotSource, gotGoal, ok := gsKantoReplaySelection(replay.name)
		if !ok || gotSource != source || gotGoal != target.progress {
			t.Fatalf("replay %q selected %+v / %s, found=%t", replay.name, gotSource, gotGoal, ok)
		}
	}
	if _, _, ok := gsKantoReplaySelection("unknown"); ok {
		t.Fatal("unknown Kanto replay selector must fail closed")
	}
}
