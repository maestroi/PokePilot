package skill

import (
	"os"
	"testing"
)

// TestRockTunnelGrindPairSkipsExitWarp pins the shared hunt invariant exposed
// by run-d6dokr184ky81: indoor encounter cells include active warp tiles, but
// GoTo refuses those destinations via warpAvoidance. Blocking warps the same
// way sprites are blocked makes Catch/Train pick free floor beside the
// Route 10 exit instead of the door itself.
func TestRockTunnelGrindPairSkipsExitWarp(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read POKEMON_RED_ROM: %v", err)
	}

	const rockTunnel1F = uint8(0x52)
	h, err := routingHeaderForROM(romData, rockTunnel1F)
	if err != nil {
		t.Fatalf("routingHeaderForROM: %v", err)
	}
	blocked := activeWarpBlockers(h)
	if !blocked[[2]int{15, 3}] {
		t.Fatal("activeWarpBlockers missed Rock Tunnel 1F exit warp (15,3)")
	}

	grass, grid, err := grassCells(romData, rockTunnel1F)
	if err != nil {
		t.Fatalf("grassCells: %v", err)
	}
	grass = grassInPlayerComponent(grass, grid, 15, 4)
	a, b, ok := grindPair(grass, grid, 15, 4, blocked)
	if !ok {
		t.Fatal("grindPair found no free pair beside Rock Tunnel exit")
	}
	if (a.x == 15 && a.y == 3) || (b.x == 15 && b.y == 3) {
		t.Fatalf("grindPair chose exit warp: %+v -> %+v", a, b)
	}
	if a != (cell{15, 4}) {
		t.Fatalf("a = %+v, want standing tile (15,4)", a)
	}
	if dist(a, b) != 1 {
		t.Fatalf("pair %+v -> %+v distance %d, want adjacent floor", a, b, dist(a, b))
	}

	// Without warp blockers the deterministic adjacent pick is the door —
	// the exact failure the farm checkpoint reproduced.
	ga, gb, ok := grindPair(grass, grid, 15, 4, nil)
	if !ok || gb != (cell{15, 3}) {
		t.Fatalf("precondition drifted: sprite-blind pair = %+v -> %+v ok=%v, want b=(15,3)", ga, gb, ok)
	}
}
