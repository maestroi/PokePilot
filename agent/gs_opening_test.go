package agent

import (
	"bytes"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

func TestGSErrandScriptOwnershipIsNarrow(t *testing.T) {
	for _, name := range []string{"ELMS_LAB", "MR_POKEMONS_HOUSE", "ROUTE_30", "CHERRYGROVE_CITY"} {
		id, err := gsOpeningMapID(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !gsErrandScriptMap(id) {
			t.Fatalf("%s (%#04x) is not owned by the errand script driver", name, id)
		}
	}
	route29, err := gsOpeningMapID("ROUTE_29")
	if err != nil {
		t.Fatal(err)
	}
	if gsErrandScriptMap(route29) {
		t.Fatal("Route 29 ordinary scripts should not be owned by the errand dialogue driver")
	}
}

func TestGoldPostStarterErrandRealROM(t *testing.T) {
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("POKEMON_GOLD_ROM not set")
	}
	e, err := emu.Open(path)
	if err != nil {
		t.Fatalf("emu.Open Gold: %v", err)
	}
	t.Cleanup(func() { e.Close() })

	if _, err := skill.BootToOverworld(e); err != nil {
		t.Fatalf("BootToOverworld: %v", err)
	}
	if err := executeGSOpening(e, e.ROM(), skill.StarterCyndaquil); err != nil {
		t.Fatalf("executeGSOpening: %v", err)
	}
	if err := executeGSPostStarterErrand(e, e.ROM()); err != nil {
		t.Fatalf("executeGSPostStarterErrand: %v", err)
	}

	facts := gsprofile.NewGold().DecodeOpening(e)
	// RivalNamed is not asserted: it decodes from Elm's lab scene being the
	// noop scene, and handing back the egg advances that scene to the aide's.
	if !facts.GaveMysteryEggToElm || !facts.CherrygroveRivalResolved ||
		!facts.HasPokedex || !facts.Controllable {
		t.Fatalf("final opening facts = %+v, want stable completed Elm errand", facts)
	}
}

// TestGoldStarterKeepsSpeciesName pins the nickname prompt: GivePoke defaults
// "Give a nickname?" to YES, and the opening's dialogue A used to accept it,
// type 'A' into the keyboard and confirm a starter named "AAAAAAAAAA".
func TestGoldStarterKeepsSpeciesName(t *testing.T) {
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("POKEMON_GOLD_ROM not set")
	}
	e, err := emu.Open(path)
	if err != nil {
		t.Fatalf("emu.Open Gold: %v", err)
	}
	t.Cleanup(func() { e.Close() })

	if _, err := skill.BootToOverworld(e); err != nil {
		t.Fatalf("BootToOverworld: %v", err)
	}
	if err := executeGSOpening(e, e.ROM(), skill.StarterChikorita); err != nil {
		t.Fatalf("executeGSOpening: %v", err)
	}
	// wPartyMonNicknames: "CHIKORITA" then terminators.
	want := []byte{0x82, 0x87, 0x88, 0x8a, 0x8e, 0x91, 0x88, 0x93, 0x80, 0x50}
	got := make([]byte, len(want))
	e.PeekInto(0xdb8c, got)
	if !bytes.Equal(got, want) {
		t.Fatalf("starter nickname = % x, want CHIKORITA % x", got, want)
	}
}
