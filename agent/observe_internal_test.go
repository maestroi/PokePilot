package agent

import (
	"os"
	"testing"
)

// TestObservedItemsAreReachable pins the Mt. Moon B2F split. Map 0x3D is two
// disconnected halves under one map id: the ladder at (15,27) lands in the
// half holding the HP UP (25,21), and every other ladder lands in the half
// holding the TM01 ball (29,5). Offering the far ball from either side is an
// objective no walk can complete, which is what made runs re-pick "pick up
// TM01" forever. Pure ROM data plus the collision grid: no emulator.
func TestObservedItemsAreReachable(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	const mtMoonB2F = 0x3D
	tm01X, tm01Y := uint8(29), uint8(5)
	hpUpX, hpUpY := uint8(25), uint8(21)

	// Standing where the (15,27) ladder drops the player: TM01 is walled off,
	// the HP UP is not.
	if reachableOnFoot(romData, mtMoonB2F, 15, 27, tm01X, tm01Y) {
		t.Error("TM01 at (29,5) reported reachable from (15,27); it is across the map split")
	}
	if !reachableOnFoot(romData, mtMoonB2F, 15, 27, hpUpX, hpUpY) {
		t.Error("HP UP at (25,21) reported unreachable from (15,27); it is in the same half")
	}

	// The other side of the same map: the mirror image.
	if !reachableOnFoot(romData, mtMoonB2F, 25, 9, tm01X, tm01Y) {
		t.Error("TM01 at (29,5) reported unreachable from the (25,9) ladder")
	}
	if reachableOnFoot(romData, mtMoonB2F, 25, 9, hpUpX, hpUpY) {
		t.Error("HP UP at (25,21) reported reachable from (25,9); it is across the map split")
	}

	// Standing beside a ball is reachable even when the player's own tile is
	// solid, and an unparsable map fails open rather than hiding items.
	if !reachableOnFoot(romData, mtMoonB2F, tm01X, tm01Y+1, tm01X, tm01Y) {
		t.Error("a ball the player is already standing beside reported unreachable")
	}
	if !reachableOnFoot(nil, mtMoonB2F, 15, 27, tm01X, tm01Y) {
		t.Error("missing ROM data should fail open")
	}
}
