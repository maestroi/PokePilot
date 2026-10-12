package skill

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

var softResetButtons = []emu.Button{emu.A, emu.B, emu.Start, emu.Select}

const softResetBudget = 12_000

// initSpritesDisabled is wUpdateSpritesEnabled after Init. LoadMapData sets
// it to 1 once the continued save's map is in VRAM. Navigation must not treat
// the post-Init title/main-menu boundary as a walkable overworld location:
// CurMap can still read as the pre-reset map while sprites stay $ff
// (run-d6dokr184ky81: Hall of Fame ending's jp Init left the agent "on"
// map 0x76 with no graph route out of Lance's north pocket).
const initSpritesDisabled uint8 = 0xff

// softResetToSave performs Red's A+B+START+SELECT reset when needed and
// CONTINUEs from the last save into a loaded overworld. Callers that are
// already post-Init (sprites == $ff) skip the button hold and only resume.
func softResetToSave(m *emu.Emu) error {
	if m == nil {
		return fmt.Errorf("skill: soft reset: nil emu")
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if mem.U8(sym.UpdateSpritesEnabled) != initSpritesDisabled {
		for _, b := range softResetButtons {
			m.Press(b)
		}
		_, err := m.StepUntil(softResetBudget, func(em *emu.Emu) bool {
			return em.Peek8(sym.UpdateSpritesEnabled) == initSpritesDisabled
		})
		for _, b := range softResetButtons {
			m.Release(b)
		}
		if err != nil {
			return fmt.Errorf("skill: soft reset: %w", err)
		}
	}
	return continueFromTitle(m)
}

// continueFromTitle advances title/intro to the main menu, selects CONTINUE,
// and waits until LoadMapData has enabled sprites on a controllable overworld.
func continueFromTitle(m *emu.Emu) error {
	if err := reachMainMenu(m, softResetBudget); err != nil {
		return err
	}
	m.Tap(emu.A, 3, 7)
	// DisplayContinueGameInfo waits for A, then SpecialEnterMap idles with the
	// saved map in wCurMap but not loaded, so Controllable alone can read true
	// while input is still dropped. sprites == 1 is the EnterMap proof.
	return tapUntil(m, softResetBudget, func(mem *state.Mem) bool {
		return mem.U8(sym.UpdateSpritesEnabled) == 1 &&
			state.Controllable(mem) && state.DecodeBattle(mem) == nil
	}, emu.A, "continued overworld")
}

// ensureNavigableOverworld normalizes start boundaries that look like a map
// coordinate but are not routable overworld control:
//
//   - post-Init title/main-menu (sprites == $ff, no battle, not controllable),
//     including Hall of Fame's scripted jp Init exit;
//   - Hall of Fame after the durable completion bit is set but before that
//     soft-reset, where the world graph has no Kanto route (Lance's Champion
//     entrance is a disconnected north pocket).
//
// sprites == $ff is NOT on its own proof of the title/main-menu boundary:
//
//   - a live battle also disables overworld sprites, so a resumed attempt that
//     stopped inside a wild encounter reads $ff too. Soft-resetting there would
//     hunt for a main menu that is a battle screen and fail ("main menu did not
//     appear") with a non-ErrBattle error the Travel loop cannot resolve;
//   - PromoteToLead / party-menu paths can leave sprites == $ff while
//     Controllable still reads true (run-2f5bbo4uwzhnv5w20i59sq17l after
//     prepareChallengeLead). Soft-resetting then Start/A-hunts into the START
//     menu and Pokémon status screen ("EXP POINTS" / "LEVEL UP") instead of
//     CONTINUE/NEW GAME.
//
// Guard the generic $ff signal with "no battle" and "not controllable" so a
// live battle falls through to GoTo's abortIfBattle (ErrBattle) and a
// controllable leftover menu boundary is left for ordinary navigation. The
// Hall of Fame completion egress stays an explicit second case: Controllable
// can read true there with a stale CurMap and no Kanto route.
func ensureNavigableOverworld(m *emu.Emu) error {
	if m == nil {
		return nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	needsResume := (mem.U8(sym.UpdateSpritesEnabled) == initSpritesDisabled &&
		state.DecodeBattle(&mem) == nil &&
		!state.Controllable(&mem)) ||
		(mem.U8(sym.CurMap) == hallOfFameMap && state.MainStoryComplete(&mem))
	if !needsResume {
		return nil
	}
	if err := softResetToSave(m); err != nil {
		return fmt.Errorf("skill: resume navigable overworld: %w", err)
	}
	state.Snapshot(m, &mem)
	if mem.U8(sym.UpdateSpritesEnabled) != 1 {
		return fmt.Errorf("skill: resume navigable overworld: sprites=%#02x after CONTINUE, want 1", mem.U8(sym.UpdateSpritesEnabled))
	}
	if mem.U8(sym.CurMap) == hallOfFameMap {
		return fmt.Errorf("skill: resume navigable overworld: still on Hall of Fame after CONTINUE")
	}
	return nil
}

// reachMainMenu taps Start and A through title/intro until CONTINUE/NEW GAME
// is on screen. Start alone is not enough on every intro frame; measured on
// the post-Hall-of-Fame Init boundary for run-d6dokr184ky81.
func reachMainMenu(m *emu.Emu, budget int) error {
	var mem state.Mem
	for spent := 0; spent < budget; spent += talkSettle {
		state.Snapshot(m, &mem)
		if mainMenuScreen(&mem) {
			return nil
		}
		m.Tap(emu.Start, 3, 7)
		m.StepFrames(talkSettle / 2)
		state.Snapshot(m, &mem)
		if mainMenuScreen(&mem) {
			return nil
		}
		m.Tap(emu.A, 3, 7)
		m.StepFrames(talkSettle / 2)
	}
	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: main menu did not appear: screen=%q", state.ScreenText(&mem))
}

// tapUntil taps btn between checks until pred holds. Pred is checked first so
// the tap never reaches the surface it was waiting for.
func tapUntil(m *emu.Emu, budget int, pred func(*state.Mem) bool, btn emu.Button, what string) error {
	var mem state.Mem
	for spent := 0; spent < budget; spent += talkSettle {
		state.Snapshot(m, &mem)
		if pred(&mem) {
			return nil
		}
		m.Tap(btn, 3, 7)
		m.StepFrames(talkSettle)
	}
	return fmt.Errorf("skill: %s did not appear: screen=%q", what, state.ScreenText(&mem))
}

func mainMenuScreen(mem *state.Mem) bool {
	text := strings.ToUpper(state.ScreenText(mem))
	return strings.Contains(text, "CONTINUE") && strings.Contains(text, "NEW GAME") &&
		mem.U8(sym.TopMenuItemX) == 1 && mem.U8(sym.TopMenuItemY) == 2 && mem.U8(sym.CurrentMenuItem) == 0
}
