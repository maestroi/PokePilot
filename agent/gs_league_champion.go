package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	// LancesRoom.asm: (4,5) is the left scripted approach tile. Lance is
	// at (5,2); he is not an ordinary face-and-talk object battle.
	gsChampionApproachX  uint8 = 4
	gsChampionApproachY  uint8 = 5
	gsChampionAttempts         = 4
	gsChampionIdleFrames       = 75
)

var errGSChampionSceneIdle = errors.New("gen2 League Champion script did not start")

// gsLeagueChampionAdvance handles the one-way retail Champion + Hall script.
// Unlike the ordinary dialogue driver, it stops at the requested cartridge
// event even if the ending cutscene is still running. Waiting for overworld
// control would incorrectly hang in the Hall of Fame and credits.
func gsLeagueChampionAdvance(m *emu.Emu, profile *gsprofile.Profile, goal game.ProgressID, lance, hall uint16) error {
	start := m.FrameCount()
	presses := 0
	idle := 0
	const encounter = "league:champion-lance"
	for m.FrameCount()-start < gsSecondBadgeScriptFrameBudget {
		if gsFirstBadgeProgressComplete(profile, m, goal) {
			return nil
		}
		world := profile.DecodeOverworld(m)
		if world.NativeMapID != lance && world.NativeMapID != hall {
			return fmt.Errorf("%w: Champion scene left League map for %#04x before %s",
				errGSSecondBadgeUnexpectedState, world.NativeMapID, goal)
		}
		if world.InBattle {
			battle, ok := profile.DecodeBattleState(m)
			if !ok || battle.Kind != game.BattleTrainer {
				return fmt.Errorf("%w: Lance battle is not decoded as a trainer encounter", errGSSecondBadgeUnexpectedState)
			}
			result, err := skill.Battle(m, skill.StatAwareMove(m.ROM()))
			if err != nil {
				return fmt.Errorf("gen2 Champion Lance battle: %w", err)
			}
			if err := skill.RequireTrainerBattleWin(encounter, result); err != nil {
				return err
			}
			idle = 0
			continue
		}
		if world.InDialogue && world.MovementIdle {
			if presses >= gsSecondBadgeMaxScriptPresses {
				return fmt.Errorf("%w: Champion dialogue exceeded %d presses", errGSSecondBadgeStalled, gsSecondBadgeMaxScriptPresses)
			}
			m.Tap(emu.A, 3, 7)
			presses++
			idle = 0
			continue
		}
		if world.Controllable {
			idle++
			if idle >= gsChampionIdleFrames {
				return fmt.Errorf("%w: map %#04x at (%d,%d) before %s",
					errGSChampionSceneIdle, world.NativeMapID, world.X, world.Y, goal)
			}
		} else {
			idle = 0
		}
		m.StepFrame()
	}
	return fmt.Errorf("%w: Champion/Hall scene exceeded %d frames before %s",
		errGSSecondBadgeStalled, gsSecondBadgeScriptFrameBudget, goal)
}

// executeGSLeagueChampion deliberately exposes two different milestones:
// the resettable EVENT_BEAT_CHAMPION_LANCE and EVENT_BEAT_ELITE_FOUR, which
// HallOfFame.asm sets only after Lance has escorted the player to the machine.
// No map arrival or battle victory is allowed to manufacture the latter.
//
// When called for Hall of Fame, it resumes the in-flight post-Lance scene
// without routing, returning to a Center, or pressing into the title screen.
func executeGSLeagueChampion(m *emu.Emu, romData []byte, goal game.ProgressID) error {
	if goal != gsprofile.ProgressLeagueLanceDefeated && goal != gsprofile.ProgressLeagueHallOfFameEntered {
		return fmt.Errorf("%w: unknown Champion goal %s", errGSControllerUnavailable, goal)
	}
	if m == nil {
		return fmt.Errorf("%w: nil League emulator", errGSSecondBadgeUnexpectedState)
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	complete := func(progress game.ProgressID) bool {
		return gsFirstBadgeProgressComplete(profile, m, progress)
	}
	if complete(goal) {
		return nil
	}
	lance, err := gsOpeningMapID("LANCES_ROOM")
	if err != nil {
		return err
	}
	hall, err := gsOpeningMapID("HALL_OF_FAME")
	if err != nil {
		return err
	}
	karen, err := gsOpeningMapID("KARENS_ROOM")
	if err != nil {
		return err
	}
	if goal == gsprofile.ProgressLeagueHallOfFameEntered {
		if !complete(gsprofile.ProgressLeagueLanceDefeated) {
			return fmt.Errorf("%w: Hall of Fame requires the real Lance victory event", errGSSecondBadgeUnexpectedState)
		}
		here := profile.DecodeOverworld(m).NativeMapID
		if here != lance && here != hall {
			return fmt.Errorf("%w: incomplete Hall of Fame ceremony on map %#04x", errGSSecondBadgeUnexpectedState, here)
		}
		if err := gsLeagueChampionAdvance(m, profile, goal, lance, hall); err != nil {
			return fmt.Errorf("gen2 Hall of Fame ceremony: %w", err)
		}
		return nil
	}

	if !complete(gsprofile.ProgressLeagueKarenDefeated) {
		return fmt.Errorf("%w: Champion Lance requires Karen's post-battle flag", errGSSecondBadgeUnexpectedState)
	}
	here := profile.DecodeOverworld(m).NativeMapID
	if here != karen && here != lance {
		return fmt.Errorf("%w: Champion must start in Karen or Lance room, map %#04x", errGSSecondBadgeUnexpectedState, here)
	}
	if here == karen {
		if err := gsSecondBadgeGoTo(m, romData, profile, skill.NativeMapDestination(lance)); err != nil {
			return fmt.Errorf("gen2 Champion: cross Karen's opened north exit: %w", err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
	}
	// A checkpoint may be taken after the approach's coord_event started the
	// scripted battle. Resume it before attempting another movement command.
	if !profile.DecodeOverworld(m).Controllable {
		err := gsLeagueChampionAdvance(m, profile, goal, lance, hall)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errGSChampionSceneIdle) {
			return fmt.Errorf("gen2 Champion: resume automatic scene: %w", err)
		}
	}
	if complete(goal) {
		return nil
	}
	mem := skill.NewNativeRouteMemory()
	for attempt := 0; attempt < gsChampionAttempts; attempt++ {
		err := skill.GoToNativeRemembering(m, romData,
			skill.ExactNativeDestination(lance, gsChampionApproachX, gsChampionApproachY), mem)
		if err != nil && !errors.Is(err, skill.ErrDialogueInterrupted) && !errors.Is(err, skill.ErrBattle) {
			return fmt.Errorf("gen2 Champion: walk to Lance's script trigger: %w", err)
		}
		// The coord_event at (4,5) starts Lance's mandatory trainer fight.
		// Script setup may lag movement by a few frames; never declare
		// success until the exact retail event appears.
		if err := gsLeagueChampionAdvance(m, profile, goal, lance, hall); err != nil {
			if errors.Is(err, errGSChampionSceneIdle) {
				continue
			}
			return fmt.Errorf("gen2 Champion: battle and post-battle scene: %w", err)
		}
		return nil
	}
	return fmt.Errorf("%w: Lance's scripted encounter never set its victory flag", errGSSecondBadgeUnexpectedState)
}
