package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestCinnabarGymGateScriptedBlocker mirrors pokered/pokeyellow
// CinnabarIslandDefaultScript: without SECRET_KEY in the bag, tile (18,4) is a
// walkable script trap. Inventory detours that Travel across the island must
// treat it as occupied so they do not loop "The door is locked...".
func TestCinnabarGymGateScriptedBlocker(t *testing.T) {
	gate := [2]int{int(cinnabarGymGateX), int(cinnabarGymGateY)}

	var empty state.Mem
	empty[sym.NumBagItems] = 0
	empty[sym.BagItems] = 0xff
	blocked := gen1ScriptedTileBlockers(cinnabarIslandMap, &empty)
	if !blocked[gate] {
		t.Fatalf("without Secret Key, blockers = %v, want gate %v occupied", blocked, gate)
	}

	var withKey state.Mem
	putBag(&withKey, state.BagItem{ID: mansionSecretKeyItem, Quantity: 1})
	if got := gen1ScriptedTileBlockers(cinnabarIslandMap, &withKey); got[gate] {
		t.Fatalf("with Secret Key in bag, blockers = %v, want gate clear", got)
	}

	// PC ownership alone does not silence IsItemInBag; keep the tile blocked.
	var pcOnly state.Mem
	pcOnly[sym.NumBagItems] = 0
	pcOnly[sym.BagItems] = 0xff
	pcOnly[sym.PCItemCount] = 1
	pcOnly[sym.PCItems], pcOnly[sym.PCItems+1], pcOnly[sym.PCItems+2] = mansionSecretKeyItem, 1, 0xff
	if got := gen1ScriptedTileBlockers(cinnabarIslandMap, &pcOnly); !got[gate] {
		t.Fatalf("Secret Key only in PC, blockers = %v, want gate occupied", got)
	}

	if got := gen1ScriptedTileBlockers(pokemonMansion1FMap, &empty); got != nil {
		t.Fatalf("non-Cinnabar map produced scripted blockers %v", got)
	}
}
