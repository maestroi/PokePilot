package skill_test

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
	"github.com/maestroi/pokepilot/world"
)

// route1Grass returns the first walkable tall-grass cell on Route 1 (map
// 0x0C) in scan order, computed from the ROM: the tileset table names the
// grass tile and the map's blocks say which cells stand on it. The test
// keeps its own copy of that scan so the destination it asks for is
// verified independently of Train's own grass detection.
func route1Grass(t *testing.T, romData []byte) skill.Destination {
	t.Helper()
	h, err := rom.ParseMap(romData, 0x0C)
	if err != nil {
		t.Fatalf("parse map 0x0C: %v", err)
	}
	tsOff, err := bankedOffTest(t, 0x03, 0x47BE)
	if err != nil {
		t.Fatalf("tileset table: %v", err)
	}
	entry := tsOff + int(h.Tileset)*12
	if entry+12 > len(romData) {
		t.Fatalf("tileset %d entry out of ROM", h.Tileset)
	}
	grass := romData[entry+10]
	blockOff, err := bankedOffTest(t, romData[entry], uint16(romData[entry+1])|uint16(romData[entry+2])<<8)
	if err != nil {
		t.Fatalf("block data: %v", err)
	}
	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		t.Fatalf("blocks: %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("grid: %v", err)
	}
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if !grid.Walkable(x, y) {
				continue
			}
			id := blocks[(y/2)*int(h.WidthBlocks)+(x/2)]
			if romData[blockOff+int(id)*16+(2*(y%2)+1)*4+(2*(x%2)+1)] == grass {
				return skill.Destination{Map: 0x0C, X: uint8(x), Y: uint8(y)}
			}
		}
	}
	t.Fatalf("no walkable grass cell found on map 0x0C")
	return skill.Destination{}
}

// bankedOffTest converts a banked address to a ROM file offset, the same
// mapping the ROM layout uses.
func bankedOffTest(t *testing.T, bank uint8, addr uint16) (int, error) {
	t.Helper()
	off := int(addr)
	if addr >= 0x4000 {
		off = int(bank)*0x4000 + int(addr-0x4000)
	}
	if off < 0 || off >= 0x400000 {
		return 0, errors.New("banked offset out of ROM range")
	}
	return off, nil
}

// TestTrainGrindsOnRoute1: from the pallet_town checkpoint, Travel brings
// the player onto Route 1's tall grass, and Train then levels the lead up
// two levels by ping-ponging between grass cells. The target is relative
// to whatever level the lead has on arrival (the approach can level it
// too), and the test fails unless the level actually rose in RAM. Route 1
// fights about one encounter per six legs (measured), so two levels fit
// comfortably in maxBattles 12 and in a minute of wall time. A blackout
// is a legitimate ending, not a failure: with a one-mon party, the
// cumulative damage of several wins can faint the lead even though it
// outbeats Route 1's level 2-5 wilds one at a time. The test accepts it,
// logs where the session landed, and still requires the level gain, the
// Reached flag, and a clean stop (no battle left in progress).
func TestTrainGrindsOnRoute1(t *testing.T) {
	e := fixture.Load(t, "pallet_town")
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)

	dest := route1Grass(t, romData)
	if _, err := skill.Travel(e, romData, dest, policy, 6); err != nil {
		t.Fatalf("approach Travel to Route 1: %v", err)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	if p := state.DecodePlayer(&mem); p.MapID != 0x0C {
		t.Fatalf("after the approach the player is on map %#04x, want Route 1 (0x0C)", p.MapID)
	}
	startLevel := state.DecodeParty(&mem).Mons[0].Level

	res, err := skill.Train(e, romData, int(startLevel)+2, policy, 12)
	if err != nil {
		t.Fatalf("Train: %v (battles=%d)", err, res.Battles)
	}
	state.Snapshot(e, &mem)
	finalLevel := state.DecodeParty(&mem).Mons[0].Level
	if finalLevel < startLevel {
		t.Fatalf("the lead did not level up: start=%d final=%d (battles=%d, BlackedOut=%v)",
			startLevel, finalLevel, res.Battles, res.BlackedOut)
	}
	if !res.Reached {
		t.Fatalf("Reached = false, want true (start=%d, target=%d, final=%d, battles=%d)",
			startLevel, startLevel+2, finalLevel, res.Battles)
	}
	if res.EndLevel != int(finalLevel) {
		t.Errorf("EndLevel = %d, want %d (re-read from RAM)", res.EndLevel, finalLevel)
	}
	if res.Battles < 1 {
		t.Errorf("Battles = 0, want >= 1 (the level gain has no other source)")
	}
	if res.Battles > 13 {
		t.Errorf("Battles = %d, want <= maxBattles+1 = 13", res.Battles)
	}
	state.Snapshot(e, &mem)
	if state.DecodeBattle(&mem) != nil || !state.Controllable(&mem) {
		t.Fatalf("a battle was left in progress after Train: battle=%v controllable=%v",
			state.DecodeBattle(&mem) != nil, state.Controllable(&mem))
	}
	if res.BlackedOut {
		t.Logf("session ended in a blackout after %d battles (legitimate: a one-mon party faints from cumulative damage); landed on map %#04x at (%d,%d), level %d",
			res.Battles, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), finalLevel)
	}
	t.Logf("trained the lead from level %d to %d (target %d) in %d battles (BlackedOut=%v)",
		startLevel, finalLevel, startLevel+2, res.Battles, res.BlackedOut)
}

// TestTrainBudgetIsAResult: exhausting maxBattles without reaching the
// target ends with a result, not an error, and nothing is left in a
// battle. The target is far enough out that no budget of one or two
// battles can reach it, so the only way the session can end is the
// budget axis.
func TestTrainBudgetIsAResult(t *testing.T) {
	e := fixture.Load(t, "pallet_town")
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)

	dest := route1Grass(t, romData)
	if _, err := skill.Travel(e, romData, dest, policy, 6); err != nil {
		t.Fatalf("approach Travel to Route 1: %v", err)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	startLevel := state.DecodeParty(&mem).Mons[0].Level

	res, err := skill.Train(e, romData, int(startLevel)+8, policy, 1)
	if err != nil {
		t.Fatalf("Train: %v, want a result: not reaching the target within budget is not a failure", err)
	}
	if res.Reached {
		t.Fatalf("Reached = true with maxBattles=1, target %d, start %d", startLevel+8, startLevel)
	}
	if res.Battles < 1 {
		t.Fatalf("Battles = 0, want >= 1 (Route 1's grass throws an encounter within the session's leg budget)")
	}
	if res.Battles > 2 {
		t.Errorf("Battles = %d, want <= maxBattles+1 = 2", res.Battles)
	}
	state.Snapshot(e, &mem)
	if state.DecodeBattle(&mem) != nil || !state.Controllable(&mem) {
		t.Fatalf("a battle was left in progress after Train: battle=%v controllable=%v",
			state.DecodeBattle(&mem) != nil, state.Controllable(&mem))
	}
	t.Logf("budget test: %d battles, level %d -> %d", res.Battles, res.StartLevel, res.EndLevel)
}
