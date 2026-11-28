package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

// Keep the private ROM replay selectors in exact lockstep with capture
// boundaries. A renamed/missing source would otherwise surface only after
// an hours-long campaign when its first authentic fixture is needed.
func TestGSKantoReplayCatalogFollowsVerifiedCheckpointChain(t *testing.T) {
	if len(gsKantoReplaySpecs) != len(gsKantoCheckpointBoundaries) {
		t.Fatalf("replays=%d, checkpoints=%d: one replay per checkpoint, ending at the uncheckpointed Red credits event",
			len(gsKantoReplaySpecs), len(gsKantoCheckpointBoundaries))
	}
	seen := make(map[string]bool)
	for i, replay := range gsKantoReplaySpecs {
		source := gsKantoCheckpointBoundaries[i]
		if replay.name == "" || seen[replay.name] {
			t.Fatalf("empty or duplicate Kanto replay selector %q", replay.name)
		}
		seen[replay.name] = true
		if replay.source != source.directory {
			t.Fatalf("replay %q starts at %q, want %q", replay.name, replay.source, source.directory)
		}
		if i+1 < len(gsKantoCheckpointBoundaries) {
			target := gsKantoCheckpointBoundaries[i+1]
			if replay.goal != target.progress || target.previous != source.progress {
				t.Fatalf("replay %q skips or mismatches adjacent verified progress: source=%+v target=%+v",
					replay.name, source, target)
			}
		} else if replay.name != "red" || replay.goal != gsprofile.ProgressMtSilverRedDefeated ||
			source.progress != gsprofile.ProgressMtSilverOpened {
			// Red sets his disappearance event while the ending is scripted.
			// It has a pre-fight checkpoint but no post-fight controllable
			// state to capture via the ordinary checkpoint hook.
			t.Fatalf("last replay must prove Red from Oak's actual permit: %+v source=%+v", replay, source)
		}
		gotSource, gotGoal, ok := gsKantoReplaySelection(replay.name)
		if !ok || gotSource != source || gotGoal != replay.goal {
			t.Fatalf("replay %q selected %+v / %s, found=%t", replay.name, gotSource, gotGoal, ok)
		}
	}
	if _, _, ok := gsKantoReplaySelection("unknown"); ok {
		t.Fatal("unknown Kanto replay selector must fail closed")
	}
}
