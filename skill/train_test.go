package skill_test

import (
	"errors"
	"os"
	"reflect"
	"strings"
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
// party and respawns it at Pallet Town (the last town's fly-warp spot).
// The retreat line is the other way a short session can end with the
// party hurt: there the player stands where the session stopped, so the
// test heals at a center before resuming. Either way the grind resumes
// from a healthy party — the caller's decision, made here — and the test
// still requires the level gain, the Reached flag, and a clean stop (no
// battle left in progress).
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

	// A lone L6 lead against Route 1's level 2-5 wilds takes enough
	// cumulative damage that a session of twelve battles ends before the
	// two-level climb is done — on the retreat line, or on a true blackout.
	// Either way the grind resumes from a healthy party at the grass: a
	// blackout already healed it at the respawn spot, a retreat left it
	// where it stopped and needs an explicit heal first (the caller's
	// decision, made here). Every battle of the effort counts toward the
	// total — the level gain may land on a walk, in which case the next
	// session ends at the target with no battles of its own.
	target := int(startLevel) + 2
	totalBattles := 0
	blackedOut := false
	var res skill.TrainResult
	for segment := 1; ; segment++ {
		r, err := skill.Train(e, romData, target, policy, 12)
		if err != nil {
			t.Fatalf("Train (segment %d): %v (battles=%d)", segment, err, totalBattles+r.Battles)
		}
		totalBattles += r.Battles
		res = r
		if r.Reached {
			break
		}
		if totalBattles > 60 {
			t.Fatalf("did not reach level %d in %d battles (endLevel=%d)", target, totalBattles, r.EndLevel)
		}
		if r.BlackedOut {
			blackedOut = true
		}
		if r.Retreated {
			center, ok := skill.Place("viridian pokemon center")
			if !ok {
				t.Fatal("Place(viridian pokemon center) did not resolve")
			}
			walk, err := fixture.Travel(e, center, policy, 6)
			if err != nil {
				t.Fatalf("travel to the center (segment %d): %v", segment, err)
			}
			totalBattles += walk.Battles
			if err := skill.Heal(e); err != nil {
				t.Fatalf("heal (segment %d): %v", segment, err)
			}
		}
		walk, err := fixture.Travel(e, dest, policy, 6)
		if err != nil {
			t.Fatalf("resume Travel to Route 1 (segment %d): %v", segment, err)
		}
		totalBattles += walk.Battles
		t.Logf("segment %d: %d battle(s), level %d, blackedOut=%v retreated=%v; resuming the grind", segment, r.Battles, r.EndLevel, r.BlackedOut, r.Retreated)
	}
	res.Battles = totalBattles
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
	// The bound counts the worst case: four segments of (session at most
	// maxBattles+1 battles, walk to the center and back at most two
	// maxBattles walks each) plus the final session.
	if res.Battles > 4*(13+6+6)+13 {
		t.Errorf("Battles = %d, want <= %d (four resume segments plus a final session)", res.Battles, 4*(13+6+6)+13)
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

// TestTrainRetreatsBeforeBlackout: a lone starter grinding Route 2 would
// have blacked out — measured, one battle before each of three such
// blackouts the lead stood at 5/27 (18.5%), 4/31 (12.9%) and 1/38 (2.6%),
// RUNNOTES S9-9 — but the session must end while the party can still walk:
// the new outcome set, no blackout, and the lead's HP above zero read from
// RAM, not inferred from the absence of a blackout flag.
func TestTrainRetreatsBeforeBlackout(t *testing.T) {
	if testing.Short() {
		t.Skip("full grind (Route 2); run without -short")
	}
	e := fixture.Load(t, "post_errand")
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)

	dest, ok := skill.Place("route 2")
	if !ok {
		t.Fatal("Place(route 2) did not resolve")
	}
	if _, err := fixture.Travel(e, dest, policy, 6); err != nil {
		t.Fatalf("setup travel to route 2: %v", err)
	}

	res, err := skill.Train(e, romData, 99, policy, 40)
	if err != nil {
		t.Fatalf("Train: %v (battles=%d)", err, res.Battles)
	}
	if res.Reached || res.BlackedOut {
		t.Fatalf("Reached=%v BlackedOut=%v, want neither (target 99 is out of budget; the session must end on the retreat line)", res.Reached, res.BlackedOut)
	}
	if !res.Retreated {
		t.Fatalf("Retreated = false after %d battles (endLevel=%d); the lone lead's cumulative damage must reach the retreat line before the budget or a blackout — measured: from this state it blacked out within 24 battles without the line", res.Battles, res.EndLevel)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	lead := state.DecodeParty(&mem).Mons[0]
	if lead.HP == 0 {
		t.Fatalf("lead HP = 0 in RAM; the retreat must leave the lead alive")
	}
	if int(lead.HP)*2 >= int(lead.MaxHP) {
		t.Errorf("lead HP = %d/%d in RAM, want below the retreat line (half max)", lead.HP, lead.MaxHP)
	}
	if state.DecodeBattle(&mem) != nil || !state.Controllable(&mem) {
		t.Fatalf("a sequence was left in progress after Train: battle=%v controllable=%v", state.DecodeBattle(&mem) != nil, state.Controllable(&mem))
	}
	t.Logf("retreat: %d battles, level %d -> %d, lead %d/%d HP — the session that would have blacked out ended alive", res.Battles, res.StartLevel, res.EndLevel, lead.HP, lead.MaxHP)
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
	// moveBite is the BITE this line is eventually offered. NOTE: the offer
	// does NOT come at level 22 — SquirtleEvosMoves says db 22, BITE, but the
	// fixture's SQUIRTLE evolves into WARTORTLE at 16 and LearnMoveFromLevelUp
	// reads the CURRENT species' table, where BITE sits at 24 (measured: a
	// grind to 22 ends with no prompt at all). So this test, which stops at
	// 22, never sees the "forget a move?" prompt; TestBattleAnswersForgetMovePrompt
	// grinds to 24 and asserts the resulting move set. Battle answers that
	// prompt on purpose (YES, replacing the lowest slot that is not the mon's
	// only damaging option); this test's assertion stays on reaching the level
	// and staying controllable.
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
// The target is level 22, which forces Train through the level-16 evolution
// cutscene (SQUIRTLE -> WARTORTLE) and keeps grinding past it. A pass proves
// Train survives the cutscene WITHOUT hanging and reaches the target.
// NOTE: this test does NOT exercise the "forget a move?" prompt — BITE is
// offered at level 24 on this line (WartortleEvosMoves, since the mon has
// already evolved), so a grind to 22 never sees it. The prompt is exercised
// by TestBattleAnswersForgetMovePrompt (target 24). On the task's Caterpie
// line the level-12 learned move (CONFUSION) is offered with only a plain
// text box (the Butterfree has three moves then, an empty slot).
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

	// Target level 22: past the level-16 evolution cutscene. A pass means
	// Train read the lead at level 22 AFTER the cutscene finished, i.e. it
	// survived it and kept grinding. (The "forget a move?" prompt is NOT on
	// this line's path to 22 — BITE is offered at 24, see the moveBite note;
	// TestBattleAnswersForgetMovePrompt covers it.)
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
		if r.Retreated {
			// The session stopped while the party was alive: the grind resumes
			// only from a healed lead, so walk to a center and heal — the
			// caller's decision, made here explicitly. A retreat does not
			// respawn the player, so without this the next segment would
			// start below the line and stop before it fights.
			center, ok := skill.Place("viridian pokemon center")
			if !ok {
				t.Fatal("Place(viridian pokemon center) did not resolve")
			}
			if _, err := fixture.Travel(e, center, policy, 6); err != nil {
				t.Fatalf("travel to the center (segment %d): %v", segment, err)
			}
			if err := skill.Heal(e); err != nil {
				t.Fatalf("heal (segment %d): %v", segment, err)
			}
		}
		t.Logf("segment %d: %d battle(s), level %d, blackedOut=%v retreated=%v; resuming the grind", segment, r.Battles, r.EndLevel, r.BlackedOut, r.Retreated)
	}
	state.Snapshot(e, &mem)
	after := state.DecodeParty(&mem).Mons[0]
	if after.Species != speciesWartortle {
		t.Fatalf("lead is species %#02x lv%d after training to %d, want WARTORTLE (%#02x) — Train did not carry the mon through the level-16 evolution", after.Species, after.Level, target, speciesWartortle)
	}
	if state.DecodeBattle(&mem) != nil || !state.Controllable(&mem) {
		t.Fatalf("a sequence was left in progress after Train: battle=%v controllable=%v", state.DecodeBattle(&mem) != nil, state.Controllable(&mem))
	}
	// Informational: BITE is NOT expected here — the offer comes at level 24
	// on this line (see the moveBite note), so a grind to 22 never sees the
	// prompt. TestBattleAnswersForgetMovePrompt asserts the learned set.
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
// TestNoEncounterDiagnostic pins Train's maxLegs diagnostic: it states what
// is actually known — legs walked, encounters rolled, the map, its grass
// rate and wild species count — and speculates about no cause. The message
// it replaced blamed the map's grass for having no encounter rate at all,
// a branch that cannot reach maxLegs any more (a zero rate makes grassCells
// return nil, so Train fails with "no walkable tall grass" long before)
// and sent the next reader in the wrong direction.
func TestNoEncounterDiagnostic(t *testing.T) {
	err := skill.NoEncounterDiagnostic(141, 2, 4, 0x33, 8, 5)
	msg := err.Error()
	for _, want := range []string{
		"no-encounter phase after 141 legs",
		"2 encounters (want 4 battles)",
		"map 0x0033",
		"grass rate 8/256",
		"5 species in wild table",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("diagnostic %q missing %q", msg, want)
		}
	}
	if strings.Contains(msg, "has no wild rate") {
		t.Errorf("diagnostic %q still blames the impossible cause the old message named", msg)
	}
}

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

// TestWildGrassMatchesTheDecomp pins the map's own answer to "what is
// catchable here" against data/wild/maps/*.asm, read as LoadWildData reads
// it: the rate byte, then ten (level, species) slots. Route 1 is six PIDGEY
// slots and four RATTATA; Pallet Town has a zero rate and so no species at
// all, which is the same fact HasGrass reports.
func TestWildGrassMatchesTheDecomp(t *testing.T) {
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
		want []skill.WildSpecies
	}{
		// Route1WildMons: rate 25, PIDGEY at 2-5 in six slots, RATTATA at
		// 2-4 in four.
		{0x0c, "ROUTE_1", []skill.WildSpecies{
			{ID: 0x24, MinLevel: 2, MaxLevel: 5, Slots: 6}, // PIDGEY
			{ID: 0xA5, MinLevel: 2, MaxLevel: 4, Slots: 4}, // RATTATA
		}},
		{0x00, "PALLET_TOWN", nil}, // NothingWildMons: rate 0
		{0x28, "OAKS_LAB", nil},
	}
	for _, tc := range cases {
		got, err := skill.WildGrass(data, tc.id)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: WildGrass = %+v, want %+v", tc.name, got, tc.want)
		}
		// The two readings of the same record must never disagree.
		hasGrass, err := skill.HasGrass(data, tc.id)
		if err != nil {
			t.Fatalf("%s: HasGrass: %v", tc.name, err)
		}
		if hasGrass && len(got) == 0 {
			t.Errorf("%s: HasGrass says yes but WildGrass names no species", tc.name)
		}
	}
}
