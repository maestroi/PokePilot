package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestFuchsiaProgressionObjectiveAndItemVocabulary(t *testing.T) {
	o := Objective{Kind: KindFuchsiaProgression}
	if err := o.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := o.String(), "reach Fuchsia, beat Koga, and get HM03 SURF + HM04 STRENGTH"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	for name, want := range map[string]uint8{"hm03": 0xC6, "hm04": 0xC7} {
		id, ok := ItemByName(name)
		if !ok || id != want {
			t.Fatalf("ItemByName(%q) = %#02x,%v, want %#02x,true", name, id, ok, want)
		}
		if got, ok := ItemName(want); !ok || got != name {
			t.Fatalf("ItemName(%#02x) = %q,%v, want %q,true", want, got, ok, name)
		}
	}
}

func TestOfferFuchsiaProgressionUntilAllPostconditions(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{})
	maps := []uint8{0x95, 0x04, 0x8D, 0x17, 0x18, 0x19, 0x1A, 0xB8, 0x07, 0x9A, 0x9B, 0x9C, 0x9D, 0xD9, 0xDA, 0xDB, 0xDC, 0xDE}

	for _, mapID := range maps {
		base := Observation{Map: mapID, PartyCount: 1, Party: []PartyMon{{Level: 30, HP: 80, MaxHP: 80}}}

		withoutFlute := base
		if got := countKind(Offer(withoutFlute, known), KindFuchsiaProgression); got != 0 {
			t.Errorf("map %#04x offers Fuchsia progression %d times without Flute, want 0", mapID, got)
		}

		withFlute := base
		withFlute.Bag = []Item{{Name: "poke flute", Quantity: 1}}
		if got := countKind(Offer(withFlute, known), KindFuchsiaProgression); got != 1 {
			t.Errorf("map %#04x offers Fuchsia progression %d times with Flute, want 1", mapID, got)
		}

		partial := base
		partial.Badges = []string{state.BadgeSoul.String()}
		partial.Bag = []Item{{Name: "poke flute", Quantity: 1}, {Name: "hm03", Quantity: 1}}
		if got := countKind(Offer(partial, known), KindFuchsiaProgression); got != 1 {
			t.Errorf("map %#04x offers Fuchsia progression %d times with Soul+HM03 but no HM04, want 1", mapID, got)
		}

		complete := partial
		complete.Bag = append(append([]Item(nil), partial.Bag...), Item{Name: "hm04", Quantity: 1})
		if got := countKind(Offer(complete, known), KindFuchsiaProgression); got != 0 {
			t.Errorf("map %#04x offers Fuchsia progression %d times after Soul+HM03+HM04, want 0", mapID, got)
		}
	}
}

func TestOfferFuchsiaProgressionOutsideSlice(t *testing.T) {
	obs := Observation{
		Map:        0x00,
		PartyCount: 1,
		Party:      []PartyMon{{Level: 30, HP: 80, MaxHP: 80}},
		Bag:        []Item{{Name: "poke flute", Quantity: 1}},
	}
	if got := countKind(Offer(obs, NewKnowledge(nil)), KindFuchsiaProgression); got != 0 {
		t.Fatalf("outside slice offers Fuchsia progression %d times, want 0", got)
	}
}
