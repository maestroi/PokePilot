package rom

import (
	"os"
	"testing"
)

func TestSpecialInteractionRole(t *testing.T) {
	cases := []struct {
		script byte
		role   ObjectInteractionRole
	}{
		{textScriptPokemonCenterNurse, InteractionPokemonCenterNurse},
		{textScriptMart, InteractionMart},
		{textScriptBillsPC, InteractionBillsPC},
		{textScriptPlayersPC, InteractionPlayersPC},
		{textScriptPokemonCenterPC, InteractionPokemonCenterPC},
		{textScriptPrizeVendor, InteractionPrizeVendor},
		{textScriptCableClub, InteractionCableClub},
		{textScriptVendingMachine, InteractionVendingMachine},
	}
	for _, tc := range cases {
		got, ok := specialInteractionRole(tc.script)
		if !ok || got != tc.role {
			t.Fatalf("specialInteractionRole(%#02x) = %q, %v; want %q, true", tc.script, got, ok, tc.role)
		}
	}
	if got, ok := specialInteractionRole(0x08); ok || got != "" {
		t.Fatalf("ordinary text_asm classified as special: role=%q ok=%v", got, ok)
	}
}

func TestSpecialInteractionActorsViridianPokecenter(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}

	actors, err := SpecialInteractionActors(romData, 0x29) // VIRIDIAN_POKECENTER
	if err != nil {
		t.Fatalf("SpecialInteractionActors: %v", err)
	}
	got := map[[2]uint8]ObjectInteractionRole{}
	for _, actor := range actors {
		got[[2]uint8{actor.X, actor.Y}] = actor.Role
	}
	if role := got[[2]uint8{3, 1}]; role != InteractionPokemonCenterNurse {
		t.Fatalf("nurse role = %q, want %q; actors=%+v", role, InteractionPokemonCenterNurse, actors)
	}
	if role := got[[2]uint8{11, 2}]; role != InteractionCableClub {
		t.Fatalf("link receptionist role = %q, want %q; actors=%+v", role, InteractionCableClub, actors)
	}
	if role := got[[2]uint8{10, 5}]; role != "" {
		t.Fatalf("ordinary gentleman was classified as service role %q", role)
	}
	if role := got[[2]uint8{4, 3}]; role != "" {
		t.Fatalf("ordinary cooltrainer was classified as service role %q", role)
	}
}
