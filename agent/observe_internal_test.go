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

// TestObservedPersonsAreReachable pins the Pewter Museum split (map 0x0034).
// A glass divider splits the room in two; the west door (LAST_MAP warp 1)
// lands in the half holding the gambler and scientist1, the east door (warp
// 2) lands in the half holding scientist2 and the Old Amber. Offering a
// "talk at" objective for an exhibit behind the wrong door is the same
// guaranteed-failing objective the item filter already rejects — it made
// runs re-pick "talk at (16,2)" eight times in one 12-hour window. The
// counter exception (a nurse or clerk, approached from two tiles away across
// their counter) must still report reachable.
func TestObservedPersonsAreReachable(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	const museum1F = 0x34
	oldAmberX, oldAmberY := uint8(16), uint8(2)
	scientist1X, scientist1Y := uint8(12), uint8(4)
	// (9,3) is open floor in the west wing (measured against the grid: row 3
	// is walkable from x=5 to x=10), on the same side as the west door.
	westX, westY := uint8(9), uint8(3)

	// Entering through the west door cannot reach the east wing's Old Amber,
	// even though it has walkable tiles beside it and is not a counter case.
	if !reachableOnFoot(romData, museum1F, scientist1X, scientist1Y+1, oldAmberX, oldAmberY) {
		t.Fatal("Old Amber at (16,2) reported no walkable tile beside it at all; test assumption is wrong")
	}
	if personReachable(romData, museum1F, westX, westY, oldAmberX, oldAmberY) {
		t.Error("Old Amber at (16,2) reported reachable from the west wing; it is behind the east door")
	}
	// scientist1, in the west wing, is reachable from the same position.
	if !personReachable(romData, museum1F, westX, westY, scientist1X, scientist1Y) {
		t.Error("scientist1 at (12,4) reported unreachable from the west wing")
	}

	// The Viridian Pokemon Center nurse (map 0x29) is a genuine counter
	// case: the tile directly beside her, (3,2), is the non-walkable
	// counter, and (3,3) — two tiles away — is the player's approach tile
	// (pokemon center layout, see skill/goto.go). A player anywhere else in
	// the lobby that can reach (3,3) must still be able to talk to her.
	const viridianCenter = 0x29
	nurseX, nurseY := uint8(3), uint8(1)
	if reachableOnFoot(romData, viridianCenter, nurseX, nurseY+2, nurseX, nurseY) {
		t.Fatal("nurse at (3,1) reported an ordinary walkable tile beside her; test assumption is wrong")
	}
	if !personReachable(romData, viridianCenter, nurseX, nurseY+2, nurseX, nurseY) {
		t.Error("nurse at (3,1) reported unreachable from her own counter approach tile (3,3)")
	}
}
