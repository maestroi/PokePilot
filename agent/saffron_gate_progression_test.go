package agent

import "testing"

func offeredProgressID(out []Objective, want ProgressID) bool {
	for _, o := range out {
		if o.Kind == KindProgress && o.Progress == want {
			return true
		}
	}
	return false
}

func TestOfferSaffronGateAfterFuchsiaCompletion(t *testing.T) {
	obs := Observation{
		Map:        0x07,
		PartyCount: 1,
		Story: ProgressState{
			{ID: redProgressFuchsiaProgressionComplete, Complete: true},
			{ID: ProgressSaffronGateOpen, Complete: false},
		},
	}
	if got := redProgressionObjectives(obs); !offeredProgressID(got, ProgressSaffronGateOpen) {
		t.Fatalf("Saffron gate objective missing after #33 completion: %v", got)
	}
}

func TestOfferSaffronGateWaitsForFuchsiaAndStopsWhenOpen(t *testing.T) {
	before := Observation{Map: 0x07, PartyCount: 1}
	if got := redProgressionObjectives(before); offeredProgressID(got, ProgressSaffronGateOpen) {
		t.Fatalf("Saffron gate offered before #33 completion: %v", got)
	}

	after := before
	after.Story = ProgressState{
		{ID: redProgressFuchsiaProgressionComplete, Complete: true},
		{ID: ProgressSaffronGateOpen, Complete: true},
	}
	if got := redProgressionObjectives(after); offeredProgressID(got, ProgressSaffronGateOpen) {
		t.Fatalf("already-open Saffron gate was re-offered: %v", got)
	}
}

func TestRedProgressionAcceptsSaffronGateFact(t *testing.T) {
	if !redProgressionKnown(ProgressSaffronGateOpen) {
		t.Fatal("ProgressSaffronGateOpen is not registered as executable Red progression")
	}
}
