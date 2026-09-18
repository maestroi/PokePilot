package main

import (
	"testing"

	redprofile "github.com/maestroi/pokepilot/red/profile"
	verifier "github.com/maestroi/pokepilot/worldverify"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestApplyGen1ReachabilityManifest(t *testing.T) {
	snapshot := verifier.Snapshot{Maps: []verifier.Map{
		{ID: "36"}, // Pewter Gym: required
		{ID: "45"}, // dead COPY
		{ID: "e7"}, // UNUSED_MAP_E7
		{ID: "ef"}, // Trade Center
		{ID: "76"}, // Hall of Fame scripted transfer
		{ID: "30"}, // ordinary side house: no special expectation
	}}
	applyGen1ReachabilityManifest(&snapshot, redprofile.New().ROMParser())

	classes := map[verifier.MapID]verifier.ReachabilityClass{}
	for _, expectation := range snapshot.MapExpectations {
		classes[expectation.Map] = expectation.Class
	}
	want := map[verifier.MapID]verifier.ReachabilityClass{
		"36": verifier.ReachabilityRequired,
		"45": verifier.ReachabilityExpectedUnreachable,
		"e7": verifier.ReachabilityExpectedUnreachable,
		"ef": verifier.ReachabilityOptional,
		"76": verifier.ReachabilityStoryStateDependent,
	}
	for id, class := range want {
		if got := classes[id]; got != class {
			t.Errorf("map %s class = %q, want %q", id, got, class)
		}
	}
	if _, ok := classes["30"]; ok {
		t.Fatalf("ordinary optional content received an invented manifest class: %q", classes["30"])
	}

	labels := map[verifier.MapID]string{}
	for _, m := range snapshot.Maps {
		labels[m.ID] = m.Label
	}
	if labels["36"] != "PEWTER_GYM" || labels["45"] != "CERULEAN_TRASHED_HOUSE_COPY" || labels["ef"] != "TRADE_CENTER" {
		t.Fatalf("Gen-I map labels not projected correctly: %+v", labels)
	}
}

func TestApplyGen1ReachabilityManifestUsesYellowMapVocabulary(t *testing.T) {
	snapshot := verifier.Snapshot{Maps: []verifier.Map{{ID: "f8"}}}
	applyGen1ReachabilityManifest(&snapshot, yellowprofile.New().ROMParser())
	if got := snapshot.Maps[0].Label; got != "SUMMER_BEACH_HOUSE" {
		t.Fatalf("Yellow map F8 label = %q, want SUMMER_BEACH_HOUSE", got)
	}
}
