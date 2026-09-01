package skill

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// The tileset table (pokered.sym: Tilesets = 03:47BE) is the same table
// world/grid.go reads for its collision side: 12 bytes per entry, the
// grass tile at +10, the block-data pointer at +1. Train keeps its own
// copy of the offsets because it needs the grass tile and block offset,
// which world does not expose.
const (
	trainTilesetsBank uint8  = 0x03
	trainTilesetsAddr uint16 = 0x47BE
	// WildDataPointers (pokered/pokered.sym): a 2-byte pointer per map to
	// that map's wild data record, whose first byte is the grass rate.
	trainWildBank        uint8  = 0x03
	trainWildAddr        uint16 = 0x4EEB
	trainTilesetEntryLen        = 12
)

// TrainResult reports what a grind session did.
type TrainResult struct {
	StartLevel int  // the lead's level when the session began
	EndLevel   int  // the lead's level when it ended
	Battles    int  // wild battles fought
	BlackedOut bool // a battle ended in ResultLost
	Reached    bool // EndLevel >= targetLevel
	Retreated  bool // stopped while the party was alive: continuing would have lost it
}

// ErrTrainRetreat marks a Train session that ended while the party was still
// able to walk: the lead's HP fell below the retreat line, so the next battle
// was more likely to faint it than to level it. It is an outcome, not a
// defect — the caller heals and trains again — but it must be distinguishable
// from both a shortfall (Reached == false on budget) and a wipe (BlackedOut),
// and it carries the exemption agent.Run gives a blackout: the session
// changed the party, so repeating the objective is not an identical attempt.
var ErrTrainRetreat = errors.New("skill: Train: stopped while the party was alive: continuing would have lost it")

// retreatLineNum/Den: a session stops when the lead's HP is below this
// fraction of its max, and refuses to start from below it. MEASURED on
// Route 2 (post_errand, one-mon L7 lead, type-aware policy): three
// blackout-ending sessions in which the lead stood at 5/27 (18.5%), 4/31
// (12.9%) and 1/38 (2.6%) one battle before the fatal one — every sub-20%
// reading in those sessions was followed by the blackout. Half max HP sits
// well above that line, with margin for other rDIV phases; RUNNOTES (S9-9)
// carries the full per-battle table.
const (
	retreatLineNum = 1
	retreatLineDen = 2
)

