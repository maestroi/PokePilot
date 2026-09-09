package skill

import (
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
	"os"
	"testing"
)

func TestWarpTargetPreservesDestinationLanding(t *testing.T) {
	p := os.Getenv("POKEMON_RED_ROM")
	if p == "" {
		t.Skip("ROM required")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := rom.ParseMap(data, 0x3d)
	if err != nil {
		t.Fatal(err)
	}
	g, err := world.Build(data, h)
	if err != nil {
		t.Fatal(err)
	}
	e := world.Edge{Kind: world.EdgeWarp, From: 0x3d, To: 0x3c, WarpX: 5, WarpY: 7}
	x, y, _, _, err := warpTarget(h, e, g, 21, 17, nil, data)
	if err != nil {
		t.Fatal(err)
	}
	if x != 5 || y != 7 {
		t.Fatalf("exit ladder replaced by (%d,%d)", x, y)
	}
}

func TestWarpTargetRoutesAroundOtherWarps(t *testing.T) {
	p := os.Getenv("POKEMON_RED_ROM")
	if p == "" {
		t.Skip("ROM required")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := rom.ParseMap(data, 0x3b)
	if err != nil {
		t.Fatal(err)
	}
	g, err := world.Build(data, h)
	if err != nil {
		t.Fatal(err)
	}
	e := world.Edge{Kind: world.EdgeWarp, From: 0x3b, To: 0x3c, WarpX: 5, WarpY: 5}
	_, _, steps, _, err := warpTarget(h, e, g, 14, 35, nil, data)
	if err != nil {
		t.Fatal(err)
	}
	warps := map[[2]int]bool{}
	for _, w := range h.Warps {
		warps[[2]int{int(w.X), int(w.Y)}] = true
	}
	if routeCrossesWarp(steps, 14, 35, warps) {
		t.Fatal("approach crosses another warp")
	}
}
