package agent

import (
	"reflect"
	"testing"
)

func TestDefaultObjectiveProviderOrderIsExplicitAndStable(t *testing.T) {
	want := []ObjectiveFamily{
		ObjectiveFamilyProgression,
		ObjectiveFamilyCollection,
		ObjectiveFamilyRecovery,
		ObjectiveFamilyTraining,
		ObjectiveFamilyEconomy,
		ObjectiveFamilyExploration,
		ObjectiveFamilyTravel,
	}
	if len(defaultObjectiveProviders) != len(want) {
		t.Fatalf("provider count = %d, want %d", len(defaultObjectiveProviders), len(want))
	}
	for i, provider := range defaultObjectiveProviders {
		if got := provider.Family(); got != want[i] {
			t.Fatalf("provider %d family = %q, want %q", i, got, want[i])
		}
	}
}

func TestObjectiveProvidersReturnStructuredBlockEvidence(t *testing.T) {
	obs := Observation{HasGrass: true}
	ctx := newObjectiveOfferContext(obs, NewKnowledge(nil))
	got := (wildCollectionProvider{}).Provide(ctx)
	if len(got.Candidates) != 0 {
		t.Fatalf("collection candidates = %+v, want none without Pokeballs", got.Candidates)
	}
	if len(got.Blocked) != 1 {
		t.Fatalf("blocked evidence = %+v, want one missing-resource record", got.Blocked)
	}
	block := got.Blocked[0]
	if block.Family != ObjectiveFamilyCollection || block.Reason != "missing_resource" || block.Requirement != "pokeball" {
		t.Fatalf("blocked evidence = %+v, want structured collection resource prerequisite", block)
	}
}

func TestOfferWithEvidenceIsDeterministic(t *testing.T) {
	obs := Observation{Map: 0x00, MapName: "PALLET_TOWN", X: 4, Y: 7, PartyCount: 1}
	adjacency := map[uint8][]uint8{0x00: {0x0c}, 0x0c: {0x00}}

	first := OfferWithEvidence(obs, NewKnowledge(adjacency))
	second := OfferWithEvidence(obs, NewKnowledge(adjacency))
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("provider pipeline is not deterministic:\nfirst=%+v\nsecond=%+v", first, second)
	}

	seenTravel := false
	for _, objective := range first.Candidates {
		if objective.Kind == KindGoTo {
			seenTravel = true
			continue
		}
		if seenTravel {
			t.Fatalf("non-travel objective %q appeared after travel objectives", objective)
		}
	}
}
