package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill"
)

func executeCatchObjective(m *emu.Emu, romData []byte, o Objective, result ObjectiveResult) (ObjectiveResult, error) {
	species, ok := redSpeciesID(o.Species)
	if !ok {
		return result, fmt.Errorf("agent: %s: unknown Red species %q", o, o.Species)
	}
	if err := skill.EnsurePartySlotForCollection(m, romData, skill.StatAwareMove(romData), species); err != nil {
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: %w", o, err)
	}

	// Safari maps are deliberately not ordinary planner Place destinations:
	// their paid finite session must own gate entry, routing, Safari Balls and
	// exit/re-entry. Every other catch source keeps the normal travel wrapper.
	if o.Place != "" && o.Intent != dexSafariIntent {
		dest, ok := skill.Place(string(o.Place))
		if !ok {
			return result, fmt.Errorf("agent: %s: unknown catch habitat %q", o, o.Place)
		}
		var (
			travel skill.TravelResult
			err    error
		)
		if o.Flee {
			travel, err = skill.TravelFlee(m, romData, dest, skill.StatAwareMove(romData), 40)
		} else {
			travel, err = skill.Travel(m, romData, dest, skill.StatAwareMove(romData), 40)
		}
		result.Travel = &travel
		if err != nil {
			return result, fmt.Errorf("agent: %s: travel to catch habitat: %w", o, err)
		}
	}

	var caught skill.CatchResult
	var err error
	switch o.Intent {
	case dexFishingIntent:
		rod, ok := redItemID(o.Item)
		if !ok {
			return result, fmt.Errorf("agent: %s: unknown fishing rod %q", o, o.Item)
		}
		caught, err = skill.Fish(m, romData, rod, []uint8{species}, skill.StatAwareMove(romData), 5)
	case dexWaterIntent:
		caught, err = skill.CatchWater(m, romData, []uint8{species}, skill.StatAwareMove(romData), 5)
	case dexSafariIntent:
		mapID, ok := dexPlaceMapID(o.Place)
		if !ok || !safariRequirement(mapID) {
			return result, fmt.Errorf("agent: %s: unknown Safari habitat %q", o, o.Place)
		}
		caught, err = skill.SafariCatch(m, romData, mapID, []uint8{species}, skill.StatAwareMove(romData), 10)
	default:
		caught, err = skill.Catch(m, romData, []uint8{species}, skill.StatAwareMove(romData), 5)
	}
	if err != nil {
		return result, fmt.Errorf("agent: %s: %w", o, err)
	}
	if caught.Outcome == skill.OutcomeCaught {
		return result, nil
	}
	result.Outcome = OutcomeBlocked
	return result, fmt.Errorf("agent: %s: no %s caught (outcome %s, balls=%d, encounters=%d)",
		o, strings.ToUpper(string(o.Species)), catchOutcomeName(caught.Outcome), caught.BallsThrown, caught.Encounters)
}
