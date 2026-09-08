package agent

import "testing"

func TestFuchsiaUsesSemanticProgressionObjectiveAndItemVocabulary(t *testing.T) {
	o := Objective{Kind: KindProgress, Progress: redProgressFuchsiaProgressionComplete}
	if err := o.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := o.String(), "progress fuchsia_progression_complete"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	for name, wantRaw := range map[string]uint8{"hm03": 0xC6, "hm04": 0xC7} {
		id, ok := ItemByName(name)
		if !ok || id != ItemID(name) {
			t.Fatalf("ItemByName(%q) = %q,%v, want %q,true", name, id, ok, name)
		}
		raw, ok := redItemID(id)
		if !ok || raw != wantRaw {
			t.Fatalf("redItemID(%q) = %#02x,%v, want %#02x,true", id, raw, ok, wantRaw)
		}
	}
}

func TestOfferFuchsiaProgressionUntilSemanticPostcondition(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{})
	planner := &redObjectiveAdapter{}
	maps := []uint8{0x95, 0x04, 0x8D, 0x17, 0x18, 0x19, 0x1A, 0xB8, 0x07, 0x9A, 0x9B, 0x9C, 0x9D, 0xD9, 0xDA, 0xDB, 0xDC, 0xDE}

	for _, mapID := range maps {
		base := Observation{Map: mapID, PartyCount: 1, Party: []PartyMon{{Level: 30, HP: 80, MaxHP: 80}}}

		if got := countProgress(OfferWithProgression(base, known, planner), redProgressFuchsiaProgressionComplete); got != 0 {
			t.Errorf("map %#04x offers Fuchsia progression %d times without Flute, want 0", mapID, got)
		}

		withFlute := base
		withFlute.Story = ProgressState{{ID: redProgressPokeFluteAcquired, Complete: true}}
		if got := countProgress(OfferWithProgression(withFlute, known, planner), redProgressFuchsiaProgressionComplete); got != 1 {
			t.Errorf("map %#04x offers Fuchsia progression %d times with Flute, want 1", mapID, got)
		}

		complete := withFlute
		complete.Story = ProgressState{
			{ID: redProgressPokeFluteAcquired, Complete: true},
			{ID: redProgressFuchsiaProgressionComplete, Complete: true},
		}
		if got := countProgress(OfferWithProgression(complete, known, planner), redProgressFuchsiaProgressionComplete); got != 0 {
			t.Errorf("map %#04x offers Fuchsia progression %d times after semantic postcondition, want 0", mapID, got)
		}
	}
}

func TestOfferFuchsiaProgressionOutsideSlice(t *testing.T) {
	obs := Observation{
		Map:        0x00,
		PartyCount: 1,
		Party:      []PartyMon{{Level: 30, HP: 80, MaxHP: 80}},
		Story:      ProgressState{{ID: redProgressPokeFluteAcquired, Complete: true}},
	}
	if got := countProgress(OfferWithProgression(obs, NewKnowledge(nil), &redObjectiveAdapter{}), redProgressFuchsiaProgressionComplete); got != 0 {
		t.Fatalf("outside slice offers Fuchsia progression %d times, want 0", got)
	}
}
