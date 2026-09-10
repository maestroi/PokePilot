package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestCinnabarSecretKeySemanticHandoff(t *testing.T) {
	var mem state.Mem
	if CinnabarSecretKeyReady(&mem) {
		t.Fatal("Cinnabar Secret Key phase is ready before #34 completion")
	}
	if CinnabarSecretKeyOwned(&mem) {
		t.Fatal("empty bag reports Secret Key owned")
	}

	// #33 handoff: Soul Badge plus Surf + Strength HMs.
	mem[sym.ObtainedBadges] |= 1 << uint(state.BadgeSoul)
	putBag(&mem,
		state.BagItem{ID: hm03SurfItem, Quantity: 1},
		state.BagItem{ID: hm04StrengthItem, Quantity: 1},
	)
	// #34 story + gym completion: Giovanni, president reward, Marsh Badge.
	setTestEvent(&mem, state.Event(0x78d)) // EVENT_GOT_MASTER_BALL
	setTestEvent(&mem, state.Event(0x78f)) // EVENT_BEAT_SILPH_CO_GIOVANNI
	mem[sym.ObtainedBadges] |= 1 << uint(state.BadgeMarsh)
	if !CinnabarSecretKeyReady(&mem) {
		t.Fatal("completed Saffron slice did not make Cinnabar Secret Key phase ready")
	}

	putBag(&mem,
		state.BagItem{ID: hm03SurfItem, Quantity: 1},
		state.BagItem{ID: hm04StrengthItem, Quantity: 1},
		state.BagItem{ID: mansionSecretKeyItem, Quantity: 1},
	)
	if !CinnabarSecretKeyOwned(&mem) {
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
