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
	for _, o := range Offer(obs, known) {
		if o.Kind != KindGoTo || o.Place != place {
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

	// Route 3 itself is not reachable before Brock; this test is about the
	// trainers ON Route 3, so model the post-Boulder state where the Pewter
	// exit is genuinely open.
	obs := Observation{
		Map: 0x02, MapName: "PEWTER_CITY", X: 15, Y: 17, PartyCount: 1,
		Party:  []PartyMon{{Level: 10, HP: 30, MaxHP: 30}},
		Badges: []string{state.BadgeBoulder.String()},
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

func TestTrainerLossNormalizesFleeVariantAndIgnoresOrdinaryBlackout(t *testing.T) {
	route3, ok := skill.Place("route 3")
	if !ok {
		t.Fatal("route 3 missing from place table")
	}
	obs := Observation{
		Map: 0x02, MapName: "PEWTER_CITY", X: 15, Y: 17, PartyCount: 1,
		Party:  []PartyMon{{Level: 10, HP: 30, MaxHP: 30}},
		Badges: []string{state.BadgeBoulder.String()},
	}

	trainer := NewKnowledge(nil)
	trainer.SawMap(obs.Map)
	trainer.SawMap(route3.Map)
	fleeObj := Objective{Kind: KindGoTo, Place: "route 3", Flee: true}
	trainer.Failed(fleeObj, fmt.Errorf("agent: %s: %w", fleeObj, skill.ErrTrainerBlackedOut))
	plain, flee := offeredJourneyTo(obs, trainer, "route 3")
	if plain || flee {
		t.Fatalf("trainer loss through fleeing variant = plain:%v flee:%v, want both suppressed", plain, flee)
	}

	ordinary := NewKnowledge(nil)
	ordinary.SawMap(obs.Map)
	ordinary.SawMap(route3.Map)
	plainObj := Objective{Kind: KindGoTo, Place: "route 3"}
	ordinary.Failed(plainObj, fmt.Errorf("agent: %s: %w", plainObj, skill.ErrBlackedOut))
	plain, flee = offeredJourneyTo(obs, ordinary, "route 3")
	if !plain || !flee {
		t.Fatalf("ordinary wild/poison blackout = plain:%v flee:%v, want no trainer gate", plain, flee)
	}
}

func TestTrainerLossGateSurvivesCheckpointMemory(t *testing.T) {
	route3, ok := skill.Place("route 3")
	if !ok {
		t.Fatal("route 3 missing from place table")
	}
	obs := Observation{
		Map: 0x02, MapName: "PEWTER_CITY", X: 15, Y: 17, PartyCount: 1,
		Party:  []PartyMon{{Level: 10, HP: 30, MaxHP: 30}},
		Badges: []string{state.BadgeBoulder.String()},
	}
	known := NewKnowledge(nil)
	known.SawMap(obs.Map)
	known.SawMap(route3.Map)
	failed := Objective{Kind: KindGoTo, Place: "route 3"}
	known.Failed(failed, fmt.Errorf("agent: %s: %w", failed, skill.ErrTrainerBlackedOut))

	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-020-frame-0002222222-route3.state")
	if err := os.WriteFile(statePath, []byte("state-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeMemoryFile(statePath, known, "get stronger before crossing route 3", 0); err != nil {
		t.Fatalf("writeMemoryFile: %v", err)
	}
	resumed := LoadCheckpointMemory(statePath, nil, nil)
	plain, flee := offeredJourneyTo(obs, resumed.Knowledge, "route 3")
	if plain || flee {
		t.Fatalf("resumed trainer-loss gate = plain:%v flee:%v, want both suppressed", plain, flee)
	}
	resumed.Knowledge.Done(Objective{Kind: KindTrain, Level: 12})
	plain, flee = offeredJourneyTo(obs, resumed.Knowledge, "route 3")
	if !plain || !flee {
		t.Fatalf("resumed route after training = plain:%v flee:%v, want both re-enabled", plain, flee)
	}
}
