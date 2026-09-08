package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

func TestTMHMItemVocabularyCoversEveryMachine(t *testing.T) {
	for i := 0; i < rom.NumHMs; i++ {
		raw := rom.HM01Item + uint8(i)
		name, ok := ItemName(raw)
		if !ok {
			t.Fatalf("HM item %#02x missing from ItemName", raw)
		}
		got, ok := ItemByName(name)
		if !ok || got != ItemID(name) {
			t.Fatalf("ItemByName(%q) = %q,%v, want %q,true", name, got, ok, name)
		}
		if roundTrip, ok := redItemID(got); !ok || roundTrip != raw {
			t.Fatalf("redItemID(%q) = %#02x,%v, want %#02x,true", got, roundTrip, ok, raw)
		}
	}
	for i := 0; i < rom.NumTMs; i++ {
		raw := rom.TM01Item + uint8(i)
		name, ok := ItemName(raw)
		if !ok {
			t.Fatalf("TM item %#02x missing from ItemName", raw)
		}
		got, ok := ItemByName(name)
		if !ok || got != ItemID(name) {
			t.Fatalf("ItemByName(%q) = %q,%v, want %q,true", name, got, ok, name)
		}
		if roundTrip, ok := redItemID(got); !ok || roundTrip != raw {
			t.Fatalf("redItemID(%q) = %#02x,%v, want %#02x,true", got, roundTrip, ok, raw)
		}
	}
}

func TestTMHMObjectiveUsesExistingUseItemContract(t *testing.T) {
	o := Objective{Kind: KindUseItem, Item: ItemID("tm01"), Slot: 2}
	if err := o.Validate(); err != nil {
		t.Fatalf("TM objective failed validation: %v", err)
	}
	if got, want := o.String(), "use a TM01 on party slot 2"; got != want {
		t.Fatalf("TM objective String() = %q, want %q", got, want)
	}

	o = Objective{Kind: KindUseItem, Item: ItemID("hm05"), Slot: 0}
	if err := o.Validate(); err != nil {
		t.Fatalf("HM objective failed validation: %v", err)
	}
	if got, want := o.String(), "use a HM05 on party slot 0"; got != want {
		t.Fatalf("HM objective String() = %q, want %q", got, want)
	}
}
