package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestOfferSecretKeyAfterSaffronCompletion(t *testing.T) {
	obs := Observation{
		PartyCount: 1,
		Badges:     []string{state.BadgeMarsh.String()},
		Story: ProgressState{
			{ID: redProgressSilphRescueComplete, Complete: true},
			{ID: ProgressSecretKeyOwned, Complete: false},
		},
	}
	got := redProgressionObjectives(obs)
	if !offeredProgressID(got, ProgressSecretKeyOwned) {
		t.Fatalf("Secret Key objective missing after #34 completion: %v", got)
	}
}

func TestOfferSecretKeyWaitsForMarshAndStopsWhenOwned(t *testing.T) {
	base := Observation{
		PartyCount: 1,
		Story: ProgressState{
			{ID: redProgressSilphRescueComplete, Complete: true},
		},
	}
	if got := redProgressionObjectives(base); offeredProgressID(got, ProgressSecretKeyOwned) {
		t.Fatalf("Secret Key offered before Marsh Badge: %v", got)
	}

	owned := base
	owned.Badges = []string{state.BadgeMarsh.String()}
	owned.Story = ProgressState{
		{ID: redProgressSilphRescueComplete, Complete: true},
		{ID: ProgressSecretKeyOwned, Complete: true},
	}
	if got := redProgressionObjectives(owned); offeredProgressID(got, ProgressSecretKeyOwned) {
		t.Fatalf("owned Secret Key was re-offered: %v", got)
	}
}

func TestRedProgressionAcceptsSecretKeyFact(t *testing.T) {
	if !redProgressionKnown(ProgressSecretKeyOwned) {
		t.Fatal("ProgressSecretKeyOwned is not registered as executable Red progression")
	}
}
