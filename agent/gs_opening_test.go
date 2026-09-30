package agent

import (
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
