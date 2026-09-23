package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestCinnabarSecretKeyUsesKnownMansionEntranceWarp(t *testing.T) {
	e := cinnabarToMansionWarp
	if e.Kind != world.EdgeWarp {
		t.Fatalf("Mansion entry kind = %v, want warp", e.Kind)
	}
	if e.From != cinnabarIslandMap || e.To != pokemonMansion1FMap {
		t.Fatalf("Mansion entry edge = %02x->%02x, want %02x->%02x",
			e.From, e.To, cinnabarIslandMap, pokemonMansion1FMap)
	}
	if e.WarpX != 6 || e.WarpY != 3 {
		t.Fatalf("Mansion entry warp = (%d,%d), want Cinnabar door (6,3)", e.WarpX, e.WarpY)
	}

	mansion, ok := Place("pokemon mansion")
	if !ok {
		t.Fatal("pokemon mansion place missing")
	}
	if mansion.Map != pokemonMansion1FMap {
		t.Fatalf("pokemon mansion place map = %02x, want Mansion 1F %02x", mansion.Map, pokemonMansion1FMap)
	}
}
