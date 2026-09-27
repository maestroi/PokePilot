package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func TestCinnabarSecretKeySemanticHandoff(t *testing.T) {
	var empty state.Mem
	if CinnabarSecretKeyReady(&empty) {
		t.Fatal("Cinnabar Secret Key phase is mechanically ready without usable Surf")
	}
	if CinnabarSecretKeyOwned(&empty) {
		t.Fatal("empty bag reports Secret Key owned")
	}

	// The reusable Mansion transaction owns mechanics, not campaign ordering:
	// Soul + a learned Surf move are sufficient to traverse Route 21. The
	// concrete adapter decides whether Silph/Sabrina must happen first.
	mem := fieldTestMem(FieldSurf, true, true, true)
	if !CinnabarSecretKeyReady(mem) {
		t.Fatal("usable Surf did not make the Secret Key transaction ready")
	}

	putBag(mem,
		state.BagItem{ID: hm03SurfItem, Quantity: 1},
		state.BagItem{ID: hm04StrengthItem, Quantity: 1},
		state.BagItem{ID: mansionSecretKeyItem, Quantity: 1},
	)
	if !CinnabarSecretKeyOwned(mem) {
		t.Fatal("Secret Key bag entry did not satisfy semantic postcondition")
	}
}

func TestMansionSwitchSpecsRequireSouthSideFacingUp(t *testing.T) {
	specs := []mansionSwitchSpec{mansion1FSwitch, mansion2FSwitch, mansion3FSwitch}
	specs = append(specs, mansionB1FSwitches...)
	for _, sw := range specs {
		if sw.StandX != sw.TargetX || int(sw.StandY) != int(sw.TargetY)+1 {
			t.Fatalf("switch on map %#04x target=(%d,%d) stand=(%d,%d) is not the required south-side approach",
				sw.Map, sw.TargetX, sw.TargetY, sw.StandX, sw.StandY)
		}
	}
}

func TestMansionStoryRouteConstants(t *testing.T) {
	if mansion1FTo2FWarp.From != pokemonMansion1FMap || mansion1FTo2FWarp.To != pokemonMansion2FMap ||
		mansion1FTo2FWarp.WarpX != 5 || mansion1FTo2FWarp.WarpY != 10 {
		t.Fatalf("1F->2F story warp = %+v", mansion1FTo2FWarp)
	}
	if mansion2FTo3FWarp.From != pokemonMansion2FMap || mansion2FTo3FWarp.To != pokemonMansion3FMap ||
		mansion2FTo3FWarp.WarpX != 7 || mansion2FTo3FWarp.WarpY != 10 {
		t.Fatalf("2F->3F story warp = %+v", mansion2FTo3FWarp)
	}
	if mansion1FToB1FWarp.From != pokemonMansion1FMap || mansion1FToB1FWarp.To != pokemonMansionB1FMap ||
		mansion1FToB1FWarp.WarpX != 21 || mansion1FToB1FWarp.WarpY != 23 {
		t.Fatalf("1F->B1F story warp = %+v", mansion1FToB1FWarp)
	}
	if len(mansionDropHoles) != 2 || mansionDropHoles[0] != [2]uint8{16, 14} || mansionDropHoles[1] != [2]uint8{17, 14} {
		t.Fatalf("1F Mansion drop holes = %v", mansionDropHoles)
	}
}

func TestCinnabarPlacesRegistered(t *testing.T) {
	checks := map[string]uint8{
		"cinnabar island":         cinnabarIslandMap,
		"pokemon mansion":         pokemonMansion1FMap,
		"cinnabar pokemon center": cinnabarPokemonCenterMap,
	}
	for name, wantMap := range checks {
		dest, ok := Place(name)
		if !ok {
			t.Fatalf("place %q is not registered", name)
		}
		if dest.Map != wantMap {
			t.Fatalf("place %q map = %#04x, want %#04x", name, dest.Map, wantMap)
		}
	}
}

// #1647: the specs once held pokered's hidden_event bytes in stored (y,x)
// order, so every "statue" was open floor and A pressed nothing. A real
// switch is a solid statue with a walkable south-side stand.
func TestMansionSwitchSpecsTargetROMStatues(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	specs := append([]mansionSwitchSpec{mansion1FSwitch, mansion2FSwitch, mansion3FSwitch}, mansionB1FSwitches...)
	for _, sw := range specs {
		h, err := rom.ParseMap(romData, sw.Map)
		if err != nil {
			t.Fatalf("parse map %#04x: %v", sw.Map, err)
		}
		g, err := world.Build(romData, h)
		if err != nil {
			t.Fatalf("build map %#04x: %v", sw.Map, err)
		}
		if g.Walkable(int(sw.TargetX), int(sw.TargetY)) {
			t.Errorf("map %#04x switch target (%d,%d) is walkable floor, not a statue", sw.Map, sw.TargetX, sw.TargetY)
		}
		if !g.Walkable(int(sw.StandX), int(sw.StandY)) {
			t.Errorf("map %#04x switch stand (%d,%d) is not walkable", sw.Map, sw.StandX, sw.StandY)
		}
	}
}
