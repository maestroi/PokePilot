package agent

import (
	"errors"
	"strings"
	"testing"
)

func TestTargetedTrainingObjectiveString(t *testing.T) {
	lead := Objective{Kind: KindTrain, Level: 18}
	if got, want := lead.String(), "train the lead to level 18"; got != want {
		t.Fatalf("legacy lead training string = %q, want %q", got, want)
	}

	secondary := Objective{Kind: KindTrain, Level: 14, Species: SpeciesID("pikachu"), Slot: 2}
	if got, want := secondary.String(), "train PIKACHU to level 14"; got != want {
		t.Fatalf("targeted training string = %q, want %q", got, want)
	}
	if err := secondary.Validate(); err != nil {
		t.Fatalf("targeted training objective should validate: %v", err)
	}
}

func TestInsertPartyTrainingObjectivesOffersViableSecondariesBeforeTravel(t *testing.T) {
	obs := Observation{
		HasGrass: true,
		Party: []PartyMon{
			{Species: SpeciesID("wartortle"), Level: 25, HP: 60, MaxHP: 72},
			{Species: SpeciesID("pikachu"), Level: 12, HP: 30, MaxHP: 30},
			{Species: SpeciesID("rattata"), Level: 9, HP: 22, MaxHP: 22},
		},
		WildGrass: []WildSpecies{{Name: "pidgey", MinLevel: 8, MaxLevel: 12, Slots: 10}},
	}
	known := NewKnowledge(map[uint8][]uint8{})
	base := []Objective{
		{Kind: KindHeal},
		{Kind: KindGoTo, Place: PlaceID("route 25")},
	}
	estimator := func(slot, target int) (TrainingEstimate, error) {
		return TrainingEstimate{
			CurrentLevel:        obs.Party[slot].Level,
			TargetLevel:         uint8(target),
			XPRemaining:         200,
			XPPerEncounter:      50,
			EstimatedEncounters: 4,
			SessionBudget:       trainSessionBattleBudget,
			Viability:           TrainingViable,
		}, nil
	}

	got := insertPartyTrainingObjectives(obs, known, base, estimator)
	if len(got) != 4 {
		t.Fatalf("got %d objectives, want 4: %#v", len(got), got)
	}
	want := []string{
		"heal the party",
		"train PIKACHU to level 14",
		"train RATTATA to level 11",
		"go to route 25",
	}
	for i := range want {
		if got[i].String() != want[i] {
			t.Fatalf("objective %d = %q, want %q", i, got[i].String(), want[i])
		}
	}
	if got[1].Slot != 1 || got[2].Slot != 2 {
		t.Fatalf("target slots = %d,%d, want 1,2", got[1].Slot, got[2].Slot)
	}
	if !strings.Contains(got[1].Note, "current lead L25") || !strings.Contains(got[1].Note, "training viable") {
		t.Fatalf("targeted training note missing party/cost context: %q", got[1].Note)
	}
}

func TestInsertPartyTrainingObjectivesFiltersUnsafeAndUnviableMembers(t *testing.T) {
	obs := Observation{
		HasGrass: true,
		Party: []PartyMon{
			{Species: SpeciesID("wartortle"), Level: 25, HP: 60, MaxHP: 72},
			{Species: SpeciesID("pikachu"), Level: 12, HP: 0, MaxHP: 30},
			{Species: SpeciesID("rattata"), Level: 9, HP: 22, MaxHP: 22},
			{Species: SpeciesID("zubat"), Level: 8, HP: 20, MaxHP: 20},
		},
	}
	known := NewKnowledge(map[uint8][]uint8{})
	estimator := func(slot, target int) (TrainingEstimate, error) {
		switch slot {
		case 2:
			return TrainingEstimate{CurrentLevel: 9, TargetLevel: uint8(target), Viability: TrainingOutsideBudget}, nil
		case 3:
			return TrainingEstimate{}, errors.New("no estimate")
		default:
			return TrainingEstimate{Viability: TrainingViable}, nil
		}
	}

	got := insertPartyTrainingObjectives(obs, known, nil, estimator)
	if len(got) != 0 {
		t.Fatalf("unsafe/unviable secondaries should not be offered, got %#v", got)
	}
}

func TestTrainingObjectiveRejectsInvalidPartySlot(t *testing.T) {
	o := Objective{Kind: KindTrain, Level: 12, Species: SpeciesID("pikachu"), Slot: 6}
	if err := o.Validate(); err == nil {
		t.Fatal("expected invalid training party slot to be rejected")
	}
}
