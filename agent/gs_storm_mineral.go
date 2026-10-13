package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsStrengthSailorX      uint8 = 4
	gsStrengthSailorY      uint8 = 3
	gsStrengthSailorStandX uint8 = 4
	gsStrengthSailorStandY uint8 = 4
	gsChuckX               uint8 = 4
	gsChuckY               uint8 = 1
	gsChuckStandX          uint8 = 4
	gsChuckStandY          uint8 = 2
	// object_const_def uses const_def 2 (player owns index 1), so the tenth
	// CianwoodCity object_event — Chuck's wife — is map-object 11, not 10.
	gsChucksWifeSlot   int   = 11
	gsChucksWifeStandX uint8 = 10
	gsChucksWifeStandY uint8 = 45
	gsJasmineGymX          uint8 = 5
	gsJasmineGymY          uint8 = 3
	gsJasmineGymStandX     uint8 = 5
	gsJasmineGymStandY     uint8 = 4
)

// gsCoastalProfile holds the cartridge-specific story state required by
// all coastal transactions. Pure objective dispatch stays game-agnostic.
func gsCoastalProfile(m *emu.Emu, romData []byte, goal game.ProgressID, previous game.ProgressID, encounter string) (*gsprofile.Profile, bool, error) {
	if m == nil {
		return nil, false, fmt.Errorf("gen2 %s: nil emulator", encounter)
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return nil, false, err
	}
	if gsFirstBadgeProgressComplete(profile, m, goal) {
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
				return nil, false, err
			}
		}
		return profile, true, nil
	}
	if !gsFirstBadgeProgressComplete(profile, m, previous) {
		return nil, false, fmt.Errorf("%w: %s requires %s", errGSSecondBadgeUnexpectedState, goal, previous)
	}
	return profile, false, nil
}

// gsCoastalHeal establishes a verified local recovery point, never assuming
// that navigating into a Center has healed the party.
func gsCoastalHeal(m *emu.Emu, romData []byte, profile *gsprofile.Profile, centerName string) error {
	center, err := gsOpeningMapID(centerName)
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile,
		skill.ExactNativeDestination(center, gsPokecenterStandX, gsPokecenterStandY)); err != nil {
		return fmt.Errorf("reach %s: %w", centerName, err)
	}
	if !profile.DecodeCenter(m).Recovered {
		if err := skill.Face(m, gsPokecenterCounterX, gsPokecenterCounterY); err != nil {
			return fmt.Errorf("face %s nurse: %w", centerName, err)
		}
		if err := skill.Heal(m); err != nil {
			return fmt.Errorf("heal at %s: %w", centerName, err)
		}
	}
	if !profile.DecodeCenter(m).Recovered {
		return fmt.Errorf("%w: %s returned without recovered party", errGSSecondBadgeUnexpectedState, centerName)
	}
	return nil
}

