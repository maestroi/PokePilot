package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestLearningMonSnapshotPrefersPartyRAMOverStaleBattleCopy(t *testing.T) {
	var mem state.Mem
	mem[sym.PartyCount] = 1
	mem[sym.PlayerMonNumber] = 0
	base := sym.PartyMon1
	mem[base+sym.MonType1] = 0x16
	mem[base+sym.MonType2] = 0x03
	want := [4]uint8{33, 45, 73, 22}
	copy(mem.Slice(base+sym.MonMoves, 4), want[:])

	// Reproduce #479's shape: the live battle copy claims slot 2 is empty,
	// while the party struct that the ROM's learn-move menu operates on is full.
	bs := &state.BattleState{
		ActiveType1: 0x01,
		ActiveType2: 0x02,
		Moves:       [4]state.Move{{ID: 33}, {ID: 45}, {ID: 0}, {ID: 22}},
	}

	got, type1, type2, slot := learningMonSnapshot(&mem, bs)
	if got != want {
		t.Fatalf("moves=%v want party RAM %v", got, want)
	}
	if type1 != 0x16 || type2 != 0x03 || slot != 0 {
		t.Fatalf("types/slot=(%#x,%#x,%d), want (0x16,0x03,0)", type1, type2, slot)
	}
	if id := learningMoveID(&mem, bs, 2); id != 73 {
		t.Fatalf("slot 2 move=%d want 73 from party RAM", id)
	}
}

func TestLearningMonSnapshotFallsBackToBattleWhenPartySlotInvalid(t *testing.T) {
	var mem state.Mem
	mem[sym.PartyCount] = 1
	mem[sym.PlayerMonNumber] = 5
	bs := &state.BattleState{
		ActiveType1: 0x09,
		ActiveType2: 0x0a,
		Moves:       [4]state.Move{{ID: 10}, {ID: 20}, {ID: 30}, {ID: 40}},
	}
	got, type1, type2, _ := learningMonSnapshot(&mem, bs)
	if got != [4]uint8{10, 20, 30, 40} || type1 != 0x09 || type2 != 0x0a {
		t.Fatalf("fallback=(%v,%#x,%#x)", got, type1, type2)
	}
}
