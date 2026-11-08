package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

const (
	gsPostCreditsFrameBudget uint64 = 240_000
	gsPostCreditsIdleFrames         = 120
)

// gsContinueAfterHall owns Gold/Silver's Hall of Fame -> saved-game CONTINUE
// transition. HallOfFame.asm sets EVENT_BEAT_ELITE_FOUR, then saves the
// cartridge's own battery data and runs the first, unskippable credits. Once
// the credits reset to the title, the retail main menu has CONTINUE first;
// selecting it spawns the player at New Bark via wSpawnAfterChampion.
//
// This is deliberately an *owned story transaction*, not generic boundary
// normalization: no other objective is authorized to press through the title
// menu. No emulator snapshot is loaded, no memory is patched, and the final
// Hall of Fame event must be re-observed after CONTINUE from the real save.
func gsContinueAfterHall(m *emu.Emu, profile *gsprofile.Profile) error {
	if m == nil || profile == nil {
		return fmt.Errorf("%w: missing Gold/Silver emulator or profile for post-credits CONTINUE",
			errGSSecondBadgeUnexpectedState)
	}
	hall, err := gsOpeningMapID("HALL_OF_FAME")
	if err != nil {
		return err
	}
	// Enter only with positive cartridge evidence. A title menu alone is
	// never enough: that could be a fresh game with an unrelated save.
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressLeagueHallOfFameEntered) {
		return fmt.Errorf("%w: cannot CONTINUE without observed Hall of Fame ceremony",
			errGSSecondBadgeUnexpectedState)
	}

	start := m.FrameCount()
	for m.FrameCount()-start < gsPostCreditsFrameBudget {
		world := profile.DecodeOverworld(m)
		// After the natural reset, the saved game's flag must still be set.
		// Hall itself can misleadingly look controllable while credits own
		// the display, so do not treat its stale map bytes as a return.
		if world.Controllable && world.NativeMapID != hall &&
			gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressLeagueHallOfFameEntered) {
			return nil
		}
		if world.InBattle {
			return fmt.Errorf("%w: battle during Hall of Fame CONTINUE", errGSSecondBadgeUnexpectedState)
		}
		menu := profile.DecodePostCreditsMenu(m)
		switch {
		case menu.ContinueVisible:
			// Row 1 is CONTINUE. Other entries include NEW GAME, which must
			// never be confirmed even if a stale cursor starts on row 2.
			if menu.Cursor > 1 {
				m.Tap(emu.Up, 3, 7)
			} else {
				m.Tap(emu.A, 3, 7)
			}
		case menu.SaveInfoVisible:
			m.Tap(emu.A, 3, 7) // ConfirmContinue in intro_menu.asm
		default:
			// Credits and title are noninteractive or accept harmless A.
			// Let the retail HallOfFame script finish saving, animating and
			// resetting naturally; A also opens the title once it appears.
			m.StepFrames(gsPostCreditsIdleFrames)
			m.Tap(emu.A, 3, 7)
		}
	}
	world := profile.DecodeOverworld(m)
	return fmt.Errorf("%w: post-credits CONTINUE exceeded %d frames, map=%#04x controllable=%v menu=%+v",
		errGSSecondBadgeStalled, gsPostCreditsFrameBudget,
		world.NativeMapID, world.Controllable, profile.DecodePostCreditsMenu(m))
}
