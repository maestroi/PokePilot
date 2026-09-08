package agent

import "testing"

func TestRocketHideoutUsesSemanticProgressionObjective(t *testing.T) {
	o := Objective{Kind: KindProgress, Progress: redProgressSilphScopeAcquired}
	if got, want := o.String(), "progress silph_scope_acquired"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if err := o.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	id, ok := ItemByName("silph scope")
	if !ok || id != ItemID("silph scope") {
		t.Fatalf("ItemByName(silph scope) = %q, %v; want silph scope, true", id, ok)
	}
	raw, ok := redItemID(id)
	if !ok || raw != 0x48 {
		t.Fatalf("redItemID(silph scope) = %#02x, %v; want 0x48, true", raw, ok)
	}
}

func TestOfferRocketHideoutProgressionUntilScopeObtained(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{})
	planner := &redObjectiveAdapter{}

	for _, mapID := range []uint8{0x06, 0x85, 0x87, 0xC7, 0xC8, 0xC9, 0xCA} {
		obs := Observation{Map: mapID, PartyCount: 1, Party: []PartyMon{{Level: 30, HP: 80, MaxHP: 80}}}
		got := OfferWithProgression(obs, known, planner)
		count := 0
		for _, o := range got {
			if o.Kind == KindProgress && o.Progress == redProgressSilphScopeAcquired {
				count++
			}
		}
		if count != 1 {
			t.Errorf("map %#04x offers Silph Scope progression %d times, want 1; offers=%v", mapID, count, got)
		}
	}

	withScope := Observation{
		Map:        0x06,
		PartyCount: 1,
		Party:      []PartyMon{{Level: 30, HP: 80, MaxHP: 80}},
		Story:      ProgressState{{ID: redProgressSilphScopeAcquired, Complete: true}},
	}
	for _, o := range OfferWithProgression(withScope, known, planner) {
		if o.Kind == KindProgress && o.Progress == redProgressSilphScopeAcquired {
			t.Fatalf("Silph Scope progression still offered after fact is complete: %v", o)
		}
	}

	outside := Observation{Map: 0x04, PartyCount: 1, Party: []PartyMon{{Level: 30, HP: 80, MaxHP: 80}}}
	for _, o := range OfferWithProgression(outside, known, planner) {
		if o.Kind == KindProgress && o.Progress == redProgressSilphScopeAcquired {
			t.Fatalf("Silph Scope progression offered outside Celadon/Hideout slice: %v", o)
		}
	}
}
