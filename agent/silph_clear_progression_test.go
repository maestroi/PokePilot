package agent

import "testing"

func TestOfferSilphRescueAfterCardKey(t *testing.T) {
	obs := Observation{
		Map:        saffronCityMap,
		PartyCount: 1,
		Story: ProgressState{
			{ID: redProgressFuchsiaProgressionComplete, Complete: true},
			{ID: ProgressSaffronGateOpen, Complete: true},
			{ID: ProgressCardKeyOwned, Complete: true},
			{ID: ProgressSilphCoCleared, Complete: false},
			{ID: redProgressSilphRescueComplete, Complete: false},
		},
	}
	got := redProgressionObjectives(obs)
	if !offeredProgressID(got, redProgressSilphRescueComplete) {
		t.Fatalf("Silph rescue objective missing after Card Key: %v", got)
	}
	if offeredProgressID(got, ProgressCardKeyOwned) {
		t.Fatalf("Card Key was re-offered after ownership: %v", got)
	}
}

func TestSilphRescueRemainsOfferedBetweenGiovanniAndPresident(t *testing.T) {
	obs := Observation{
		Map:        saffronCityMap,
		PartyCount: 1,
		Story: ProgressState{
			{ID: redProgressFuchsiaProgressionComplete, Complete: true},
			{ID: ProgressSaffronGateOpen, Complete: true},
			{ID: ProgressSilphCoCleared, Complete: true},
			{ID: redProgressSilphRescueComplete, Complete: false},
		},
	}
	if got := redProgressionObjectives(obs); !offeredProgressID(got, redProgressSilphRescueComplete) {
		t.Fatalf("Silph rescue disappeared before president reward: %v", got)
	}
}

func TestSilphRescueWaitsForCardKeyAndStopsWhenComplete(t *testing.T) {
	beforeKey := Observation{
		Map:        saffronCityMap,
		PartyCount: 1,
		Story: ProgressState{
			{ID: redProgressFuchsiaProgressionComplete, Complete: true},
			{ID: ProgressSaffronGateOpen, Complete: true},
		},
	}
	if got := redProgressionObjectives(beforeKey); offeredProgressID(got, redProgressSilphRescueComplete) {
		t.Fatalf("Silph rescue offered before Card Key: %v", got)
	}

	complete := beforeKey
	complete.Story = ProgressState{
		{ID: redProgressFuchsiaProgressionComplete, Complete: true},
		{ID: ProgressSaffronGateOpen, Complete: true},
		{ID: ProgressCardKeyOwned, Complete: true},
		{ID: ProgressSilphCoCleared, Complete: true},
		{ID: redProgressSilphRescueComplete, Complete: true},
	}
	if got := redProgressionObjectives(complete); offeredProgressID(got, redProgressSilphRescueComplete) {
		t.Fatalf("completed Silph rescue was re-offered: %v", got)
	}
}

func TestRedProgressionAcceptsSilphRescueFact(t *testing.T) {
	if !redProgressionKnown(redProgressSilphRescueComplete) {
		t.Fatal("silph_rescue_complete is not registered as executable Red progression")
	}
}
