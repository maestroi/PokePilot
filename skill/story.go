package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// Starter identifies which Poké Ball on Oak's table to take.
type Starter uint8

const (
	StarterCharmander Starter = iota // ball at (6,3), approached from (6,4)
	StarterSquirtle                  // ball at (7,3), approached from (7,4)
	StarterBulbasaur                 // ball at (8,3), approached from (8,4)
)

// oaksLabMap is Oak's lab, where the starter choice and the rival battle
// happen. Pallet Town (0x00) and Route 1 (0x0C) are addressed through
// Place() and the map graph, not by raw id.
const oaksLabMap = 0x28

// Frame budgets for the scripted phases. The whole sequence runs in about
// eight seconds of game time; a budget this large can only be exhausted by a
// loop, so exhausting one is a real failure, not slowness.
const (
	gateWaitBudget    = 60
	cutsceneBudget    = 30000 // Oak's cutscene: lab walk plus four text boxes
	choiceWaitBudget  = 3000  // A on the ball to the YesNoChoice menu
	starterWaitBudget = 10000 // ball taken: flag plus party mon
	battleWaitBudget  = 10000 // challenge text to battle start
)

// labNPCBlocked lists the NPC sprites inside Oak's lab that block tiles the
// static grid does not model: the rival at (4,3) and Professor Oak at (5,2).
var labNPCBlocked = [][2]int{{4, 3}, {5, 2}}

// ball returns the ball tile and the approach tile one row below it.
func (s Starter) ball() (ballX, ballY, approachX, approachY uint8) {
	switch s {
	case StarterCharmander:
		return 6, 3, 6, 4
	case StarterSquirtle:
		return 7, 3, 7, 4
	case StarterBulbasaur:
		return 8, 3, 8, 4
	}
	return 0, 0, 0, 0
}

