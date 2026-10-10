package skill

import (
	"os"
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func setOwnedDex(mem *state.Mem, dex uint8) {
	bit := uint16(dex - 1)
	mem[sym.PokedexOwned+bit/8] |= 1 << (bit % 8)
}

func TestRetiredBagItemsOnlyDeadItems(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var mem state.Mem
	putBag(&mem,
		state.BagItem{ID: ssTicketItem, Quantity: 1},
		state.BagItem{ID: oldRodItem, Quantity: 1},
		state.BagItem{ID: itemfinderItem, Quantity: 1},
		state.BagItem{ID: silphScopeItem, Quantity: 1},
	)
	// Ship still docked, Old Rod catch not owned: only the Itemfinder is dead.
	if got := RetiredBagItems(romData, &mem); !reflect.DeepEqual(got, []uint8{itemfinderItem}) {
		t.Fatalf("retired = %#v, want only the Itemfinder", got)
	}

	setEvent(&mem, state.EventSSAnneLeft)
	encounters, err := rom.FishingEncounters(romData)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range encounters {
		if e.Rod == rom.RodOld {
			dex, err := rom.InternalSpeciesDexNumber(romData, e.Species)
			if err != nil {
				t.Fatal(err)
			}
			setOwnedDex(&mem, dex)
		}
	}
	want := []uint8{ssTicketItem, oldRodItem, itemfinderItem}
	if got := RetiredBagItems(romData, &mem); !reflect.DeepEqual(got, want) {
		t.Fatalf("retired = %#v, want %#v (Silph Scope must stay)", got, want)
	}
}

func TestLearnedHMRetiresLastAndStaysOwnedInPC(t *testing.T) {
	var mem state.Mem
	mem[sym.ObtainedBadges] = 1 << state.BadgeCascade
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonSpecies] = 44 // Oddish: Cut-compatible
	copy(mem.Slice(sym.PartyMon1+sym.MonMoves, 4), []byte{cutMove, 0, 0, 0})
	putBag(&mem,
		state.BagItem{ID: hm01Item, Quantity: 1},
		state.BagItem{ID: fieldHM03Item, Quantity: 1}, // Surf not learned: stays
		state.BagItem{ID: itemfinderItem, Quantity: 1},
	)
	if got := RetiredBagItems(nil, &mem); !reflect.DeepEqual(got, []uint8{itemfinderItem, hm01Item}) {
		t.Fatalf("retired = %#v, want Itemfinder then learned HM01", got)
	}

	// HM01 now only in the PC, Cut forgotten: still owned, not teachable in place.
	putBag(&mem, state.BagItem{ID: fieldHM03Item, Quantity: 1})
	mem[sym.PCItemCount] = 1
	mem[sym.PCItems], mem[sym.PCItems+1], mem[sym.PCItems+2] = hm01Item, 1, 0xff
	mem[sym.PartyMon1+sym.MonMoves] = 0
	cut := FieldCapabilityFor(&mem, FieldCut)
	if !cut.HMOwned || cut.Usable {
		t.Fatalf("cut = %+v, want HM owned via PC and not usable", cut)
	}
	if CanPrepareFieldMove(nil, &mem, FieldCut) {
		t.Fatal("CanPrepareFieldMove true with HM01 only in the PC")
	}
}