// Train levels the lead party member by fighting the wild encounters that
// tall grass on the current map throws at it.
//
// The session ping-pongs the player between two nearby grass cells: each
// leg is a Travel, so an encounter that interrupts a leg is fought with
// policy and Travel re-plans the rest of the leg from the encounter tile.
// After every leg the lead's level is re-read from RAM, and the session
// ends when the level reaches targetLevel, when maxBattles battles have
// been fought, when a blackout ends it, or when the lead's HP falls below
// the retreat line (Retreated == true): the fourth exit, which stops while
// the party can still walk instead of trading half the money and the walk
// back for the last battle of a session that was already lost.
//
// Non-battle sequences that a grind throws up — the level-up EVOLUTION
// cutscene ("CATERPIE evolved into METAPOD!") and the learned-move box
// ("learned CONFUSION!") — are survived without Train doing anything
// special, and this was MEASURED, not assumed (TestTrainSurvivesEvolution
// carries a level-15 SQUIRTLE through its level-16 evolution up to level 22
// without stopping or hanging).
// The reason is timing: both sequences run during the battle-end sequence
// while wIsInBattle is still set (pokered/engine/battle/end_of_battle.asm
// calls EvolutionAfterBattle before it clears wIsInBattle; the learned-move
// box plays during GainExperience, likewise in-battle), so Battle's loop is
// still running and its default A-tap branch advances them before Train ever
// re-reads the lead's level. Train therefore needs no separate handling for
// a plain learned-move box or an evolution cutscene. That includes the
// "forget a move?" prompt (a mon offered a new move while already holding
// four): it plays during GainExperience, likewise in-battle, so Battle's
// loop is the one that sees it and answers it — YES, then replacing the move
// in the lowest slot that is not the mon's only damaging option (forgetSlot
// in battle.go) — and Train only has to keep going. That prompt does not
// occur on the Caterpie->Butterfree line (the level-12 Butterfree holds
// three moves, an empty slot, so CONFUSION is a plain box).
//
// Three things bound the session: at most maxBattles battles are fought,
// reaching the target ends it early, and the retreat line ends it while the
// party can still walk. Not reaching the target within budget is a result
// (Reached == false, nil error), not a failure. A
// blackout ends the session and is reported: with no healthy mon left,
// grinding is over. A blackout is a legitimate ending, not a failure —
// cumulative damage across several battles can faint even a lead that
// beats the local wilds one at a time — and it does not visibly transport
// the player to a center in the fixture ROM, so the reported position is
// wherever the last leg's post-battle re-plan left the player, and
// resuming a grind means healing first.
//
// Grass cells come from the ROM: the tileset table names the map's grass
// tile and the map's blocks say which cells stand on it, and only
// walkable cells count, so a leg is never planned into a wall. If an
// encounter interrupts a leg exactly as the budget runs out, the pending
// battle is still fought: Train never leaves a battle in progress, so a
// session may fight maxBattles+1.
func Train(m *emu.Emu, romData []byte, targetLevel int, policy MovePolicy, maxBattles int) (TrainResult, error) {
	if policy == nil {
		return TrainResult{}, fmt.Errorf("skill: Train: nil policy")
	}
	if targetLevel < 1 {
		return TrainResult{}, fmt.Errorf("skill: Train: targetLevel must be >= 1, got %d", targetLevel)
	}
	if maxBattles <= 0 {
		return TrainResult{}, fmt.Errorf("skill: Train: maxBattles must be > 0, got %d", maxBattles)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return TrainResult{}, fmt.Errorf("skill: Train: player not controllable on map %#04x", m.Peek8(sym.CurMap))
	}
	now := currentWorld(m)
	res := TrainResult{StartLevel: int(state.DecodeParty(&mem).Mons[0].Level)}

	grass, grid, err := grassCells(romData, now.Map)
	if err != nil {
		return res, err
	}
	if len(grass) == 0 {
		return res, fmt.Errorf("skill: Train: no walkable tall grass on map %#04x", now.Map)
	}
	a, b, ok := grindPair(grass, grid, int(now.X), int(now.Y))
	if !ok {
		return res, fmt.Errorf("skill: Train: map %#04x has no two walkable grass cells close enough to grind between", now.Map)
	}

	// A session that starts below the retreat line has already lost: the
	// next battle is more likely to faint the lead than to level it, so the
	// session ends before it fights anything. Healing is the caller's
	// decision (KindHeal exists for exactly this).
	if leadBelowRetreatLine(m) {
		res.EndLevel = res.StartLevel
		res.Retreated = true
		return res, nil
	}

	// maxLegs bounds the session when encounters come sparser than the
	// budget assumes: tripping it means the game rolled fewer encounters
	// than the rate implies over maxLegs legs of walking. The roll is
	// hRandomAdd against the map's grass rate, sampled once per step onto
	// grass, and hRandomAdd advances with rDIV every frame — so which legs
	// produce a fight depends on when the steps land, not just on how many
	// there are (S9-8). The budget is therefore a function of THIS map's
	// rate, not a constant sized once against one map's 8/256.
	rate, err := wildGrassRate(romData, now.Map)
	if err != nil {
		return res, err
	}
	maxLegs := LegBudget(maxBattles, rate)
	next := b
	legs := 0
	for {
		tr, err := Travel(m, romData, Destination{Map: now.Map, X: uint8(next.x), Y: uint8(next.y)}, policy, maxBattles-res.Battles)
		res.Battles += tr.Battles
		if tr.BlackedOut {
			res.BlackedOut = true
		}
		if err != nil && !battleInFlight(m) {
			// A non-battle failure: nothing is left in progress, so the
			// error is reported as-is — unless the party has blacked out.
			// A blackout teleports the player to a Pokemon Center, and
			// Travel re-plans from there: it walks the whole route back
			// toward grind cells that are now maps away, and fails
			// somewhere along it. The session ended at the blackout, so
			// report that ending rather than the walk that followed it.
			res.EndLevel = leadLevel(m)
			res.Reached = res.EndLevel >= targetLevel
			if res.BlackedOut {
				return res, nil
			}
			return res, err
		}
		if err != nil {
			// Travel's budget ran out with a fresh encounter pending.
			// Finish it: the session ends, but nothing may be left
			// mid-battle.
			outcome, berr := Battle(m, policy)
			if berr != nil {
				return res, fmt.Errorf("skill: Train: battle %d: %w", res.Battles+1, berr)
			}
			res.Battles++
			if outcome == state.ResultLost {
				res.BlackedOut = true
			}
		}
		res.EndLevel = leadLevel(m)
		res.Reached = res.EndLevel >= targetLevel
		if res.BlackedOut || res.Reached || res.Battles >= maxBattles {
			return res, nil
		}
		// The fourth exit: the party is alive but continuing would have lost
		// it. End the session and report the retreat; a blackout costs half
		// the money and the walk back, this costs only the level the next,
		// healed, session can still earn.
		if leadBelowRetreatLine(m) {
			res.Retreated = true
			return res, nil
		}
		if legs+1 > maxLegs {
			species := 0
			if sp, serr := WildGrass(romData, now.Map); serr == nil {
				species = len(sp)
			}
			return res, NoEncounterDiagnostic(legs+1, res.Battles, maxBattles, now.Map, rate, species)
		}
		next = flip(a, b, next)
		legs++
	}
}

