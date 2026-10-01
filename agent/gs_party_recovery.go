package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// Johto Pokemon Center 1F maps share the retail nurse at (3,1) behind a
// counter on (3,2). The player talks from (3,3). Measured from pret/pokegold
// maps/AzaleaPokecenter1F.asm (object_event 3, 1, SPRITE_NURSE) plus the live
// Azalea 1F grid: y=2 x=0..5 is the counter, so (3,2) is not walkable.
const (
	gsPokecenterNurseX   uint8 = 3
	gsPokecenterNurseY   uint8 = 1
	gsPokecenterCounterX uint8 = 3
	gsPokecenterCounterY uint8 = 2
	gsPokecenterStandX   uint8 = 3
	gsPokecenterStandY   uint8 = 3
)

var gsOwnedPokecenterNames = []string{
	"AZALEA_POKECENTER_1F",
	"VIOLET_POKECENTER_1F",
	"CHERRYGROVE_POKECENTER_1F",
}

func gsOwnedPokecenterID(from uint16) (uint16, error) {
	fromInfo, fromOK := gsdata.Map(from)
	var sameGroup, any []uint16
	for _, name := range gsOwnedPokecenterNames {
		id, err := gsOpeningMapID(name)
		if err != nil {
			continue
		}
		any = append(any, id)
		info, ok := gsdata.Map(id)
		if fromOK && ok && info.Group == fromInfo.Group {
			sameGroup = append(sameGroup, id)
		}
	}
	if len(sameGroup) > 0 {
		return sameGroup[0], nil
	}
	if len(any) > 0 {
		return any[0], nil
	}
	return 0, fmt.Errorf("%w: no owned Pokemon Center is catalogued", errGSSecondBadgeUnexpectedState)
}

func gsEnsurePartyRecovered(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	if m == nil || profile == nil {
		return fmt.Errorf("%w: missing emulator/profile", errGSSecondBadgeUnexpectedState)
	}
	center := profile.DecodeCenter(m)
	if !center.PartyPresent || center.Recovered {
		return nil
	}
	world := profile.DecodeOverworld(m)
	pokecenter, err := gsOwnedPokecenterID(world.NativeMapID)
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(pokecenter, gsPokecenterStandX, gsPokecenterStandY)); err != nil {
		return fmt.Errorf("reach Pokemon Center: %w", err)
	}
	if err := skill.Face(m, gsPokecenterCounterX, gsPokecenterCounterY); err != nil {
		return fmt.Errorf("face nurse: %w", err)
	}
	if err := skill.Heal(m); err != nil {
		return err
	}
	if !profile.DecodeCenter(m).Recovered {
		return fmt.Errorf("%w: Pokemon Center returned control without a recovered party", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
