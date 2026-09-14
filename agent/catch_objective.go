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
	// Ordinary catches/gifts/fossils/statics add a party member and therefore
	// need collection storage preflight. NPC trades replace one party member
	// with another, so a deposit there is unnecessary and could remove the exact
	// give-species the trade planner selected.
	if o.Intent != dexTradeIntent {
		if err := skill.EnsurePartySlotForCollection(m, romData, skill.StatAwareMove(romData), species); err != nil {
			result.Outcome = OutcomeBlocked
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
	}

	// Safari, NPC trades, fossils, one-time statics and Lapras' Silph route own
	// semantics that an ordinary Place traversal cannot safely reproduce.
	ownsTravel := o.Intent == dexSafariIntent || o.Intent == dexTradeIntent || o.Intent == dexFossilIntent || o.Intent == dexStaticIntent || (o.Intent == dexGiftIntent && o.Species == "lapras")
	if o.Place != "" && !ownsTravel {
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
	case dexGiftIntent:
		switch o.Species {
		case "eevee":
			caught, err = skill.ReceiveEeveeGift(m, romData, skill.StatAwareMove(romData))
		case "lapras":
			caught, err = skill.ReceiveLaprasGift(m, romData, skill.StatAwareMove(romData))
		case "hitmonlee", "hitmonchan":
			caught, err = skill.ReceiveFightingDojoGift(m, romData, skill.StatAwareMove(romData), species)
		default:
			return result, fmt.Errorf("agent: %s: no scripted gift executor for %q", o, o.Species)
		}
	case dexTradeIntent:
		caught, err = skill.InGameTrade(m, romData, species, skill.StatAwareMove(romData))
	case dexFossilIntent:
		caught, err = skill.ReviveFossil(m, romData, species, skill.StatAwareMove(romData))
	case dexStaticIntent:
		caught, err = skill.CaptureStatic(m, romData, species, skill.StatAwareMove(romData))
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
	return result, fmt.Errorf("agent: %s: no %s acquired (outcome %s, balls=%d, encounters=%d)",
		o, strings.ToUpper(string(o.Species)), catchOutcomeName(caught.Outcome), caught.BallsThrown, caught.Encounters)
}