// PromoteToLead moves party member index (1..Count-1) into slot 0 through
// the in-game party swap: START -> PKMN -> select the current lead ->
// SWITCH -> select the partner. Every step is verified against RAM before
// the next input, and the function returns only once the wanted species is
// asserted to be Mons[0] and the player is controllable again.
//
// The swap is the only way a caught mon can ever fight: US Red has no other
// party reordering, every battle sends out slot 0 (InitBattleVariables
// zeroes wPlayerMonNumber), and only the mon that fights gains experience
// (wPartyGainExpFlags is set for wPlayerMonNumber alone, see
// engine/battle/experience.asm). Training a caught Caterpie to Butterfree
// therefore starts here.
func PromoteToLead(m *emu.Emu, index int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	if index < 1 || index >= int(party.Count) {
		return fmt.Errorf("skill: PromoteToLead: index %d out of range for a party of %d (want 1..%d)", index, party.Count, party.Count-1)
	}
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: PromoteToLead: player not controllable on map %#04x", m.Peek8(sym.CurMap))
	}
	want := party.Mons[index].Species
	partyMax := int(party.Count) - 1

	waitFor := func(budget int, what string, pred func(*state.Mem) bool) error {
		if _, err := m.StepUntil(budget, func(m *emu.Emu) bool {
			var s state.Mem
			state.Snapshot(m, &s)
			return pred(&s)
		}); err != nil {
			return fmt.Errorf("skill: PromoteToLead: %s did not appear within %d frames: %w", what, budget, err)
		}
		return nil
	}
	tap := func(b emu.Button) { m.Tap(b, 3, 7) }
	onScreen := func(s *state.Mem, marker string) bool {
		return strings.Contains(state.ScreenText(s), marker)
	}

	// moveCursor taps toward target until wCurrentMenuItem reads it. It is
	// convention-agnostic on purpose: the party menu stores wMaxMenuItem as
	// the LAST valid index (count-1) while the start menu stores the COUNT,
	// so SelectMenuItem's exclusive range check rejects the party menu's last
	// entry (the partner). Moving by verifying wCurrentMenuItem sidesteps the
	// mismatch; the cursor index is the positive fact, as in SelectMenuItem.
	moveCursor := func(target int) error {
		for i := 0; i < 60; i++ {
			var s state.Mem
			state.Snapshot(m, &s)
			cur := state.DecodeMenu(&s).Current
			if cur == target {
				return nil
			}
			btn := emu.Down
			if cur > target {
				btn = emu.Up
			}
			m.Tap(btn, 3, 7)
		}
		var s state.Mem
		state.Snapshot(m, &s)
		return fmt.Errorf("skill: PromoteToLead: cursor stuck at %d, want %d", state.DecodeMenu(&s).Current, target)
	}

	// pressUntil presses A and waits for the positive fact pred (the next
	// screen / RAM change). The FIRST A can be lost in the menu's joypad-init
	// window — the footer text is drawn a few frames before HandlePartyMenuInput
	// starts polling, so an A fired on the detection frame is dropped (measured:
	// the first A never lands, the second always does). It re-presses A until
	// pred holds, each re-press gated on stillHere so a stray A that left the
	// expected screen is reported instead of chased. Success is pred, never a
	// press count.
	// pressKeyUntil presses btn and waits for the positive fact pred. The
	// FIRST press can be lost in the menu's joypad-init window — the screen
	// is drawn a few frames before the input handler starts polling, so a
	// press fired on the detection frame is dropped (measured: the first A
	// never lands, the second always does; B behaves the same). It re-presses
	// until pred holds, each re-press gated on stillHere so a stray press that
	// left the expected screen is reported instead of chased. Success is pred,
	// never a press count.
	pressKeyUntil := func(btn emu.Button, budget int, what string, stillHere, pred func(*state.Mem) bool) error {
		for i := 0; i < budget/25; i++ {
			var s state.Mem
			state.Snapshot(m, &s)
			if pred(&s) {
				return nil
			}
			m.Tap(btn, 3, 7)
			if _, err := m.StepUntil(25, func(m *emu.Emu) bool {
				var s2 state.Mem
				state.Snapshot(m, &s2)
				return pred(&s2)
			}); err == nil {
				return nil
			}
			state.Snapshot(m, &s)
			if !stillHere(&s) {
				return fmt.Errorf("skill: PromoteToLead: %s: left the expected screen before the press took", what)
			}
		}
		return fmt.Errorf("skill: PromoteToLead: %s did not appear after repeated presses", what)
	}

	pressUntil := func(budget int, what string, stillHere, pred func(*state.Mem) bool) error {
		return pressKeyUntil(emu.A, budget, what, stillHere, pred)
	}

	pick := func(index, budget int, what string, stillHere, pred func(*state.Mem) bool) error {
		if err := moveCursor(index); err != nil {
			return fmt.Errorf("skill: PromoteToLead: %s: %w", what, err)
		}
		return pressUntil(budget, what, stillHere, pred)
	}

	// Every wait below is a POSITIVE screen fact, never a stale cursor or
	// menu counter: wMaxMenuItem and wCurrentMenuItem survive across menus
	// in RAM, so "Max == 7" matched the overworld on the first attempt and
	// the taps went to the player sprite instead of a menu.

	// START menu: seven entries (POKEDex PKMN ITEM name SAVE OPTIONS EXIT),
	// opened with the START button (A talks to sprites in the overworld),
	// drawn straight into wTileMap rather than as a text box.
	tap(emu.Start)
	// Require a valid Max (6 or 7) in the SAME snapshot as the labels: the
	// footer text is drawn before wMaxMenuItem is written, so reading Max
	// from an earlier frame could give a stale count and pick the wrong entry.
	if err := waitFor(500, "start menu", func(s *state.Mem) bool {
		mx := state.DecodeMenu(s).Max
		return onScreen(s, "SAVE") && onScreen(s, "EXIT") && (mx == 6 || mx == 7)
	}); err != nil {
		return err
	}
	// PKMN is the second entry when the POKéDEX is present (7 items) and the
	// first when it is not (6 items); the POKéDEX entry sits at the top and
	// pushes everything down by one.
	var mem2 state.Mem
	state.Snapshot(m, &mem2)
	pkmnIndex := 0
	if state.DecodeMenu(&mem2).Max == 7 {
		pkmnIndex = 1
	}
	// Normal party screen: the footer reads "Choose a POKéMON." (the # glyph
	// renders as the POKé ligature).
	if err := pick(pkmnIndex, 600, "start menu", func(s *state.Mem) bool {
		return onScreen(s, "SAVE") && onScreen(s, "EXIT")
	}, func(s *state.Mem) bool {
		return onScreen(s, "Choose") && state.DecodeMenu(s).Max == partyMax
	}); err != nil {
		return err
	}
	// Select the current lead; its option menu (STATUS SWITCH CANCEL with no
	// field moves known) is a text box.
	if err := pick(0, 600, "party screen", func(s *state.Mem) bool {
		return onScreen(s, "Choose")
	}, func(s *state.Mem) bool {
		return onScreen(s, "SWITCH")
	}); err != nil {
		return err
	}
	// SWITCH is the middle entry: STATUS SWITCH CANCEL. Selecting it enters
	// swap mode; the footer changes to "Move POKéMON where?".
	if err := pick(1, 600, "option menu", func(s *state.Mem) bool {
		return onScreen(s, "SWITCH")
	}, func(s *state.Mem) bool {
		return onScreen(s, "where?") && state.DecodeMenu(s).Max == partyMax
	}); err != nil {
		return err
	}
	// Select the partner; SwitchPartyMon performs the swap. The positive fact
	// that it happened: the wanted species is now Mons[0] in RAM.
	if err := pick(index, 600, "swap-mode party screen", func(s *state.Mem) bool {
		return onScreen(s, "where?")
	}, func(s *state.Mem) bool {
		return state.DecodeParty(s).Mons[0].Species == want
	}); err != nil {
		return err
	}
	// B out of the party screen (back to the start menu), B out of the
	// start menu (overworld). Both are retry-gated: the first B is lost in
	// the same joypad-init window as the A presses.
	if err := pressKeyUntil(emu.B, 600, "party screen exit", func(s *state.Mem) bool {
		return onScreen(s, "Choose")
	}, func(s *state.Mem) bool {
		return onScreen(s, "SAVE") && onScreen(s, "EXIT")
	}); err != nil {
		return err
	}
	if err := pressKeyUntil(emu.B, 600, "start menu exit", func(s *state.Mem) bool {
		return onScreen(s, "SAVE") && onScreen(s, "EXIT")
	}, func(s *state.Mem) bool {
		return state.Controllable(s)
	}); err != nil {
		return err
	}

	state.Snapshot(m, &mem)
	if got := state.DecodeParty(&mem).Mons[0].Species; got != want {
		return fmt.Errorf("skill: PromoteToLead: lead is species %#02x, want %#02x", got, want)
	}
	return nil
}

