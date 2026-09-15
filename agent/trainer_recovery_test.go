package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func offeredJourneyTo(obs Observation, known *Knowledge, place string) (plain, flee bool) {
	for _, o := range OfferWithProgression(obs, known, &redObjectiveAdapter{}) {
		if o.Kind != KindGoTo || o.Place != PlaceID(place) {
			continue
		}
		if o.Flee {
			flee = true
		} else {
			plain = true
		}
	}
	return
}

func TestTrainerLossBlocksSameJourneyUntilTraining(t *testing.T) {
	route2, ok := skill.Place("route 2")
	if !ok {
		t.Fatal("route 2 missing from place table")
	}
	route3, ok := skill.Place("route 3")
	if !ok {
		t.Fatal("route 3 missing from place table")
	}

	// Route 3 itself is not reachable before Brock, and Route 2 is not
	// reachable before Oak's parcel is delivered; this test is about the
	// trainers ON Route 3, so model the state where both scripted exits are
	// genuinely open — a run standing in Pewter with a badge has long since
	// been handed the Pokedex.
	obs := Observation{
		Map: 0x02, MapName: "PEWTER_CITY", X: 15, Y: 17, PartyCount: 1,
		Party:  []PartyMon{{Level: 10, HP: 30, MaxHP: 30}},
		Badges: []string{state.BadgeBoulder.String()},
		Events: []string{state.EventGotPokedex.String()},
	}
	known := NewKnowledge(nil)
	known.SawMap(obs.Map)
	known.SawMap(route2.Map)
	known.SawMap(route3.Map)

	plain, flee := offeredJourneyTo(obs, known, "route 3")
	if !plain || !flee {
		t.Fatalf("fresh route 3 offers = plain:%v flee:%v, want both", plain, flee)
	}

	failed := Objective{Kind: KindGoTo, Place: "route 3"}
	known.Failed(failed, fmt.Errorf("agent: %s: %w", failed, skill.ErrTrainerBlackedOut))

	plain, flee = offeredJourneyTo(obs, known, "route 3")
	if plain || flee {
		t.Fatalf("route 3 after trainer blackout = plain:%v flee:%v, want both suppressed", plain, flee)
	}

	// A different known route remains available: the recovery gate is scoped
	// to the failed logical objective, not a global ban on movement.
	otherPlain, otherFlee := offeredJourneyTo(obs, known, "route 2")
	if !otherPlain || !otherFlee {
		t.Fatalf("unrelated route 2 = plain:%v flee:%v, want both still available", otherPlain, otherFlee)
	}

	// One successful training rung is the material-change boundary.
	known.Done(Objective{Kind: KindTrain, Level: 12})
	plain, flee = offeredJourneyTo(obs, known, "route 3")
	if !plain || !flee {
		t.Fatalf("route 3 after training = plain:%v flee:%v, want both re-enabled", plain, flee)
	}
}

func TestTrainerLossStateSurvivesMemoryRoundTrip(t *testing.T) {
	known := NewKnowledge(nil)
	failed := Objective{Kind: KindGoTo, Place: "route 3"}
	known.Failed(failed, fmt.Errorf("agent: %s: %w", failed, skill.ErrTrainerBlackedOut))

	dir := t.TempDir()
	statePath := filepath.Join(dir, "checkpoint.state")
	if err := os.WriteFile(statePath, []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveCheckpointMemory(statePath, known, "", 0, Plan{}); err != nil {
		t.Fatal(err)
	}
	restored := LoadCheckpointMemory(statePath, nil, nil).Knowledge
	if _, ok := restored.Failures[trainerLossFailureKey(failed)]; !ok {
		t.Fatalf("trainer-loss failure missing after round trip: %+v", restored.Failures)
	}
}
