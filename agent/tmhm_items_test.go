package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

func TestTMHMItemVocabularyCoversEveryMachine(t *testing.T) {
	for i := 0; i < rom.NumHMs; i++ {
		id := rom.HM01Item + uint8(i)
		name, ok := ItemName(id)
		if !ok {
			t.Fatalf("HM item %#02x missing from ItemName", id)
		}
		if got, ok := ItemByName(name); !ok || got != id {
			t.Fatalf("ItemByName(%q) = %#02x,%v, want %#02x,true", name, got, ok, id)
		}
	}
	for i := 0; i < rom.NumTMs; i++ {
		id := rom.TM01Item + uint8(i)
		name, ok := ItemName(id)
		if !ok {
			t.Fatalf("TM item %#02x missing from ItemName", id)
		}
		if got, ok := ItemByName(name); !ok || got != id {
			t.Fatalf("ItemByName(%q) = %#02x,%v, want %#02x,true", name, got, ok, id)
		}
	}
}

func TestTMHMObjectiveUsesExistingUseItemContract(t *testing.T) {
	o := Objective{Kind: KindUseItem, Item: rom.TM01Item, Slot: 2}
	if err := o.Validate(); err != nil {
		t.Fatalf("TM objective failed validation: %v", err)
	}
	if got, want := o.String(), "use a TM01 on party slot 2"; got != want {
		t.Fatalf("TM objective String() = %q, want %q", got, want)
	}

	o = Objective{Kind: KindUseItem, Item: rom.HM05Item, Slot: 0}
	if err := o.Validate(); err != nil {
		t.Fatalf("HM objective failed validation: %v", err)
	}
	if got, want := o.String(), "use a HM05 on party slot 0"; got != want {
		t.Fatalf("HM objective String() = %q, want %q", got, want)
	}
}