// cell is a game-tile position on a map.
type cell struct{ x, y int }

// HasGrass reports whether mapID has any walkable tall grass where the
// game actually rolls encounters — the precondition for training at all.
// It is a map feature, decoded from the same tileset table and wild data
// Train reads, so a caller that knows the current map can say whether
// "train" is even possible there without hunting.
func HasGrass(romData []byte, mapID uint8) (bool, error) {
	grass, _, err := grassCells(romData, mapID)
	if err != nil {
		return false, err
	}
	return len(grass) > 0, nil
}

// grassCells returns the walkable cells of mapID that stand on the
// tileset's grass tile — the cells where the game actually rolls wild
// encounters — along with the map's collision grid (nil when the map has
// no such cells). The grass tile and block data come from the tileset
// table, exactly as world/grid.go reads it for collisions.
func grassCells(romData []byte, mapID uint8) ([]cell, *world.Grid, error) {
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return nil, nil, fmt.Errorf("skill: Train: map %#04x: %w", mapID, err)
	}
	// The game rolls grass encounters only where BOTH the tile underfoot
	// matches the tileset's grass id AND the map's wild data has a
	// non-zero grass rate (pokered/engine/battle/wild_encounters.asm).
	// The tile match alone lies: Pallet Town's top edge stands on its
	// tileset's grass tile but points at NothingWildMons, so the game stays
	// quiet there and a session would ping-pong those tiles forever.
	if rate, err := wildGrassRate(romData, mapID); err != nil {
		return nil, nil, err
	} else if rate == 0 {
		return nil, nil, nil
	}
	tsOff, err := bankedOff(trainTilesetsBank, trainTilesetsAddr)
	if err != nil {
		return nil, nil, err
	}
	entry := tsOff + int(h.Tileset)*trainTilesetEntryLen
	if entry+trainTilesetEntryLen > len(romData) {
		return nil, nil, fmt.Errorf("skill: Train: map %#04x: tileset %d entry out of ROM", mapID, h.Tileset)
	}
	grass := romData[entry+10]
	blockOff, err := bankedOff(romData[entry], uint16(romData[entry+1])|uint16(romData[entry+2])<<8)
	if err != nil {
		return nil, nil, err
	}
	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		return nil, nil, fmt.Errorf("skill: Train: map %#04x: %w", mapID, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return nil, nil, fmt.Errorf("skill: Train: map %#04x: %w", mapID, err)
	}

	var out []cell
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if !grid.Walkable(x, y) {
				continue
			}
			bx, by := x/2, y/2
			sx, sy := x%2, y%2
			id := blocks[by*int(h.WidthBlocks)+bx]
			// The game rolls encounters on the bottom-right tile of the
			// 2x2 step, so that is the tile that must be grass.
			if romData[blockOff+int(id)*16+(2*sy+1)*4+(2*sx+1)] == grass {
				out = append(out, cell{x, y})
			}
		}
	}
	return out, grid, nil
}

