package agent

import "testing"

func TestRocketHideoutObjectiveStringAndItemVocabulary(t *testing.T) {
	o := Objective{Kind: KindRocketHideout}
	if got, want := o.String(), "clear the Rocket Hideout and get the SILPH SCOPE"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if err := o.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	id, ok := ItemByName("silph scope")
	if !ok || id != 0x48 {
		t.Fatalf("ItemByName(silph scope) = %#02x, %v; want 0x48, true", id, ok)
	}
	name, ok := ItemName(0x48)
	if !ok || name != "silph scope" {
		t.Fatalf("ItemName(0x48) = %q, %v; want silph scope, true", name, ok)
	}
}

func TestOfferRocketHideoutUntilScopeObtained(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{})

	for _, mapID := range []uint8{0x06, 0x85, 0x87, 0xC7, 0xC8, 0xC9, 0xCA} {
		obs := Observation{Map: mapID, PartyCount: 1, Party: []PartyMon{{Level: 30, HP: 80, MaxHP: 80}}}
		got := Offer(obs, known)
		count := 0
		for _, o := range got {
			if o.Kind == KindRocketHideout {
				count++
			}
		}
		if count != 1 {
			t.Errorf("map %#04x offers Rocket Hideout %d times, want 1; offers=%v", mapID, count, got)
		}
	}

	withScope := Observation{
		Map:        0x06,
		PartyCount: 1,
		Party:      []PartyMon{{Level: 30, HP: 80, MaxHP: 80}},
		Bag:        []Item{{Name: "silph scope", Quantity: 1}},
	}
	for _, o := range Offer(withScope, known) {
		if o.Kind == KindRocketHideout {
			t.Fatalf("Rocket Hideout still offered with Silph Scope in bag: %v", o)
		}
	}

	outside := Observation{Map: 0x04, PartyCount: 1, Party: []PartyMon{{Level: 30, HP: 80, MaxHP: 80}}}
	for _, o := range Offer(outside, known) {
		if o.Kind == KindRocketHideout {
			t.Fatalf("Rocket Hideout offered outside Celadon/Hideout slice: %v", o)
		}
	}
}
