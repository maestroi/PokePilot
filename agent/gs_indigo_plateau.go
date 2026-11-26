package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// gsLeagueApproachMaps are the maps east of New Bark on the way to the
// plateau. A resume that is already on them continues on foot; anywhere
// else first heals in Blackthorn and, when the party can, flies to New Bark.
var gsLeagueApproachMaps = []string{
	"NEW_BARK_TOWN", "ROUTE_27", "TOHJO_FALLS", "ROUTE_26", "ROUTE_26_HEAL_HOUSE",
	"VICTORY_ROAD_GATE", "VICTORY_ROAD", "ROUTE_23", "INDIGO_PLATEAU_POKECENTER_1F",
}

// gsPastRoute26HealHouse are the approach maps north of the last heal stop.
var gsPastRoute26HealHouse = []string{
	"VICTORY_ROAD_GATE", "VICTORY_ROAD", "ROUTE_23", "INDIGO_PLATEAU_POKECENTER_1F",
}

// The Route 26 Heal House teacher stands at (2,3) facing down and runs
// special HealParty (pokegold/maps/Route26HealHouse.asm).
const (
	gsRoute26HealerX      uint8 = 2
	gsRoute26HealerY      uint8 = 3
	gsRoute26HealerStandX uint8 = 2
	gsRoute26HealerStandY uint8 = 4
)

func gsLeagueApproachMap(mapID uint16) bool {
	return gsMapIn(mapID, gsLeagueApproachMaps)
}

func gsMapIn(mapID uint16, names []string) bool {
	for _, name := range names {
		if id, err := gsOpeningMapID(name); err == nil && id == mapID {
			return true
		}
	}
	return false
}

// executeGSIndigoPlateau walks the retail League approach after the Rising
// Badge: Surf east from New Bark across Route 27 and Tohjo Falls, Route 26,
// the Victory Road Gate badge check and Victory Road with its rival battle.
// The gate scene and the rival are interruptions of that walk, fought by the
// owned driver; a lost rival battle is a typed combat outcome for recovery.
//
// Completion is only Route 23's own ENGINE_FLYPOINT_INDIGO_PLATEAU, which
// its MAPCALLBACK_NEWMAP sets on arrival. Entering the Elite Four is not
// part of this objective.
func executeGSIndigoPlateau(m *emu.Emu, romData []byte) error {
	const label = "league:indigo-plateau"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressIndigoPlateauReached,
		gsprofile.ProgressRisingBadgeEarned, label)
	if err != nil || done {
		return err
	}
	reached := func() bool {
		return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressIndigoPlateauReached)
	}
	if _, err := skill.EnsureFieldMove(m, skill.FieldSurf); err != nil {
		return fmt.Errorf("gen2 Indigo Plateau: prepare Surf: %w", err)
	}
	if !gsLeagueApproachMap(profile.DecodeOverworld(m).NativeMapID) {
		if err := gsCoastalHeal(m, romData, profile, "BLACKTHORN_POKECENTER_1F"); err != nil {
			return fmt.Errorf("gen2 Indigo Plateau: heal before the League approach: %w", err)
		}
		blackthorn, err := gsOpeningMapID("BLACKTHORN_CITY")
		if err != nil {
			return err
		}
		if err := gsSecondBadgeGoTo(m, romData, profile, skill.NativeMapDestination(blackthorn)); err != nil {
			return fmt.Errorf("gen2 Indigo Plateau: leave Blackthorn Pokecenter: %w", err)
		}
		newBark, err := gsOpeningMapID("NEW_BARK_TOWN")
		if err != nil {
			return err
		}
		// Fly only shortens the trip. Without a carrier the owned Johto
		// route walks back to New Bark, so it is not a prerequisite.
		if err := skill.UseNativeFly(m, newBark); err != nil && !errors.Is(err, skill.ErrFieldMovePrerequisite) {
			return fmt.Errorf("gen2 Indigo Plateau: fly to New Bark: %w", err)
		}
	}
	// The Route 27/26 trainers drain the party between Blackthorn and the
	// gate, and there is no Center before Indigo. Rest in the retail Route 26
	// Heal House unless already past it; a full party is no reason to skip,
	// since the walk there is what drains it.
	if !gsMapIn(profile.DecodeOverworld(m).NativeMapID, gsPastRoute26HealHouse) {
		if err := gsRoute26HealHouseRest(m, romData, profile, label); err != nil {
			return fmt.Errorf("gen2 Indigo Plateau: rest at Route 26 Heal House: %w", err)
		}
	}
	route23, err := gsOpeningMapID("ROUTE_23")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoToUntil(m, romData, profile, skill.NativeMapDestination(route23), reached); err != nil {
		return fmt.Errorf("gen2 Indigo Plateau: cross Victory Road: %w", err)
	}
	if !profile.DecodeOverworld(m).Controllable {
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 Indigo Plateau: settle arrival: %w", err)
		}
	}
	if !reached() {
		return fmt.Errorf("%w: reached Route 23 without the Indigo Plateau flypoint", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

// gsRoute26HealHouseRest talks to the heal-house teacher and accepts only a
// recovered party, never the end of the dialogue, as proof.
func gsRoute26HealHouseRest(m *emu.Emu, romData []byte, profile *gsprofile.Profile, label string) error {
	house, err := gsOpeningMapID("ROUTE_26_HEAL_HOUSE")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeFaceFrom(m, romData, profile,
		skill.ExactNativeDestination(house, gsRoute26HealerStandX, gsRoute26HealerStandY),
		gsRoute26HealerX, gsRoute26HealerY, label); err != nil {
		return err
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
		return err
	}
	if !profile.DecodeCenter(m).Recovered {
		return fmt.Errorf("%w: Route 26 Heal House returned without a recovered party", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
