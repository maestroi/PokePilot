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
