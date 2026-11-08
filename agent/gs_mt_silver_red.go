package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsRedStandX uint8 = 9
	gsRedStandY uint8 = 11
	gsRedX      uint8 = 9
	gsRedY      uint8 = 10
)

// executeGSMtSilverPermit talks to Professor Oak in Pallet Town after all
// sixteen badges. His real EVENT_OPENED_MT_SILVER unlocks the west exit of
// Victory Road Gate. The Hall of Fame and eight-Kanto-badge flags alone do
// not bypass that officer.
func executeGSMtSilverPermit(m *emu.Emu, romData []byte) error {
	const label = "kanto:mt-silver-oak-permit"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressMtSilverOpened, gsprofile.ProgressKantoAllBadgesEarned, label)
	if err != nil || done {
		return err
	}
	return gsKantoStoryTalk(m, romData, profile, gsprofile.ProgressMtSilverOpened,
		"OAKS_LAB", 4, 3, 4, 2, label)
}

// gsRedBattleAndEnding owns only the fixed Red encounter in Mt. Silver Room
// 3. Do not wait for ordinary overworld control after victory: retail Red's
// script sets EVENT_RED_IN_MT_SILVER while he disappears, then runs credits.
// The objective's semantic postcondition verifies this exact event change.
func gsRedBattleAndEnding(m *emu.Emu, profile *gsprofile.Profile, room uint16) error {
	const encounter = "kanto:mt-silver:red"
	start := m.FrameCount()
	presses := 0
	for m.FrameCount()-start < gsSecondBadgeScriptFrameBudget {
		if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressMtSilverRedDefeated) {
			return nil
		}
		world := profile.DecodeOverworld(m)
		if world.NativeMapID != room {
			return fmt.Errorf("%w: Red ending left Mt. Silver room for map %#04x before the disappearance event",
				errGSSecondBadgeUnexpectedState, world.NativeMapID)
		}
		if world.InBattle {
			battle, ok := profile.DecodeBattleState(m)
			if !ok || battle.Kind != game.BattleTrainer {
				return fmt.Errorf("%w: Red encounter is not an observed trainer battle",
					errGSSecondBadgeUnexpectedState)
			}
			result, err := skill.Battle(m, skill.StatAwareMove(m.ROM()))
			if err != nil {
				return fmt.Errorf("gen2 Red: battle: %w", err)
			}
			if err := skill.RequireTrainerBattleWin(encounter, result); err != nil {
				return err
			}
			continue
		}
		if world.InDialogue && world.MovementIdle {
			if presses >= gsSecondBadgeMaxScriptPresses {
				return fmt.Errorf("%w: Red ending exceeded %d dialogue presses",
					errGSSecondBadgeStalled, gsSecondBadgeMaxScriptPresses)
			}
			m.Tap(emu.A, 3, 7)
			presses++
			continue
		}
		m.StepFrame()
	}
	return fmt.Errorf("%w: Red ending exceeded %d frames without a disappearance event",
		errGSSecondBadgeStalled, gsSecondBadgeScriptFrameBudget)
}

// executeGSMtSilverRed routes through the retail west Victory Road gate,
// Route 28 and the three Mt. Silver cave rooms, then challenges Red himself.
// The native pathfinder and battle driver must prove each transition. If an
// intermediate gate or cave route is missing, the resulting objective failure
// provides a reproducible checkpoint for the autofixer instead of teleporting.
func executeGSMtSilverRed(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("%w: nil Gold/Silver emulator for Red", errGSSecondBadgeUnexpectedState)
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressMtSilverRedDefeated) {
		return nil
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressMtSilverOpened) {
		return fmt.Errorf("%w: Red requires Oak's retail Mt. Silver unlock event",
			errGSSecondBadgeUnexpectedState)
	}
	room, err := gsOpeningMapID("SILVER_CAVE_ROOM_3")
	if err != nil {
		return err
	}
	world := profile.DecodeOverworld(m)
	if world.NativeMapID == room && !world.Controllable {
		return gsRedBattleAndEnding(m, profile, room)
	}
	if !world.Controllable {
		return fmt.Errorf("%w: Red approach resumed from an unknown scripted map %#04x",
			errGSSecondBadgeUnexpectedState, world.NativeMapID)
	}
	if err := gsSecondBadgeFaceFrom(m, romData, profile,
		skill.ExactNativeDestination(room, gsRedStandX, gsRedStandY),
		gsRedX, gsRedY, "kanto:mt-silver:red"); err != nil {
		return fmt.Errorf("gen2 Red: reach summit encounter: %w", err)
	}
	m.StepFrames(gsLakeScriptSettleFrames)
	m.Tap(emu.A, 3, 7)
	return gsRedBattleAndEnding(m, profile, room)
}
