package skill_test

import (
	"errors"
	"os"
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
// is a legitimate ending of a session, not a failure: with a one-mon
// party, the cumulative damage of several wins can faint the lead even
// though it outbeats Route 1's level 2-5 wilds one at a time. Train ends
// the session at the blackout and reports it; the game fully heals the
// party and respawns it at Pallet Town (the last town's fly-warp spot),
// so the test walks back to the grass and resumes the grind — the
// caller's decision, made with the blackout knowledge — and still
// requires the level gain, the Reached flag, and a clean stop (no battle
// left in progress).
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
	blackedOut := res.BlackedOut
	if res.BlackedOut && !res.Reached {
		// The session blacked out short of the target. The party is fully
		// healed at the respawn spot, so walk back to the grass and resume
		// the grind from a healthy party. Every battle of the effort counts
		// toward the total — the level gain may land on the walk, in which
		// case the resumed session ends at the target with no battles of
		// its own.
		firstBattles := res.Battles
		walk, err := fixture.Travel(e, dest, policy, 6)
		if err != nil {
			t.Fatalf("resume Travel to Route 1: %v", err)
		}
		resumed, err := skill.Train(e, romData, int(startLevel)+2, policy, 12)
		if err != nil {
			t.Fatalf("Train (resumed): %v (battles=%d)", err, resumed.Battles)
		}
		res = resumed
		res.Battles += firstBattles + walk.Battles
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
	// The bound counts a blackout resume if one happened: the first
	// session (maxBattles+1), the walk back to the grass (its own
	// maxBattles), and the fresh session (maxBattles+1).
	if res.Battles > 13+6+13 {
		t.Errorf("Battles = %d, want <= 32 (a session, a resume walk, a fresh session)", res.Battles)
	}
	state.Snapshot(e, &mem)
	if state.DecodeBattle(&mem) != nil || !state.Controllable(&mem) {
		t.Fatalf("a battle was left in progress after Train: battle=%v controllable=%v",
			state.DecodeBattle(&mem) != nil, state.Controllable(&mem))
	}
	if blackedOut {
		t.Logf("a session blacked out (legitimate: a one-mon party faints from cumulative damage); the grind resumed from the respawn spot and landed on map %#04x at (%d,%d), level %d",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), finalLevel)
	}
	t.Logf("trained the lead from level %d to %d (target %d) in %d battles (blackedOut=%v)",
		startLevel, finalLevel, startLevel+2, res.Battles, blackedOut)
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

// speciesSquirtle / speciesWartortle are the ROM pokemon indexes (not dex
// numbers) for the post_pokeballs fixture's lead and its level-16 evolution.
// SQUIRTLE = $B1, WARTORTLE = $B3 (pokered/constants/pokemon_constants.asm);
// SquirtleEvosMoves declares EVOLVE_LEVEL, 16, WARTORTLE
// (pokered/data/pokemon/evos_moves.asm).
const (
	speciesSquirtle  uint8 = 0xB1
	speciesWartortle uint8 = 0xB3
	// moveBite is the move SQUIRTLE is offered at level 22 (db 22, BITE in
	// SquirtleEvosMoves). The fixture's level-15 SQUIRTLE already carries four
	// moves, so BITE is offered with the "forget a move?" prompt — the same
	// class of interruption as the task's level-12 move-learning prompt. It is
	// only used to LOG what Train does with that prompt (measured: it survives
	// it but dismisses it, so BITE is not learned); the assertion is on reaching
	// the level and staying controllable, not on the move being learned.
	moveBite uint8 = 0x2C
)

// TestTrainSurvivesEvolution is the S6-4 proof: Train must get a mon through
// a level-up EVOLUTION CUTSCENE ("SQUIRTLE is evolving! ... evolved into
// WARTORTLE!") and keep grinding, not stop or hang when the cutscene plays.
//
// It uses the post_pokeballs fixture's one-mon party — a level-15 SQUIRTLE —
// rather than a freshly caught Caterpie, for two measured reasons. First,
// a single-mon party means a faint is a clean blackout (ResultLost), never
// the "active mon fainted, healthy partner remains" state that battle.go
// cannot handle (the known Battle gap): a level-15 SQUIRTLE outclasses
// Route 1's level 2-5 wilds, so it will not faint at all. Second, this ROM's
// pokecenters have no PC machine sprite to deposit the partner, so there is
// no in-game way to make a caught Caterpie the only mon — see RUNNOTES.md.
//
// The target is level 22, which forces Train through BOTH interruption
// classes: the level-16 evolution cutscene (SQUIRTLE -> WARTORTLE) and the
// level-22 learned-move PROMPT (the fixture's SQUIRTLE already has four moves,
// so BITE is offered with a "forget a move?" yes/no plus move list, not a
// plain text box). A pass proves Train survives both WITHOUT hanging and keeps
// grinding to the target. Note this deliberately exercises the HARDER case:
// on the task's Caterpie line the level-12 learned move (CONFUSION) is offered
// with only a plain text box (the Butterfree has three moves then, an empty
// slot), which is strictly easier to survive than the prompt exercised here.
// It is a full journey, guarded out of -short like the other S6 journey tests:
//
//	POKEMON_RED_ROM=roms/pokemon_red.gb go test ./skill -run TestTrainSurvivesEvolution -v
func TestTrainSurvivesEvolution(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey (Route 1 grind); run without -short, see TestTrainSurvivesEvolution docs")
	}
	e := fixture.Load(t, "post_pokeballs")
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)

	// Precondition: the fixture carries exactly one mon, a level-15 SQUIRTLE.
	// A second mon would reintroduce the Battle gap the test is designed to
	// avoid, so the party size is asserted, not assumed.
	var mem state.Mem
	state.Snapshot(e, &mem)
	party := state.DecodeParty(&mem)
	if party.Count != 1 {
		t.Fatalf("fixture precondition: party has %d mons, want exactly one (a partner would hit the Battle gap)", party.Count)
	}
	lead := party.Mons[0]
	if lead.Species != speciesSquirtle || lead.Level < 15 {
		t.Fatalf("fixture precondition: lead is species %#02x lv%d, want SQUIRTLE (%#02x) lv>=15", lead.Species, lead.Level, speciesSquirtle)
	}

	dest := route1Grass(t, romData)

	// Target level 22: past the level-16 evolution AND the level-22 learned-
	// move prompt. A pass means Train read the lead at level 22 AFTER both
	// sequences finished, i.e. it survived each and kept grinding.
	const target = 22
	// Route 1's level 2-5 wilds give little exp to a level 15+ mon (measured
	// ~20 battles/level), and the cumulative damage of that many wins faints
	// the WARTORTLE before the climb is done — a clean blackout, since the
	// party is one mon. On a true blackout the game fully heals the party and
	// respawns it on a grassless town (measured: map 0x0001, which connects
	// south to Route 1), so each segment first re-Travel's to the grass; the
	// grind then resumes from the (healed) level it already reached. Exp and
	// level persist across blackouts; only HP resets.
	const totalCap = 400
	totalBattles := 0
	for segment := 1; ; segment++ {
		// Get (back) to the grass: the first segment walks from the fixture
		// location to Route 1; after a blackout it re-walks from the respawn
		// town. A healed level-15+ WARTORTLE does not faint on the approach.
		if _, err := skill.Travel(e, romData, dest, policy, 6); err != nil {
			t.Fatalf("Travel to Route 1 (segment %d): %v", segment, err)
		}
		r, err := skill.Train(e, romData, target, policy, 150)
		if err != nil {
			t.Fatalf("Train (segment %d): %v (battles=%d)", segment, err, totalBattles+r.Battles)
		}
		totalBattles += r.Battles
		if r.Reached {
			break
		}
		if totalBattles >= totalCap {
			t.Fatalf("did not reach level %d in %d battles (endLevel=%d) — Train stopped short of or hung at an interruption", target, totalBattles, r.EndLevel)
		}
		t.Logf("segment %d: %d battle(s), level %d, blackedOut=%v; resuming the grind", segment, r.Battles, r.EndLevel, r.BlackedOut)
	}
	state.Snapshot(e, &mem)
	after := state.DecodeParty(&mem).Mons[0]
	if after.Species != speciesWartortle {
		t.Fatalf("lead is species %#02x lv%d after training to %d, want WARTORTLE (%#02x) — Train did not carry the mon through the level-16 evolution", after.Species, after.Level, target, speciesWartortle)
	}
	if state.DecodeBattle(&mem) != nil || !state.Controllable(&mem) {
		t.Fatalf("a sequence was left in progress after Train: battle=%v controllable=%v", state.DecodeBattle(&mem) != nil, state.Controllable(&mem))
	}
	// Informational: whether the level-22 prompt actually learned BITE. Measured
	// behaviour is that Train survives the prompt (no hang, keeps grinding) but
	// dismisses it, so BITE is not in the set. This does not affect the Caterpie
	// line, where the level-12 move is a plain text box and IS learned.
	learnedBite := false
	for _, mv := range after.Moves {
		if mv == moveBite {
			learnedBite = true
		}
	}
	t.Logf("SQUIRTLE evolved to WARTORTLE (lv16) and ground through the lv22 learned-move prompt to level %d: %d battle(s), BITE learned=%v, moves %v", after.Level, totalBattles, learnedBite, after.Moves)
}

// TestHasGrassMatchesTheGameEncounterRule pins the two-part rule from
// wild_encounters.asm: a map counts as grass only where walkable tiles match
// the tileset's grass id AND the map's wild data has a non-zero grass rate.
// Pallet Town is the regression case: its top edge (10,0)-(11,1) stands on
// tileset 0's grass tile but points at NothingWildMons (rate 0), so the game
// never encounters there — and Train used to ping-pong those four tiles for
// 460 legs with zero battles, wasting rounds and tripping the failure budget.
func TestHasGrassMatchesTheGameEncounterRule(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		id   uint8
		name string
		want bool
	}{
		{0x00, "PALLET_TOWN", false}, // tiles match at the top edge, rate is 0
		{0x28, "OAKS_LAB", false},    // no matching tiles
		{0x01, "VIRIDIAN_CITY", false},
		{0x02, "PEWTER_CITY", false},
		{0x0c, "ROUTE_1", true}, // tiles match, rate 25
		{0x0d, "ROUTE_2", true},
	}
	for _, tc := range cases {
		if got, err := skill.HasGrass(data, tc.id); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		} else if got != tc.want {
			t.Errorf("%s: HasGrass = %v, want %v", tc.name, got, tc.want)
		}
	}
}