// GetStarter plays the opening story from a freshly booted game: walk north
// in Pallet Town until Oak's gate fires, follow Oak into the lab, take the
// chosen starter, and win the rival's battle. It returns nil when the story
// is already complete (idempotent) and an error naming the map, coordinates,
// wJoyIgnore and the relevant story flags on any failure.
func GetStarter(m *emu.Emu, romData []byte, which Starter, policy MovePolicy) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.HasEvent(&mem, state.EventBattledRivalInOaksLab) {
		return nil
	}
	if which > StarterBulbasaur {
		return fmt.Errorf("skill: GetStarter: unknown starter %d", which)
	}
	if policy == nil {
		return fmt.Errorf("skill: GetStarter: nil policy")
	}

	// 1. Be in Pallet Town.
	dest, ok := Place("pallet town")
	if !ok {
		return fmt.Errorf("skill: GetStarter: Place: pallet town not found")
	}
	if err := GoTo(m, romData, dest); err != nil {
		return fmt.Errorf("skill: GetStarter: %w", err)
	}

	// 2. Walk north to the exit. The gate fires at wYCoord == 1; the path is
	//    measured on the real map (20x18) and matches
	//    TestCutsceneEnduresOakGate: from (5,6) right x3, up x4, right x2,
	//    up to (10,1).
	gatePath := []world.Step{
		world.StepRight, world.StepRight, world.StepRight,
		world.StepUp, world.StepUp, world.StepUp, world.StepUp,
		world.StepRight, world.StepRight,
		world.StepUp,
	}
	if err := WalkPath(m, gatePath); err != nil && !errors.Is(err, ErrDialogueInterrupted) {
		x, y := playerXY(m)
		return fmt.Errorf("skill: GetStarter: walk to the north exit on map %#04x at (%d,%d): %w",
			m.Peek8(sym.CurMap), x, y, err)
	}

	// 3. The gate fires within a frame of the player reaching y==1:
	//    wJoyIgnore goes non-zero and the step into the exit is blocked. That
	//    block is the trigger, not a failure.
	if _, err := m.StepUntil(gateWaitBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.JoyIgnore) != 0
	}); err != nil {
		state.Snapshot(m, &mem)
		return fmt.Errorf("skill: GetStarter: Oak's gate did not fire: map=%#04x at (%d,%d) wJoyIgnore=%#04x EventFollowedOakIntoLab=%v",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), state.HasEvent(&mem, state.EventFollowedOakIntoLab))
	}

	// 4. Oak's cutscene: he walks the player into the lab and the script
	//    holds control for four text boxes. Use EventOakAskedToChooseMon as
	//    the predicate: at EventFollowedOakIntoLab all four directions are
	//    still blocked.
	if err := Cutscene(m, cutsceneBudget, func(mm *state.Mem) bool {
		return state.HasEvent(mm, state.EventOakAskedToChooseMon)
	}); err != nil {
		return fmt.Errorf("skill: GetStarter: %w", err)
	}
	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, state.EventOakAskedToChooseMon) {
		return fmt.Errorf("skill: GetStarter: %s not set after the cutscene: map=%#04x at (%d,%d) wJoyIgnore=%#04x",
			state.EventOakAskedToChooseMon, mem.U8(sym.CurMap), mem.U8(sym.XCoord),
			mem.U8(sym.YCoord), mem.U8(sym.JoyIgnore))
	}
	if mem.U8(sym.CurMap) != oaksLabMap || mem.U8(sym.XCoord) != 5 || mem.U8(sym.YCoord) != 3 {
		return fmt.Errorf("skill: GetStarter: after the cutscene the player is on map %#04x at (%d,%d), want map %#04x at (5,3); wJoyIgnore=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), oaksLabMap, mem.U8(sym.JoyIgnore))
	}

	// 5. Walk to the approach tile below the chosen ball. The rival at (4,3)
	//    and Oak at (5,2) block tiles world.Grid does not model, and the
	//    three ball tiles are objects, so pass all of them as blocked.
	_, _, ax, ay := which.ball()
	if err := walkLab(m, romData, int(ax), int(ay), labBlockedSet()); err != nil {
		return err
	}

	// 6. Face the ball, answer YES. Taking the ball sets wStatusFlags4 bit 3
	//    (TookStarterBall) and adds the party mon; it does NOT set
	//    EventGotStarter.
	bx, by, _, _ := which.ball()
	if err := Face(m, bx, by); err != nil {
		return fmt.Errorf("skill: GetStarter: %w", err)
	}
	if err := chooseStarterBall(m); err != nil {
		return err
	}
	mem = advanceUntil(m, starterWaitBudget, func(mm *state.Mem) bool {
		return state.TookStarterBall(mm) && state.DecodeParty(mm).Count >= 1
	})
	if !state.TookStarterBall(&mem) || state.DecodeParty(&mem).Count < 1 {
		return fmt.Errorf("skill: GetStarter: starter not taken within %d frames: TookStarterBall=%v party=%d map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			starterWaitBudget, state.TookStarterBall(&mem), state.DecodeParty(&mem).Count,
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}

	// 7. The game takes control while the rival walks to the table and takes
	//    his own mon. EventGotStarter is set at the end of that script.
	if err := Cutscene(m, cutsceneBudget, func(mm *state.Mem) bool {
		return state.HasEvent(mm, state.EventGotStarter)
	}); err != nil {
		return fmt.Errorf("skill: GetStarter: %w", err)
	}

	// 8. Only now walk to wYCoord == 6: before EventGotStarter the map script
	//    force-walks the player back up, after it the rival challenges. Row
	//    6 is mostly counters, so path to (5,6). The challenge text opens the
	//    moment the player reaches row 6 (at (5,6) or, if the path passes
	//    through (4,6) first, there); that interruption is the expected
	//    outcome.
	if err := walkLab(m, romData, 5, 6, labBlockedSet()); err != nil {
		if !errors.Is(err, ErrDialogueInterrupted) {
			return err
		}
		x, y := playerXY(m)
		if int(y) != 6 {
			return fmt.Errorf("skill: GetStarter: dialogue at (%d,%d), want row 6: %w", x, y, err)
		}
	}

	// 9. Advance the challenge text until the battle starts.
	mem = advanceUntil(m, battleWaitBudget, func(mm *state.Mem) bool {
		return state.DecodeBattle(mm) != nil
	})
	if state.DecodeBattle(&mem) == nil {
		return fmt.Errorf("skill: GetStarter: rival battle did not start within %d frames: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x EventGotStarter=%v",
			battleWaitBudget, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded), state.HasEvent(&mem, state.EventGotStarter))
	}

	result, err := Battle(m, policy)
	if err != nil {
		return err
	}
	if result != state.ResultWon {
		state.Snapshot(m, &mem)
		return fmt.Errorf("skill: GetStarter: rival battle result = %d, want win: map=%#04x at (%d,%d) wJoyIgnore=%#04x EventBattledRivalInOaksLab=%v",
			result, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), state.HasEvent(&mem, state.EventBattledRivalInOaksLab))
	}

	// 10. The EVENT_BATTLED_RIVAL_IN_OAKS_LAB flag is set by the battle-end
	//     script BEFORE that script finishes unwinding: measured on the real
	//     ROM, wJoyIgnore is still 0x00f0 the frame the flag lands, and the
	//     post-battle dialogue is still ahead. So wait for the flag AND for
	//     control to come back (wJoyIgnore == 0, no text box), advancing the
	//     text with A as it appears, then assert the positive facts.
	mem = advanceUntil(m, battleWaitBudget, func(mm *state.Mem) bool {
		return state.HasEvent(mm, state.EventBattledRivalInOaksLab) && state.Controllable(mm)
	})
	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, state.EventBattledRivalInOaksLab) {
		return fmt.Errorf("skill: GetStarter: %s not set after the battle: map=%#04x at (%d,%d) wJoyIgnore=%#04x",
			state.EventBattledRivalInOaksLab, mem.U8(sym.CurMap), mem.U8(sym.XCoord),
			mem.U8(sym.YCoord), mem.U8(sym.JoyIgnore))
	}
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: GetStarter: not controllable after the battle: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	return nil
}

