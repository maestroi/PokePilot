package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestCatchAcquiredWantedAcceptsPartyBoxOrNewDexBit(t *testing.T) {
	want := []uint8{0x24}
	wantDex := []uint8{16}

	if species, ok := catchAcquiredWanted(1, state.PartyState{Count: 2, Mons: []state.Mon{{Species: 0x99}, {Species: 0x24}}}, 0, state.BoxState{}, nil, nil, want, wantDex); !ok || species != 0x24 {
		t.Fatalf("party growth = species %#02x ok=%v, want caught 0x24", species, ok)
	}
	if species, ok := catchAcquiredWanted(6, state.PartyState{Count: 6}, 2, state.BoxState{Count: 3, Mons: []state.BoxMon{{}, {}, {Species: 0x24}}}, nil, nil, want, wantDex); !ok || species != 0x24 {
		t.Fatalf("box growth = species %#02x ok=%v, want caught 0x24", species, ok)
	}
	if _, ok := catchAcquiredWanted(6, state.PartyState{Count: 6}, 20, state.BoxState{Count: 20}, nil, []uint8{16}, want, wantDex); !ok {
		t.Fatal("new Pokédex owned bit must count as a catch when party and box cannot grow")
	}
	if _, ok := catchAcquiredWanted(1, state.PartyState{Count: 1, Mons: []state.Mon{{Species: 0x99}}}, 0, state.BoxState{}, []uint8{16}, []uint8{16}, want, wantDex); ok {
		t.Fatal("already-owned bit without party/box growth must not count as a new catch")
	}
	if _, ok := catchAcquiredWanted(1, state.PartyState{Count: 1}, 0, state.BoxState{}, nil, nil, want, wantDex); ok {
		t.Fatal("no evidence of acquisition must not count as a catch")
	}
}
