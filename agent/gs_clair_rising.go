package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/gs/sym"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsIcePathWaterfallX  uint8 = 31
	gsIcePathWaterfallY  uint8 = 7
	gsClairX             uint8 = 5
	gsClairY             uint8 = 3
	gsClairStandX        uint8 = 5
	gsClairStandY        uint8 = 4
	gsDragonFangX        uint8 = 35
	gsDragonFangY        uint8 = 16
	gsDragonFangStandX   uint8 = 35
	gsDragonFangStandY   uint8 = 17
	gsClairStoryAttempts       = 4
)

// executeGSIcePathWaterfall approaches the retail item ball on 1F through
// Route 44. Tile 0x23 slides and Strength pit warps are owned by the shared
// native router, not skipped by a map-level arrival or synthetic teleport.
// HM07 is optional in the game, but its native event is a useful durable
// checkpoint before entering the Blackthorn Gym.
func executeGSIcePathWaterfall(m *emu.Emu, romData []byte) error {
	const label = "ice-path:hm07-waterfall"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressIcePathWaterfallAcquired,
		gsprofile.ProgressRadioTowerTakeoverSettled, label)
	if err != nil || done {
		return err
	}
	if _, err := skill.EnsureFieldMove(m, skill.FieldStrength); err != nil {
		return fmt.Errorf("gen2 Ice Path: prepare HM04 Strength: %w", err)
	}
	for attempt := 0; attempt < gsClairStoryAttempts; attempt++ {
		if err := gsRadioScriptInteraction(m, romData, profile,
			"ICE_PATH_1F", 31, 8, gsIcePathWaterfallX, gsIcePathWaterfallY,
			label, func() bool {
				return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressIcePathWaterfallAcquired)
			}); err != nil {
			return fmt.Errorf("gen2 Ice Path: acquire HM07: %w", err)
		}
		if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressIcePathWaterfallAcquired) &&
			profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Ice Path HM07 item ball event missing", errGSSecondBadgeUnexpectedState)
}

// executeGSClair includes the live Ice Path traversal, the Blackthorn 2F
// Strength boulder holes and the real Clair battle. EVENT_BEAT_CLAIR is only
// a *battle* completion: it can never be used as Rising Badge completion.
// The ROM deliberately withholds that badge until Dragon's Den.
func executeGSClair(m *emu.Emu, romData []byte) error {
	const label = "gym:clair"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressClairDefeated,
		gsprofile.ProgressBlackthornGymBouldersDropped, label)
	if err != nil || done {
		return err
	}
	if _, err := skill.EnsureFieldMove(m, skill.FieldStrength); err != nil {
		return fmt.Errorf("gen2 Clair: prepare Strength for Ice Path and Gym: %w", err)
	}
	if err := gsCoastalHeal(m, romData, profile, "BLACKTHORN_POKECENTER_1F"); err != nil {
		return fmt.Errorf("gen2 Clair: cross Ice Path and heal in Blackthorn: %w", err)
	}
	if _, err := skill.EnsureFieldMove(m, skill.FieldStrength); err != nil {
		return fmt.Errorf("gen2 Clair: prepare Strength for Gym 2F: %w", err)
	}
	gym, err := gsOpeningMapID("BLACKTHORN_GYM_1F")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsClairStoryAttempts; attempt++ {
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(gym, gsClairStandX, gsClairStandY),
			gsClairX, gsClairY, label); err != nil {
			return fmt.Errorf("gen2 Clair: solve 2F boulders and approach leader: %w", err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 Clair: battle / Dragon's Den instruction: %w", err)
		}
		if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressClairDefeated) &&
			profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Clair battle never set EVENT_BEAT_CLAIR", errGSSecondBadgeUnexpectedState)
}

// executeGSDragonFangRising navigates the original Dragon's Den B1F with Surf
// and Whirlpool, interacts with the unique Dragon Fang object at (35,16),
// then finishes Clair's scripted award in-place. Reaching the Fang item or
// even taking it is not sufficient: only ENGINE_RISINGBADGE in wJohtoBadges
// confirms that all eight badges have actually been earned.
//
// The inventory has a hard 20-slot ordinary-item pocket. A full pocket can
// make retail giveitem DRAGON_FANG fail before Clair appears; we fail with a
// specific restartable diagnosis rather than repeating the script forever,
// editing RAM, or claiming badge success.
func executeGSDragonFangRising(m *emu.Emu, romData []byte) error {
	const label = "dragons-den:dragon-fang-rising"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRisingBadgeEarned,
		gsprofile.ProgressClairDefeated, label)
	if err != nil || done {
		return err
	}
	if !profile.DecodeOverworld(m).Controllable {
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 Dragon's Den: resume badge dialogue: %w", err)
		}
		if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRisingBadgeEarned) {
			return nil
		}
	}
	// Do not force another item interaction when the ordinary bag is full.
	// Dragon Fang lives in the Items pocket, not the independent key pocket.
	if int(m.Peek8(sym.NumItems)) >= sym.MaxItems {
		return fmt.Errorf("%w: Dragon Fang cannot be received with a full %d-slot Items pocket; free a slot and resume this checkpoint",
			errGSSecondBadgeUnexpectedState, sym.MaxItems)
	}
	if _, err := skill.EnsureFieldMove(m, skill.FieldSurf); err != nil {
		return fmt.Errorf("gen2 Dragon's Den: prepare Surf: %w", err)
	}
	if _, err := skill.EnsureFieldMove(m, skill.FieldWhirlpool); err != nil {
		return fmt.Errorf("gen2 Dragon's Den: prepare Whirlpool: %w", err)
	}
	if err := gsCoastalHeal(m, romData, profile, "BLACKTHORN_POKECENTER_1F"); err != nil {
		return fmt.Errorf("gen2 Dragon's Den: heal before Surf/Whirlpool: %w", err)
	}
	for attempt := 0; attempt < gsClairStoryAttempts; attempt++ {
		if err := gsRadioScriptInteraction(m, romData, profile,
			"DRAGONS_DEN_B1F", gsDragonFangStandX, gsDragonFangStandY,
			gsDragonFangX, gsDragonFangY, label,
			func() bool { return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRisingBadgeEarned) }); err != nil {
			return fmt.Errorf("gen2 Dragon's Den: claim Dragon Fang and finish Clair: %w", err)
		}
		if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRisingBadgeEarned) &&
			profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Dragon's Den returned without the retail Rising Badge bit", errGSSecondBadgeUnexpectedState)
}
