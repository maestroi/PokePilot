package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
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


func TestCinnabarGymGateSitsOnShortestMansionApproach(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("ROM required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := rom.ParseMap(data, cinnabarIslandMap)
	if err != nil {
		t.Fatal(err)
	}
	g, err := world.Build(data, h)
	if err != nil {
		t.Fatal(err)
	}

	stage := places["cinnabar island"]
	gate := [2]int{int(cinnabarGymGateX), int(cinnabarGymGateY)}
	unrestricted, err := world.FindPath(g, int(stage.X), int(stage.Y), int(cinnabarToMansionWarp.WarpX), int(cinnabarToMansionWarp.WarpY), nil)
	if err != nil {
		t.Fatalf("unrestricted path to Mansion door: %v", err)
	}
	x, y := int(stage.X), int(stage.Y)
	touchesGate := false
	for _, step := range unrestricted {
		x += step.DX
		y += step.DY
		if x == gate[0] && y == gate[1] {
			touchesGate = true
			break
		}
	}
	if !touchesGate {
		t.Fatalf("shortest Mansion approach no longer crosses scripted Gym gate tile (%d,%d)", gate[0], gate[1])
	}

	avoided, err := world.FindPath(g, int(stage.X), int(stage.Y), int(cinnabarToMansionWarp.WarpX), int(cinnabarToMansionWarp.WarpY), map[[2]int]bool{gate: true})
	if err != nil {
		t.Fatalf("path to Mansion door with Gym gate avoided: %v", err)
	}
	if len(avoided) > len(unrestricted) {
		t.Fatalf("avoiding Gym gate makes Mansion approach longer: %d vs %d steps", len(avoided), len(unrestricted))
	}
}
