package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeBattleResourcesProjectsRosterAndBag(t *testing.T) {
	var mem fakeMemory
	mem[sym.IsInBattle] = 1
	mem[sym.PlayerMonNumber] = 1
	mem[sym.PartyCount] = 2

	base0 := sym.PartyMon1
	mem[base0+sym.MonSpecies] = 1
	mem[base0+sym.MonLevel] = 12
	mem[base0+sym.MonHP+1] = 20
	mem[base0+sym.MonMaxHP+1] = 30
	mem[base0+sym.MonMoves] = 33
	mem[base0+sym.MonPP] = 0xc5

	base1 := sym.PartyMon1 + sym.PartyMonSize
	mem[base1+sym.MonSpecies] = 2
	mem[base1+sym.MonLevel] = 15
	mem[base1+sym.MonHP+1] = 40
	mem[base1+sym.MonMaxHP+1] = 40
	mem[base1+sym.MonSpecial+1] = 77
	mem[base1+sym.MonMoves] = 55
	mem[base1+sym.MonPP] = 9

	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = 0x14
	mem[sym.BagItems+1] = 3

	got := New().DecodeBattleResources(&mem)
	if !got.InBattle || got.ActiveSlot != 1 {
		t.Fatalf("battle=%v active=%d want true,1", got.InBattle, got.ActiveSlot)
	}
	if len(got.Party) != 2 || got.Party[0].Moves[0].PP != 5 {
		t.Fatalf("party projection = %+v", got.Party)
	}
	if got.Party[1].SpecialAttack != 77 || got.Party[1].SpecialDefense != 77 {
		t.Fatalf("Gen-I special projection = %d/%d want 77/77", got.Party[1].SpecialAttack, got.Party[1].SpecialDefense)
	}
	if got.ItemQuantity(0x14) != 3 {
		t.Fatalf("Potion quantity = %d want 3", got.ItemQuantity(0x14))
	}
}
