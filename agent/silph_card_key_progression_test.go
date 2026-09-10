package agent

import "testing"

func TestOfferCardKeyAfterSaffronGate(t *testing.T) {
	obs := Observation{
		Map:        saffronCityMap,
		PartyCount: 1,
		Story: ProgressState{
			{ID: redProgressFuchsiaProgressionComplete, Complete: true},
			{ID: ProgressSaffronGateOpen, Complete: true},
			{ID: ProgressCardKeyOwned, Complete: false},
			{ID: ProgressSilphCoCleared, Complete: false},
		},
	}
	got := redProgressionObjectives(obs)
	if !offeredProgressID(got, ProgressCardKeyOwned) {
		t.Fatalf("Card Key objective missing after Saffron opens: %v", got)
	}
	if offeredProgressID(got, ProgressSaffronGateOpen) {
		t.Fatalf("Saffron gate was re-offered after opening: %v", got)
	}
}

func TestOfferCardKeyWaitsForHandoffAndStopsWhenSatisfied(t *testing.T) {
	base := Observation{
		Map:        saffronCityMap,
		PartyCount: 1,
		Story: ProgressState{
			{ID: redProgressFuchsiaProgressionComplete, Complete: true},
		},
	}
	if got := redProgressionObjectives(base); offeredProgressID(got, ProgressCardKeyOwned) {
		t.Fatalf("Card Key offered before Saffron opens: %v", got)
	}

	withKey := base
	withKey.Story = ProgressState{
		{ID: redProgressFuchsiaProgressionComplete, Complete: true},
		{ID: ProgressSaffronGateOpen, Complete: true},
		{ID: ProgressCardKeyOwned, Complete: true},
	}
	if got := redProgressionObjectives(withKey); offeredProgressID(got, ProgressCardKeyOwned) {
		t.Fatalf("owned Card Key was re-offered: %v", got)
	}

	cleared := base
	cleared.Story = ProgressState{
		{ID: redProgressFuchsiaProgressionComplete, Complete: true},
		{ID: ProgressSaffronGateOpen, Complete: true},
		{ID: ProgressSilphCoCleared, Complete: true},
	}
	if got := redProgressionObjectives(cleared); offeredProgressID(got, ProgressCardKeyOwned) {
		t.Fatalf("Card Key offered after Silph was already cleared: %v", got)
	}
}

func TestRedProgressionAcceptsCardKeyFact(t *testing.T) {
	if !redProgressionKnown(ProgressCardKeyOwned) {
		t.Fatal("ProgressCardKeyOwned is not registered as executable Red progression")
	}
}