// grindPair picks the two cells the session ping-pongs between: a is the
// grass cell nearest the player, and b is a grass cell on the same row or
// column, 1-6 cells away, chosen for the densest grass along the straight
// path between them. Every cell of that path is walkable, so each leg is
// a short straight walk.
func grindPair(grass []cell, grid *world.Grid, px, py int) (cell, cell, bool) {
	at := cell{px, py}
	a := grass[0]
	for _, c := range grass {
		if dist(c, at) < dist(a, at) {
			a = c
		}
	}
	isGrass := make(map[cell]bool, len(grass))
	for _, c := range grass {
		isGrass[c] = true
	}

	var (
		best      cell
		bestScore = -1
		bestDist  = 1 << 30
	)
	for _, c := range grass {
		if c == a || (c.x != a.x && c.y != a.y) {
			continue
		}
		d := dist(a, c)
		if d < 1 || d > 6 {
			continue
		}
		dx, dy := sign(c.x-a.x), sign(c.y-a.y)
		ok := true
		for s := 1; s < d; s++ {
			if !grid.Walkable(a.x+dx*s, a.y+dy*s) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		score := 0
		for s := 0; s <= d; s++ {
			if isGrass[cell{a.x + dx*s, a.y + dy*s}] {
				score++
			}
		}
		if score > bestScore || (score == bestScore && d < bestDist) {
			best, bestScore, bestDist = c, score, d
		}
	}
	if bestScore < 0 {
		return cell{}, cell{}, false
	}
	return a, best, true
}

// battleInFlight reports whether a battle is in progress in RAM: the same
// check Battle performs before it will fight.
func battleInFlight(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.DecodeBattle(&mem) != nil
}

// leadLevel re-reads the lead party member's level from RAM.
func leadLevel(m *emu.Emu) int {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return int(state.DecodeParty(&mem).Mons[0].Level)
}

// BelowRetreatLine reports whether a lead at hp/maxHP is below the
// retreat line — the one fraction a Train session enforces, written only
// in retreatLineNum/Den. It is the shared predicate: Train's start check
// and the loop exit call it, and agent.Offer withholds the Train objective
// on it, so a lead below the line is never handed an objective that would
// refuse before it fights anything. The comparison is strict: a lead at
// exactly the line (hp*Den == maxHP*Num) is NOT below it, and a session
// started there is allowed to fight. A zero max reads as below the line:
// a fainted lead (HP 0, a backup mon still standing) is exactly the state
// where continuing would spend that backup the same way.
func BelowRetreatLine(hp, maxHP uint16) bool {
	if maxHP == 0 {
		return true
	}
	return int(hp)*retreatLineDen < int(maxHP)*retreatLineNum
}

// leadBelowRetreatLine reports whether the lead's HP is below the fraction
// of its max at which a session stops instead of risking the party.
func leadBelowRetreatLine(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	lead := state.DecodeParty(&mem).Mons[0]
	return BelowRetreatLine(lead.HP, lead.MaxHP)
}

// flip returns the other end of the ping-pong.
func flip(a, b, cur cell) cell {
	if cur == a {
		return b
	}
	return a
}

// LegBudget is how many ping-pong legs a Train session may walk looking
// for want battles on a map whose grass rate is rate/256 per step.
//
// It is the expected legs — want ÷ (rate/256) — plus three standard
// deviations of the negative binomial that models the walk
// (sigma = 16·sqrt(want·(256−rate)) ÷ rate). The old constant
// (20·want + 60) was sized once against map 0x33's 8/256 rate and sat
// only ~⅕σ above the mean legs for four battles, so ~35% of sessions
// walked it out (S9-8). Bounded and positive for every rate in 1..255:
// base is a ceiling division (so at least 1) and slack is non-negative.
func LegBudget(want int, rate uint8) int {
	if want <= 0 || rate == 0 {
		return 0
	}
	base := (want*256 + int(rate) - 1) / int(rate) // want ÷ (rate/256), rounded up
	// 3σ in integer arithmetic: 48·sqrt(want·(256−rate)) ÷ rate, rounded up.
	slack := (48*ceilSqrt(want*(256-int(rate))) + int(rate) - 1) / int(rate)
	return base + slack
}

// ceilSqrt is the ceiling of the integer square root of n (n >= 0).
func ceilSqrt(n int) int {
	s := int(math.Sqrt(float64(n)))
	if s*s < n {
		s++
	}
	return s
}

// NoEncounterDiagnostic renders Train's maxLegs diagnostic: what is
// actually known when a session walks out its leg budget short of battles —
// how many legs, how many encounters, on which map, with that map's grass
// rate and wild species count read from the wild table. It states no cause:
// the roll is hRandomAdd against the grass rate, sampled once per step onto
// grass, and hRandomAdd advances with rDIV every frame, so a dry stretch is
// a property of WHEN the steps land as much as of how many there are.
// Callers that retry after a no-encounter phase match on its stable prefix.
func NoEncounterDiagnostic(legs, battles, want int, mapID uint8, rate uint8, species int) error {
	return fmt.Errorf("skill: Train: no-encounter phase after %d legs: %d encounters (want %d battles) on map %#04x, grass rate %d/256, %d species in wild table",
		legs, battles, want, mapID, rate, species)
}

// wildSlots is how many (level, species) pairs a grass record carries.
// def_grass_wildmons is followed by exactly NUM_WILDMONS entries
// (pokered/macros/asserts.asm asserts the length), then the water record.
const wildSlots = 10

// WildSpecies is one species a map's grass can roll, with the level band it
// appears at and how many of the ten encounter slots it holds. Slots is
// rarity in the only form the ROM states it: a species in six slots is met
// roughly six times as often as one in a single slot.
type WildSpecies struct {
	ID       uint8
	MinLevel uint8
	MaxLevel uint8
	Slots    int
}

// WildGrass returns the species a map's tall grass can roll, in ROM slot
// order, deduplicated. It is empty when the map has no grass encounters at
// all — the same zero rate HasGrass checks, so the two agree by
// construction. This is the map's own answer to "what is catchable here";
// nothing about it is a preference, and no species is special.
func WildGrass(romData []byte, mapID uint8) ([]WildSpecies, error) {
	off, err := wildRecord(romData, mapID)
	if err != nil {
		return nil, err
	}
	if romData[off] == 0 {
		return nil, nil // no grass encounters on this map
	}
	if off+1+2*wildSlots > len(romData) {
		return nil, fmt.Errorf("skill: WildGrass: map %#04x: wild data record runs past the ROM", mapID)
	}
	var out []WildSpecies
	index := map[uint8]int{}
	for i := 0; i < wildSlots; i++ {
		level, species := romData[off+1+2*i], romData[off+2+2*i]
		if species == 0 {
			continue
		}
		if at, seen := index[species]; seen {
			out[at].Slots++
			if level < out[at].MinLevel {
				out[at].MinLevel = level
			}
			if level > out[at].MaxLevel {
				out[at].MaxLevel = level
			}
			continue
		}
		index[species] = len(out)
		out = append(out, WildSpecies{ID: species, MinLevel: level, MaxLevel: level, Slots: 1})
	}
	return out, nil
}

// wildRecord returns the ROM offset of a map's wild data record, read
// exactly as LoadWildData does (pokered/engine/overworld/wild_mons.asm):
// the map's WildDataPointers entry names the record. The record's first
// byte is the grass encounter rate.
func wildRecord(romData []byte, mapID uint8) (int, error) {
	base, err := bankedOff(trainWildBank, trainWildAddr)
	if err != nil {
		return 0, fmt.Errorf("skill: wild data: %w", err)
	}
	pOff := base + int(mapID)*2
	if pOff+2 > len(romData) {
		return 0, fmt.Errorf("skill: wild data: map %#04x: pointer out of ROM", mapID)
	}
	// The pointer is a 16-bit offset in the same bank as WildDataPointers
	// (LoadWildData loads it straight into hl and uses it):
	// ld a,[hli] / ld h,[hl] / ld l,a.
	off := uint16(romData[pOff]) | uint16(romData[pOff+1])<<8
	recOff, err := bankedOff(trainWildBank, off)
	if err != nil {
		return 0, fmt.Errorf("skill: wild data: map %#04x: %w", mapID, err)
	}
	if recOff >= len(romData) {
		return 0, fmt.Errorf("skill: wild data: map %#04x: record out of ROM", mapID)
	}
	return recOff, nil
}

// wildGrassRate returns the map's grass encounter rate: the first byte of
// the record wildRecord locates. Zero means the game never rolls an
// encounter there, whatever the tiles underfoot look like.
func wildGrassRate(romData []byte, mapID uint8) (uint8, error) {
	off, err := wildRecord(romData, mapID)
	if err != nil {
		return 0, err
	}
	return romData[off], nil
}

// bankedOff converts a banked address (bank:addr) to a ROM file offset,
// the same mapping world.bankedOffset uses. Kept local: Train only reads
// the tileset table, which the world package does not expose.
func bankedOff(bank uint8, addr uint16) (int, error) {
	if addr >= 0x4000 {
		return int(bank)*0x4000 + int(addr-0x4000), nil
	}
	if bank != 0 {
		return 0, fmt.Errorf("address %04X in bank %d is below 0x4000", addr, bank)
	}
	return int(addr), nil
}

func dist(a, b cell) int { return absInt(a.x-b.x) + absInt(a.y-b.y) }

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v int) int {
	if v < 0 {
		return -1
	}
	return 1
}
