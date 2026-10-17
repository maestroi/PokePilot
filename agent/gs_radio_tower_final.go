package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsRadioTowerTrainerAttempts       = 3
	gsRadioTowerExecutiveM2X    uint8 = 14
	gsRadioTowerExecutiveM2Y    uint8 = 1
	gsRadioTowerExecutiveF1X    uint8 = 17
	gsRadioTowerExecutiveF1Y    uint8 = 2
	gsRadioTowerBossSceneX      uint8 = 16
	gsRadioTowerBossSceneY      uint8 = 5
)

// gsRadioTowerTrainer owns a trainer's normal line-of-sight or manual
// interaction. The retail trainer-flag bit, not reaching the floor or seeing
// a battle, proves completion. An approach can itself trigger the trainer;
// in that case do not talk to a defeated NPC a second time.
func gsRadioTowerTrainer(m *emu.Emu, romData []byte, label, floor string,
	previous, goal game.ProgressID, standX, standY, targetX, targetY uint8,
) error {
	profile, done, err := gsCoastalProfile(m, romData, goal, previous, label)
	if err != nil || done {
		return err
	}
	floorID, err := gsOpeningMapID(floor)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsRadioTowerTrainerAttempts; attempt++ {
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
				return fmt.Errorf("gen2 Radio Tower %s: resume battle/script: %w", label, err)
			}
		}
		if gsFirstBadgeProgressComplete(profile, m, goal) && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if err := gsSecondBadgeGoTo(m, romData, profile,
			skill.ExactNativeDestination(floorID, standX, standY)); err != nil {
			return fmt.Errorf("gen2 Radio Tower %s: route to trainer: %w", label, err)
		}
		if gsFirstBadgeProgressComplete(profile, m, goal) && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if err := skill.Face(m, targetX, targetY); err != nil {
			return fmt.Errorf("gen2 Radio Tower %s: face trainer: %w", label, err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 Radio Tower %s: settle trainer battle: %w", label, err)
		}
		if gsFirstBadgeProgressComplete(profile, m, goal) && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Radio Tower %s retail trainer event absent after battle",
		errGSSecondBadgeUnexpectedState, label)
}

// RadioTower4F.asm: EXECUTIVEM_2 is a standing trainer at (14,1), guarding
// the Card-Key-shutter side of the tower.
func executeGSRadioTowerExecutiveM2(m *emu.Emu, romData []byte) error {
	return gsRadioTowerTrainer(m, romData, "radio-tower:4f-executive-m2",
		"RADIO_TOWER_4F", gsprofile.ProgressRadioTowerShutterOpened,
		gsprofile.ProgressRadioTowerExecutiveM2Beaten, 13, 1,
		gsRadioTowerExecutiveM2X, gsRadioTowerExecutiveM2Y)
}

// RadioTower5F.asm: EXECUTIVEF_1 is the (17,2) standing trainer, distinct
// from the B2F Rocket Hideout EXECUTIVEF_2 battle.
func executeGSRadioTowerExecutiveF1(m *emu.Emu, romData []byte) error {
	return gsRadioTowerTrainer(m, romData, "radio-tower:5f-executive-f1",
		"RADIO_TOWER_5F", gsprofile.ProgressRadioTowerExecutiveM2Beaten,
		gsprofile.ProgressRadioTowerExecutiveF1Beaten, 16, 2,
		gsRadioTowerExecutiveF1X, gsRadioTowerExecutiveF1Y)
}

// executeGSRadioTowerFinalBoss walks onto the RETAIL coordinate-triggered
// scene (16,5) on Radio Tower 5F after the executive fights. It drives the
// boss battle and the Director's Gold/Silver-specific wing dialogue. The
// temporary state with CLEARED_RADIO_TOWER=true but wing=false is deliberately
// NOT complete; a recoverable in-flight script is settled first.
//
// This is an unadvertised controller until both games pass real-ROM replay.
func executeGSRadioTowerFinalBoss(m *emu.Emu, romData []byte) error {
	const encounter = "radio-tower:5f-rocket-boss-and-director"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRadioTowerTakeoverSettled,
		gsprofile.ProgressRadioTowerExecutiveF1Beaten, encounter)
	if err != nil || done {
		return err
	}
	fifth, err := gsOpeningMapID("RADIO_TOWER_5F")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsRadioTowerTrainerAttempts; attempt++ {
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
				return fmt.Errorf("gen2 Radio Tower: settle boss/wing dialogue: %w", err)
			}
		}
		victory := profile.DecodeRadioTowerVictory(m)
		if victory.Complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		// The actual ROM sets this flag BEFORE awarding the wing. If the
		// player regains control without that wing event, replaying the
		// single-use boss scene would be an unsafe false recovery.
		if victory.Cleared {
			return fmt.Errorf("%w: Radio Tower boss cleared but Director wing script is incomplete (Gold/Silver reward missing)",
				errGSSecondBadgeUnexpectedState)
		}
		if err := gsSecondBadgeGoTo(m, romData, profile,
			skill.ExactNativeDestination(fifth,
				gsRadioTowerBossSceneX, gsRadioTowerBossSceneY)); err != nil {
			return fmt.Errorf("gen2 Radio Tower: approach 5F boss scene: %w", err)
		}
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
				return fmt.Errorf("gen2 Radio Tower: boss combat and Director gift: %w", err)
			}
		}
		if profile.DecodeRadioTowerVictory(m).Complete() &&
			profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Radio Tower finale did not finish boss, clear and version-specific wing events",
		errGSSecondBadgeUnexpectedState)
}
