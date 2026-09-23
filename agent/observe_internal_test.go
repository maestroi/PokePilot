package agent

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
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

	// The Old Amber sits in the east wing. From the east door it has a free
	// side (measured: (17,2) and (16,3) are floor; the west-side (15,2) is
	// scientist2's STAY home tile and not a valid standing spot). This
	// establishes the amber is offerable at all, so the split assertion below
	// is about the glass divider, not geometry.
	const eastDoorX, eastDoorY = uint8(17), uint8(7)
	if !reachableOnFoot(romData, museum1F, eastDoorX, eastDoorY, oldAmberX, oldAmberY) {
		t.Fatal("Old Amber at (16,2) reported no walkable tile beside it from the east door; test assumption is wrong")
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

// TestObservedPersonsRespectStationaryBlockers pins the Mt. Moon Pokemon
// Center clipboard (map 0x44). The clipboard at (7,2) has exactly one free
// side, (7,3), which is the home tile of a STAY gentleman who never leaves it.
// The static collision grid sees (7,3) as floor, so the old filter offered
// "talk at (7,2)" — an objective no walk can complete — and runs looped on
// "object 5 did not remain adjacent after 4 approaches" (run-ed01c5vw33n4).
// The filter must treat STAY home tiles as permanently occupied.
func TestObservedPersonsRespectStationaryBlockers(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	const mtMoonCenter = 0x44
	clipboardX, clipboardY := uint8(7), uint8(2)
	gentlemanX, gentlemanY := uint8(7), uint8(3)
	px, py := uint8(6), uint8(3)

	// Test assumptions: the raw collision grid has a free side on the
	// clipboard (the gentleman's tile), so the only thing making it
	// unreachable is the stationary blocker, not geometry — and the gentleman
	// really is a STAY object.
	g := mapObjectReachabilityGrid(romData, mtMoonCenter)
	if g == nil {
		t.Fatal("could not build the Mt. Moon center grid")
	}
	if !g.Walkable(int(gentlemanX), int(gentlemanY)) {
		t.Fatalf("assumption wrong: (%d,%d) is not floor in the collision grid", gentlemanX, gentlemanY)
	}
	if !stationaryHomeTiles(romData, mtMoonCenter)[[2]int{int(gentlemanX), int(gentlemanY)}] {
		t.Fatalf("assumption wrong: the gentleman at (%d,%d) is not a STAY object", gentlemanX, gentlemanY)
	}

	// The clipboard's only free side is the gentleman's home tile, so it can
	// never be approached.
	if personReachable(romData, mtMoonCenter, px, py, clipboardX, clipboardY) {
		t.Error("clipboard at (7,2) reported reachable; its only free side is a stationary gentleman")
	}
	// The gentleman himself is reachable from the same position.
	if !personReachable(romData, mtMoonCenter, px, py, gentlemanX, gentlemanY) {
		t.Error("gentleman at (7,3) reported unreachable from (6,3)")
	}
}


func TestMapObjectReachabilityUsesLiveBlockReplacement(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const mapID = uint8(0xA5) // Pokemon Mansion 1F: scripts replace blocks here.
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		t.Fatal(err)
	}
	static, err := world.Build(romData, h)
	if err != nil {
		t.Fatal(err)
	}

	// Find one legal replacement block that changes collision, then project it
	// into the same bordered wOverworldMap buffer scripts mutate at runtime.
	var replacement byte
	var expected *world.Grid
	found := false
	for candidate := 0; candidate <= 0xff; candidate++ {
		if byte(candidate) == blocks[0] {
			continue
		}
		changed := append([]byte(nil), blocks...)
		changed[0] = byte(candidate)
		g, err := world.BuildFromBlocks(romData, h, changed)
		if err != nil {
			continue
		}
		diff := false
		for y := 0; y < static.Height && !diff; y++ {
			for x := 0; x < static.Width; x++ {
				if static.Walkable(x, y) != g.Walkable(x, y) {
					diff = true
					break
				}
			}
		}
		if diff {
			replacement, expected, found = byte(candidate), g, true
			break
		}
	}
	if !found {
		t.Fatal("could not find a Mansion block replacement that changes walkability")
	}

	var mem state.Mem
	mem[sym.CurMap] = mapID
	mem[sym.CurMapWidth] = h.WidthBlocks
	mem[sym.CurMapHeight] = h.HeightBlocks
	const border = 3
	stride := int(h.WidthBlocks) + 2*border
	first := border*stride + border
	for y := 0; y < int(h.HeightBlocks); y++ {
		for x := 0; x < int(h.WidthBlocks); x++ {
			off := first + y*stride + x
			mem[sym.OverworldMap+uint16(off)] = blocks[y*int(h.WidthBlocks)+x]
		}
	}
	mem[sym.OverworldMap+uint16(first)] = replacement

	got := mapObjectReachabilityGridLive(romData, mapID, &mem)
	if got == nil {
		t.Fatal("live object reachability grid is nil")
	}
	changed := false
	for y := 0; y < got.Height; y++ {
		for x := 0; x < got.Width; x++ {
			if got.Walkable(x, y) != static.Walkable(x, y) {
				changed = true
			}
			if got.Walkable(x, y) != expected.Walkable(x, y) {
				t.Fatalf("live grid walkability at (%d,%d) = %v, want replacement geometry %v",
					x, y, got.Walkable(x, y), expected.Walkable(x, y))
			}
		}
	}
	if !changed {
		t.Fatal("live object reachability silently fell back to static ROM geometry")
	}
}
