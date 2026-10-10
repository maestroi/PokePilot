package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsJasmineLighthouseX      uint8 = 8
	gsJasmineLighthouseY      uint8 = 8
	gsJasmineLighthouseStandX uint8 = 8
	gsJasmineLighthouseStandY uint8 = 9
	gsCianwoodPharmacistX     uint8 = 2
	gsCianwoodPharmacistY     uint8 = 3
	gsCianwoodPharmacyStandX  uint8 = 2
	gsCianwoodPharmacyStandY  uint8 = 4
)

// executeGSLighthouseSickness advances the first Olivine story gate.
// Entering Olivine triggers the retail rival departure scene; climbing the
// Lighthouse can start trainer battles. The shared native route owns both,
// while the story transaction owns Jasmine's conversation at 6F. The ROM's
// EVENT_JASMINE_EXPLAINED_AMPHYS_SICKNESS is the only completion evidence.
func executeGSLighthouseSickness(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Lighthouse: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressLighthouseSicknessExplained) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "olivine:lighthouse-sickness")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressHM03SurfAcquired) {
		return fmt.Errorf("%w: Lighthouse requires HM03 Surf frontier", errGSSecondBadgeUnexpectedState)
	}
	// Establish a local healing anchor before the multi-floor trainer climb.
	center, err := gsOpeningMapID("OLIVINE_POKECENTER_1F")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile,
		skill.ExactNativeDestination(center, gsPokecenterStandX, gsPokecenterStandY)); err != nil {
		return fmt.Errorf("gen2 Lighthouse: reach Olivine Center: %w", err)
	}
	if !profile.DecodeCenter(m).Recovered {
		if err := skill.Face(m, gsPokecenterCounterX, gsPokecenterCounterY); err != nil {
			return fmt.Errorf("gen2 Lighthouse: face Olivine nurse: %w", err)
		}
		if err := skill.Heal(m); err != nil {
			return fmt.Errorf("gen2 Lighthouse: heal party: %w", err)
		}
	}
	return gsTalkToLighthouseJasmine(m, romData, profile,
		gsprofile.ProgressLighthouseSicknessExplained, "olivine:lighthouse-sickness")
}

// executeGSSecretPotion continues through Route 40/41's water traversal to
// Cianwood Pharmacy. The medicine is not granted until the Lighthouse event
// is set, so this executor fails before travelling if that gate is missing.
// EVENT_GOT_SECRETPOTION_FROM_PHARMACY is durable even after the item is
// consumed by Jasmine in the subsequent objective.
func executeGSSecretPotion(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 SecretPotion: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSecretPotionAcquired) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "cianwood:secretpotion")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressLighthouseSicknessExplained) {
		return fmt.Errorf("%w: Cianwood pharmacist needs Jasmine's Lighthouse request", errGSSecondBadgeUnexpectedState)
	}
	pharmacy, err := gsOpeningMapID("CIANWOOD_PHARMACY")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeFaceFrom(m, romData, profile,
		skill.ExactNativeDestination(pharmacy, gsCianwoodPharmacyStandX, gsCianwoodPharmacyStandY),
		gsCianwoodPharmacistX, gsCianwoodPharmacistY, "cianwood:secretpotion"); err != nil {
		return fmt.Errorf("gen2 SecretPotion: reach pharmacist: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, "cianwood:secretpotion"); err != nil {
		return fmt.Errorf("gen2 SecretPotion: pickup script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSecretPotionAcquired) ||
		!profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: pharmacist returned without durable SecretPotion acquisition", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

// executeGSJasmineCured returns to Olivine and gives the SecretPotion to
// Ampharos. The script's yes/no prompt is owned by this transaction (YES is
// the cartridge default); Jasmine's departure event—not the earlier pharmacy
// event or a successful interaction—is the positive postcondition.
func executeGSJasmineCured(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Jasmine cure: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressJasmineReturnedToGym) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "olivine:jasmine-cured")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSecretPotionAcquired) {
		return fmt.Errorf("%w: Jasmine needs SecretPotion from Cianwood", errGSSecondBadgeUnexpectedState)
	}
	return gsTalkToLighthouseJasmine(m, romData, profile,
		gsprofile.ProgressJasmineReturnedToGym, "olivine:jasmine-cured")
}

func gsTalkToLighthouseJasmine(
	m *emu.Emu, romData []byte, profile *gsprofile.Profile,
	goal game.ProgressID, encounter string,
) error {
	lighthouse, err := gsOpeningMapID("OLIVINE_LIGHTHOUSE_6F")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeFaceFrom(m, romData, profile,
		skill.ExactNativeDestination(lighthouse, gsJasmineLighthouseStandX, gsJasmineLighthouseStandY),
		gsJasmineLighthouseX, gsJasmineLighthouseY, encounter); err != nil {
		return fmt.Errorf("gen2 Lighthouse: reach Jasmine: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
		return fmt.Errorf("gen2 Lighthouse: Jasmine script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, goal) {
		return fmt.Errorf("%w: Lighthouse script finished without %s event", errGSSecondBadgeUnexpectedState, goal)
	}
	if !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Lighthouse script did not restore controllable overworld", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
