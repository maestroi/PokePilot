package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

type gsKimonoTrainer struct {
	name           string
	x, y           uint8
	standX, standY uint8
}

var gsDanceTheaterKimonoGirls = []gsKimonoTrainer{
	{name: "naoko", x: 0, y: 2, standX: 0, standY: 3},
	{name: "sayo", x: 2, y: 1, standX: 2, standY: 2},
	{name: "zuki", x: 6, y: 2, standX: 6, standY: 3},
	{name: "kuni", x: 9, y: 1, standX: 9, standY: 2},
	{name: "miki", x: 11, y: 2, standX: 11, standY: 3},
}

const (
	gsDanceTheaterSurfGuyX      uint8 = 7
	gsDanceTheaterSurfGuyY      uint8 = 10
	gsDanceTheaterSurfGuyStandX uint8 = 7
	gsDanceTheaterSurfGuyStandY uint8 = 11
)

// executeGSHM03Surf advances the post-Morty frontier through the Ecruteak
// Dance Theater. Every Kimono Girl is deliberately interacted with: on a
// resumed save an already-defeated trainer only drains her after-battle text,
// while an undefeated trainer starts a required battle through the shared Gen2
// battle controller. The gentleman's EVENT_GOT_HM03_SURF bit is the durable
// completion boundary, so a retry never has to infer progress from inventory
// menus or dialogue timing.
func executeGSHM03Surf(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 HM03 Surf: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressHM03SurfAcquired) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "ecruteak:hm03-surf")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressFogBadgeEarned) {
		return fmt.Errorf("%w: HM03 Surf requires the Fog Badge", errGSSecondBadgeUnexpectedState)
	}
	if err := gsRecoverAtEcruteakCenter(m, romData, profile); err != nil {
		return fmt.Errorf("gen2 HM03 Surf: recover party: %w", err)
	}

	theater, err := gsOpeningMapID("DANCE_THEATER")
	if err != nil {
		return err
	}
	for _, girl := range gsDanceTheaterKimonoGirls {
		encounter := "dance-theater:" + girl.name
		if err := gsSecondBadgeFaceFrom(
			m,
			romData,
			profile,
			skill.ExactNativeDestination(theater, girl.standX, girl.standY),
			girl.x,
			girl.y,
			encounter,
		); err != nil {
			return fmt.Errorf("gen2 HM03 Surf: reach %s: %w", girl.name, err)
		}
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 HM03 Surf: %s battle/script: %w", girl.name, err)
		}

		// The five fights are a gauntlet but the retail game allows leaving
		// between them. Reuse the normal Center transaction after a fight so
		// unattended progress does not depend on carrying damage across all
		// five; already-defeated trainers leave the recovered state unchanged.
		if !profile.DecodeCenter(m).Recovered {
			if err := gsRecoverAtEcruteakCenter(m, romData, profile); err != nil {
				return fmt.Errorf("gen2 HM03 Surf: recover after %s: %w", girl.name, err)
			}
		}
	}

	if err := gsSecondBadgeFaceFrom(
		m,
		romData,
		profile,
		skill.ExactNativeDestination(theater, gsDanceTheaterSurfGuyStandX, gsDanceTheaterSurfGuyStandY),
		gsDanceTheaterSurfGuyX,
		gsDanceTheaterSurfGuyY,
		"ecruteak:hm03-surf",
	); err != nil {
		return fmt.Errorf("gen2 HM03 Surf: reach Surf gentleman: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, "ecruteak:hm03-surf"); err != nil {
		return fmt.Errorf("gen2 HM03 Surf: reward script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressHM03SurfAcquired) {
		return fmt.Errorf("%w: Dance Theater returned control without HM03 Surf", errGSSecondBadgeUnexpectedState)
	}
	if !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: HM03 Surf completed without stable overworld control", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