// executeGSHM04Strength obtains the machine from the sailor in Olivine Café.
// It is needed to open Chuck's Gym's real boulder barrier. The receipt event
// is durable and distinct from teaching or using the move.
func executeGSHM04Strength(m *emu.Emu, romData []byte) error {
	const encounter = "olivine:strength"
	profile, done, err := gsCoastalProfile(m, romData, gsprofile.ProgressHM04StrengthAcquired,
		gsprofile.ProgressJasmineReturnedToGym, encounter)
	if err != nil || done {
		return err
	}
	cafe, err := gsOpeningMapID("OLIVINE_CAFE")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeFaceFrom(m, romData, profile,
		skill.ExactNativeDestination(cafe, gsStrengthSailorStandX, gsStrengthSailorStandY),
		gsStrengthSailorX, gsStrengthSailorY, encounter); err != nil {
		return fmt.Errorf("gen2 Strength: reach sailor: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
		return fmt.Errorf("gen2 Strength: handoff: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressHM04StrengthAcquired) ||
		!profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Olivine sailor did not award HM04 Strength", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

// executeGSChuck uses the shared native Strength puzzle solver for the
// actual three-boulder wall, rather than teleporting through it or declaring
// the gym cleared because a dialogue completed. Gym trainers and Chuck's
// scripted boulder throw are settled by the owned interruption driver.
func executeGSChuck(m *emu.Emu, romData []byte) error {
	const encounter = "gym:chuck"
	profile, done, err := gsCoastalProfile(m, romData, gsprofile.ProgressStormBadgeEarned,
		gsprofile.ProgressHM04StrengthAcquired, encounter)
	if err != nil || done {
		return err
	}
	if err := gsCoastalHeal(m, romData, profile, "CIANWOOD_POKECENTER_1F"); err != nil {
		return fmt.Errorf("gen2 Chuck: recover before gym: %w", err)
	}
	if _, err := skill.EnsureFieldMove(m, skill.FieldStrength); err != nil {
		return fmt.Errorf("gen2 Chuck: prepare Strength before the boulder barrier: %w", err)
	}
	gym, err := gsOpeningMapID("CIANWOOD_GYM")
	if err != nil {
		return err
	}
	dest := skill.ExactNativeDestination(gym, gsChuckStandX, gsChuckStandY)
	if err := gsSecondBadgeGoTo(m, romData, profile, dest); err != nil {
		return fmt.Errorf("gen2 Chuck: solve gym boulder barrier: %w", err)
	}
	if !profile.DecodeCenter(m).Recovered {
		if err := gsCoastalHeal(m, romData, profile, "CIANWOOD_POKECENTER_1F"); err != nil {
			return fmt.Errorf("gen2 Chuck: recover after gym trainers: %w", err)
		}
		if _, err := skill.EnsureFieldMove(m, skill.FieldStrength); err != nil {
			return fmt.Errorf("gen2 Chuck: re-prepare Strength: %w", err)
		}
		if err := gsSecondBadgeGoTo(m, romData, profile, dest); err != nil {
			return fmt.Errorf("gen2 Chuck: return past boulders: %w", err)
		}
	}
	if err := skill.Face(m, gsChuckX, gsChuckY); err != nil {
		return fmt.Errorf("gen2 Chuck: face leader: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
		return fmt.Errorf("gen2 Chuck: battle and badge script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressStormBadgeEarned) ||
		!profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Chuck battle returned without Storm Badge", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

// executeGSHM02Fly speaks to Chuck's wife, a moving NPC outdoors. The
// profile decodes her live location via her map-object index (const_def 2),
// rather than assuming she stays at the object-event spawn coordinate. The
// encounter remains bounded to her known (x=9..11, y=46) WALK_LEFT_RIGHT
// range, and completion requires EVENT_GOT_HM02_FLY.
func executeGSHM02Fly(m *emu.Emu, romData []byte) error {
	const encounter = "cianwood:hm02-fly"
	profile, done, err := gsCoastalProfile(m, romData, gsprofile.ProgressHM02FlyAcquired,
		gsprofile.ProgressStormBadgeEarned, encounter)
	if err != nil || done {
		return err
	}
	city, err := gsOpeningMapID("CIANWOOD_CITY")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile,
		skill.ExactNativeDestination(city, gsChucksWifeStandX, gsChucksWifeStandY)); err != nil {
		return fmt.Errorf("gen2 HM02 Fly: approach Chuck's wife: %w", err)
	}
	for i := 0; i < 240; i++ {
		overworld := profile.DecodeOverworld(m)
		if !overworld.Controllable || overworld.NativeMapID != city {
			return fmt.Errorf("%w: Chuck's wife interaction lost owned city control", errGSSecondBadgeUnexpectedState)
		}
		live, err := profile.DecodeLiveTopology(m)
		if err != nil {
			return err
		}
		wife, ok := live.ObjectPositions[gsChucksWifeSlot]
		if !ok || wife.Y != 46 || wife.X < 9 || wife.X > 11 {
			return fmt.Errorf("%w: Chuck's wife is missing from known roaming range", errGSSecondBadgeUnexpectedState)
		}
		delta := int(overworld.X) - wife.X
		if delta < 0 {
			delta = -delta
		}
		if delta+int(wife.Y)-int(overworld.Y) == 1 {
			if err := skill.Face(m, uint8(wife.X), uint8(wife.Y)); err != nil {
				return fmt.Errorf("gen2 HM02 Fly: face wife: %w", err)
			}
			m.Tap(emu.A, 3, 7)
			if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
				return fmt.Errorf("gen2 HM02 Fly: reward script: %w", err)
			}
			if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressHM02FlyAcquired) ||
				!profile.DecodeOverworld(m).Controllable {
				return fmt.Errorf("%w: Chuck's wife returned without HM02 Fly", errGSSecondBadgeUnexpectedState)
			}
			return nil
		}
		m.StepFrames(8)
	}
	return fmt.Errorf("%w: Chuck's wife never approached the owned interaction tile", errGSSecondBadgeStalled)
}

// executeGSJasmineBadge ends the coastal slice at Olivine Gym. The
// engine's Mineral Badge byte is checked independently from the trainer's
// pre-battle event, which can be set before the victory sequence settles.
func executeGSJasmineBadge(m *emu.Emu, romData []byte) error {
	const encounter = "gym:jasmine"
	profile, done, err := gsCoastalProfile(m, romData, gsprofile.ProgressMineralBadgeEarned,
		gsprofile.ProgressHM02FlyAcquired, encounter)
	if err != nil || done {
		return err
	}
	if err := gsCoastalHeal(m, romData, profile, "OLIVINE_POKECENTER_1F"); err != nil {
		return fmt.Errorf("gen2 Jasmine: recover before gym: %w", err)
	}
	gym, err := gsOpeningMapID("OLIVINE_GYM")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeFaceFrom(m, romData, profile,
		skill.ExactNativeDestination(gym, gsJasmineGymStandX, gsJasmineGymStandY),
		gsJasmineGymX, gsJasmineGymY, encounter); err != nil {
		return fmt.Errorf("gen2 Jasmine: reach leader: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
		return fmt.Errorf("gen2 Jasmine: battle and badge script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressMineralBadgeEarned) ||
		!profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Jasmine battle returned without Mineral Badge", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