// advanceUntil steps frames, pressing A while a text box is up, until pred
// holds or the frame budget is exhausted. It returns the final snapshot. It
// is the overworld counterpart of Cutscene for predicates that must be
// checked while text is being advanced.
func advanceUntil(m *emu.Emu, budget int, pred func(*state.Mem) bool) state.Mem {
	var mem state.Mem
	for i := 0; i < budget; i++ {
		state.Snapshot(m, &mem)
		if pred(&mem) {
			return mem
		}
		if mem.U8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
		} else {
			m.StepFrame()
		}
	}
	state.Snapshot(m, &mem)
	return mem
}

// chooseStarterBall presses A on the ball the player is facing and answers
// YES (menu index 0) to the YesNoChoice menu. It returns an error if the
// menu does not appear within the budget.
func chooseStarterBall(m *emu.Emu) error {
	m.Tap(emu.A, 3, 7)
	var mem state.Mem
	for i := 0; i < choiceWaitBudget; i++ {
		state.Snapshot(m, &mem)
		if yesNoMenuUp(&mem) {
			if err := SelectMenuItem(m, 0); err != nil {
				return fmt.Errorf("skill: GetStarter: select YES: %w", err)
			}
			return nil
		}
		if mem.U8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
		} else {
			m.StepFrame()
		}
	}
	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: GetStarter: YesNoChoice menu did not appear within %d frames: map=%#04x at (%d,%d) wFontLoaded=%#04x wJoyIgnore=%#04x menu=%+v",
		choiceWaitBudget, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
		mem.U8(sym.FontLoaded), mem.U8(sym.JoyIgnore), state.DecodeMenu(&mem))
}

// yesNoMenuUp reports whether the YES/NO choice box is drawn: a text box is
// up and wMaxMenuItem carries the yes/no shape (highest valid index 1,
// inclusive; index 0 = YES, index 1 = NO). wMaxMenuItem is stale-0 until the
// choice box writes it, so the shape is a positive identifier in this flow.
func yesNoMenuUp(mem *state.Mem) bool {
	return mem.U8(sym.FontLoaded) != 0 && state.DecodeMenu(mem).Max == 1
}

// walkLab walks within Oak's lab to (tx,ty), re-planning around dynamic
// obstacles (NPC sprites) the static grid does not model. It returns the
// wrapped WalkPath error when a step stays blocked after all retries.
func walkLab(m *emu.Emu, romData []byte, tx, ty int, blocked map[[2]int]bool) error {
	if cur := m.Peek8(sym.CurMap); cur != oaksLabMap {
		return fmt.Errorf("skill: GetStarter: walkLab: on map %#04x, want map %#04x", cur, oaksLabMap)
	}
	h, err := rom.ParseMap(romData, oaksLabMap)
	if err != nil {
		return fmt.Errorf("skill: GetStarter: parse map %#04x: %w", oaksLabMap, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return fmt.Errorf("skill: GetStarter: build map %#04x: %w", oaksLabMap, err)
	}

	const maxRetries = 4
	for attempt := 0; ; attempt++ {
		x, y := playerXY(m)
		steps, err := world.FindPath(grid, int(x), int(y), tx, ty, blocked)
		if err != nil {
			return fmt.Errorf("skill: GetStarter: no path on map %#04x from (%d,%d) to (%d,%d): %w",
				oaksLabMap, x, y, tx, ty, err)
		}
		if err := WalkPath(m, steps); err != nil {
			var eb *ErrBlocked
			if errors.As(err, &eb) {
				if attempt >= maxRetries {
					return fmt.Errorf("skill: GetStarter: blocked on map %#04x at (%d,%d) after %d retries: %w",
						oaksLabMap, eb.At.X, eb.At.Y, maxRetries, err)
				}
				blocked[[2]int{int(eb.At.X) + eb.Step.DX, int(eb.At.Y) + eb.Step.DY}] = true
				continue
			}
			return fmt.Errorf("skill: GetStarter: walk on map %#04x at (%d,%d): %w", oaksLabMap, x, y, err)
		}
		return nil
	}
}

// labBlockedSet is the blocked set for Oak's lab: the rival at (4,3) and Oak
// at (5,2) block tiles world.Grid does not model, and the three ball tiles
// at (6,3), (7,3), (8,3) are objects, not floor.
func labBlockedSet() map[[2]int]bool {
	blocked := make(map[[2]int]bool, len(labNPCBlocked)+3)
	for _, p := range labNPCBlocked {
		blocked[[2]int{p[0], p[1]}] = true
	}
	for x := 6; x <= 8; x++ {
		blocked[[2]int{x, 3}] = true
	}
	return blocked
}
