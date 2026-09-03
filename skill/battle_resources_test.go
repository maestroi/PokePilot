package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func resourceTestMem(activeHP, activeMax uint16, status uint8) state.Mem {
	var mem state.Mem
	mem[sym.IsInBattle] = 2
	mem[sym.PartyCount] = 1
	mem[sym.PlayerMonNumber] = 0
	base := sym.PartyMon1
	mem[base+sym.MonSpecies] = 1
	putU16BE(&mem, base+sym.MonHP, activeHP)
	putU16BE(&mem, base+sym.MonMaxHP, activeMax)
	mem[base+sym.MonStatus] = status
	mem[base+sym.MonMoves] = 33
	mem[base+sym.MonPP] = 10
	return mem
}

func putU16BE(mem *state.Mem, addr uint16, value uint16) {
	mem[addr] = uint8(value >> 8)
	mem[addr+1] = uint8(value)
}

func putBag(mem *state.Mem, items ...state.BagItem) {
	mem[sym.NumBagItems] = uint8(len(items))
	for i, item := range items {
		off := sym.BagItems + uint16(i)*2
		mem[off] = item.ID
		mem[off+1] = item.Quantity
	}
}

func TestChooseBattleMedicineLeavesHealthyMonAlone(t *testing.T) {
	mem := resourceTestMem(40, 50, 0)
	putBag(&mem, state.BagItem{ID: itemPotion, Quantity: 5})
	if got, ok := chooseBattleMedicine(&mem); ok {
		t.Fatalf("chooseBattleMedicine = %+v, true; want no automatic medicine above one-third HP", got)
	}
}

func TestChooseBattleMedicineUsesSmallestSufficientHeal(t *testing.T) {
	mem := resourceTestMem(15, 50, 0) // missing 35: Potion is too small, Super Potion covers it
	putBag(&mem,
		state.BagItem{ID: itemHyperPotion, Quantity: 1},
		state.BagItem{ID: itemPotion, Quantity: 2},
		state.BagItem{ID: itemSuperPotion, Quantity: 1},
	)
	got, ok := chooseBattleMedicine(&mem)
	if !ok {
		t.Fatal("chooseBattleMedicine = no choice, want a heal")
	}
	if got.Item != itemSuperPotion {
		t.Fatalf("medicine = %#02x, want SUPER POTION %#02x", got.Item, itemSuperPotion)
	}
}

func TestChooseBattleMedicinePrefersSpecificStatusCure(t *testing.T) {
	mem := resourceTestMem(50, 50, 1<<3) // poison bit
	putBag(&mem,
		state.BagItem{ID: itemFullHeal, Quantity: 1},
		state.BagItem{ID: itemAntidote, Quantity: 1},
	)
	got, ok := chooseBattleMedicine(&mem)
	if !ok {
		t.Fatal("chooseBattleMedicine = no choice, want poison cure")
	}
	if got.Item != itemAntidote {
		t.Fatalf("medicine = %#02x, want ANTIDOTE %#02x", got.Item, itemAntidote)
	}
}

func TestChooseBattleMedicineUsesFullRestoreForLowHPAndStatus(t *testing.T) {
	mem := resourceTestMem(10, 60, 1<<6) // low HP and paralysis
	putBag(&mem,
		state.BagItem{ID: itemPotion, Quantity: 1},
		state.BagItem{ID: itemParlyzHeal, Quantity: 1},
		state.BagItem{ID: itemFullRestore, Quantity: 1},
	)
	got, ok := chooseBattleMedicine(&mem)
	if !ok {
		t.Fatal("chooseBattleMedicine = no choice, want combined recovery")
	}
	if got.Item != itemFullRestore {
		t.Fatalf("medicine = %#02x, want FULL RESTORE %#02x", got.Item, itemFullRestore)
	}
}

func TestPPRecoverySlotSkipsFaintedAndExhaustedMons(t *testing.T) {
	mem := resourceTestMem(20, 50, 0)
	mem[sym.PartyCount] = 3

	// Active slot 0 has a move but no PP.
	mem[sym.PartyMon1+sym.MonPP] = 0

	// Slot 1 has PP but is fainted, so it cannot recover the battle.
	base1 := sym.PartyMon1 + sym.PartyMonSize
	mem[base1+sym.MonSpecies] = 2
	putU16BE(&mem, base1+sym.MonMaxHP, 40)
	mem[base1+sym.MonMoves] = 45
	mem[base1+sym.MonPP] = 10

	// Slot 2 is live and has PP.
	base2 := sym.PartyMon1 + 2*sym.PartyMonSize
	mem[base2+sym.MonSpecies] = 3
	putU16BE(&mem, base2+sym.MonHP, 25)
	putU16BE(&mem, base2+sym.MonMaxHP, 40)
	mem[base2+sym.MonMoves] = 52
	mem[base2+sym.MonPP] = 7

	got, ok := ppRecoverySlot(&mem)
	if !ok || got != 2 {
		t.Fatalf("ppRecoverySlot = %d,%v, want 2,true", got, ok)
	}
}

func TestLivePartyCurrentPPRejectsPPUpBitsOnly(t *testing.T) {
	mem := resourceTestMem(20, 50, 0)
	mem[sym.PartyMon1+sym.MonPP] = 0xc0 // PP Up count only, zero current PP
	if livePartyHasCurrentPP(&mem) {
		t.Fatal("livePartyHasCurrentPP = true for raw PP 0xc0, want false")
	}
}
