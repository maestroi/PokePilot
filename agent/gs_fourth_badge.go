package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsFlowerShopTeacherX uint8 = 2
	gsFlowerShopTeacherY uint8 = 4
	gsFlowerShopStandX   uint8 = 3
	gsFlowerShopStandY   uint8 = 4

	gsSudowoodoX      uint8 = 35
	gsSudowoodoY      uint8 = 9
	gsSudowoodoStandX uint8 = 35
	gsSudowoodoStandY uint8 = 10

	gsBurnedTowerBeastsX uint8 = 9
	gsBurnedTowerBeastsY uint8 = 5

	gsMortyX      uint8 = 5
	gsMortyY      uint8 = 1
	gsMortyStandX uint8 = 5
	gsMortyStandY uint8 = 2
)

func gsRecoverAtEcruteakCenter(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	center, err := gsOpeningMapID("ECRUTEAK_POKECENTER_1F")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(center, gsPokecenterStandX, gsPokecenterStandY)); err != nil {
		return fmt.Errorf("reach Ecruteak Center: %w", err)
	}
	if profile.DecodeCenter(m).Recovered {
		return nil
	}
	if err := skill.Face(m, gsPokecenterCounterX, gsPokecenterCounterY); err != nil {
		return fmt.Errorf("face Ecruteak nurse: %w", err)
	}
	if err := skill.Heal(m); err != nil {
		return err
	}
	if !profile.DecodeCenter(m).Recovered {
		return fmt.Errorf("%w: Ecruteak Center returned control without a recovered party", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

// executeGSSquirtBottle claims the story item that opens Route 36. The Flower
// Shop teacher gates the handoff on ENGINE_PLAINBADGE, so the event bit is a
// durable postcondition and a resumed save never needs to replay Whitney.
func executeGSSquirtBottle(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 SquirtBottle: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSquirtBottleAcquired) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "goldenrod:squirtbottle")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressPlainBadgeEarned) {
		return fmt.Errorf("%w: SquirtBottle requires the Plain Badge", errGSSecondBadgeUnexpectedState)
	}

	shop, err := gsOpeningMapID("GOLDENROD_FLOWER_SHOP")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(shop, gsFlowerShopStandX, gsFlowerShopStandY)); err != nil {
		return fmt.Errorf("gen2 SquirtBottle: reach Flower Shop teacher: %w", err)
	}
	if err := skill.Face(m, gsFlowerShopTeacherX, gsFlowerShopTeacherY); err != nil {
		return fmt.Errorf("gen2 SquirtBottle: face teacher: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, "goldenrod:squirtbottle"); err != nil {
		return fmt.Errorf("gen2 SquirtBottle: handoff: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSquirtBottleAcquired) {
		return fmt.Errorf("%w: Flower Shop returned control without SquirtBottle", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

// executeGSSudowoodo clears the mandatory Route 36 blocker. Interacting with
// the weird tree opens the retail YES/NO surface; the owned interruption loop
// selects the default YES choice, resolves the static battle, and waits for
// EVENT_FOUGHT_SUDOWOODO before returning.
func executeGSSudowoodo(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Sudowoodo: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSudowoodoCleared) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "route36:sudowoodo")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSquirtBottleAcquired) {
		return fmt.Errorf("%w: Sudowoodo requires SquirtBottle", errGSSecondBadgeUnexpectedState)
	}
	if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
		return fmt.Errorf("gen2 Sudowoodo: recover party: %w", err)
	}

	route36, err := gsOpeningMapID("ROUTE_36")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(route36, gsSudowoodoStandX, gsSudowoodoStandY)); err != nil {
		return fmt.Errorf("gen2 Sudowoodo: reach blocker: %w", err)
	}
	if err := skill.Face(m, gsSudowoodoX, gsSudowoodoY); err != nil {
		return fmt.Errorf("gen2 Sudowoodo: face blocker: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, "route36:sudowoodo"); err != nil {
		return fmt.Errorf("gen2 Sudowoodo: battle/script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSudowoodoCleared) {
		return fmt.Errorf("%w: Sudowoodo battle returned without durable clear event", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

// executeGSBurnedTower follows the normal Ecruteak story detour. Entering 1F
// owns the rival scene; native routing then drops to B1F and walks onto the
// legendary-beast coordinate event. Completion is EVENT_RELEASED_THE_BEASTS,
// not the earlier rival event, so a lost/retried rival battle cannot fake it.
func executeGSBurnedTower(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Burned Tower: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressBurnedTowerCleared) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "ecruteak:burned-tower")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSudowoodoCleared) {
		return fmt.Errorf("%w: Burned Tower requires the Route 36 blocker cleared", errGSSecondBadgeUnexpectedState)
	}
	if err := gsRecoverAtEcruteakCenter(m, romData, profile); err != nil {
		return fmt.Errorf("gen2 Burned Tower: recover party: %w", err)
	}

	b1f, err := gsOpeningMapID("BURNED_TOWER_B1F")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(b1f, gsBurnedTowerBeastsX, gsBurnedTowerBeastsY)); err != nil {
		return fmt.Errorf("gen2 Burned Tower: reach beasts: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressBurnedTowerCleared) {
		return fmt.Errorf("%w: Burned Tower trigger returned without releasing the beasts", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

// executeGSMorty owns the fourth Johto gym through the Fog Badge. The native
// provider marks the two landing-only warp records inert while keeping the
// actual pit warps blocked, so ordinary live-grid routing follows the legal
// invisible-floor path without a hard-coded input script.
func executeGSMorty(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Morty: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressFogBadgeEarned) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "gym:morty")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressBurnedTowerCleared) {
		return fmt.Errorf("%w: Morty requires the Burned Tower story boundary", errGSSecondBadgeUnexpectedState)
	}
	if err := gsRecoverAtEcruteakCenter(m, romData, profile); err != nil {
		return fmt.Errorf("gen2 Morty: recover party: %w", err)
	}

	gym, err := gsOpeningMapID("ECRUTEAK_GYM")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(gym, gsMortyStandX, gsMortyStandY)); err != nil {
		return fmt.Errorf("gen2 Morty: reach leader: %w", err)
	}

	// Mandatory gym trainers can drain HP/PP. Recover once more before Morty,
	// then traverse the invisible floor from a clean party state.
	if !profile.DecodeCenter(m).Recovered {
		if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
			return fmt.Errorf("gen2 Morty: recover party before leader: %w", err)
		}
		if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(gym, gsMortyStandX, gsMortyStandY)); err != nil {
			return fmt.Errorf("gen2 Morty: return to leader: %w", err)
		}
	}

	if err := skill.Face(m, gsMortyX, gsMortyY); err != nil {
		return fmt.Errorf("gen2 Morty: face leader: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, "gym:morty"); err != nil {
		return fmt.Errorf("gen2 Morty: battle/script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressFogBadgeEarned) {
		return fmt.Errorf("%w: Morty returned control without Fog Badge", errGSSecondBadgeUnexpectedState)
	}
	if !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Morty completed without stable overworld control", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
