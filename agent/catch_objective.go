package agent

import (
	"fmt"
	"slices"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func catchObjectiveOwnsTravel(o Objective) bool {
	switch o.Intent {
	case dexSafariIntent, dexTradeIntent, dexFossilIntent, dexGameCornerIntent, dexStaticIntent, dexGiftIntent,
		dexVirtualTradebackIntent, dexVirtualVersionIntent, dexVirtualPokedexIntent:
		return true
	default:
		return false
	}
}

// plainWildCatchIntent reports whether the catch objective hunts ordinary tall
// grass via skill.Catch, as opposed to a specialized executor that approaches
// its habitat differently (fishing and water within the travel block; the
// scripted sources are already excluded by catchObjectiveOwnsTravel). Only the
// plain grass hunt needs the player inside the encounter-cell component.
func plainWildCatchIntent(intent string) bool {
	switch intent {
	case dexFishingIntent, dexWaterIntent:
		return false
	default:
		return true
	}
}

func virtualTradeIntent(intent string) bool {
	switch intent {
	case dexVirtualTradebackIntent, dexVirtualVersionIntent, dexVirtualPokedexIntent:
		return true
	default:
		return false
	}
}

// catchObjectiveNeedsPartySlot separates acquisitions whose scripts require a
// free party slot from actual captures. In Generation I a wild/static/Safari
// catch made with a full six-Pokemon party is sent directly to the active PC
// box, so depositing a party member first is both unnecessary and dangerous:
// it can strand Cut/Surf/Strength ownership and turn a valid catch into
// field_roster_no_recovery. Gifts, fossil revivals and Game Corner prizes are
// direct party additions and still need the explicit slot preflight. Trades
// replace an existing party member and need neither path.
func catchObjectiveNeedsPartySlot(o Objective) bool {
	switch o.Intent {
	case dexGiftIntent, dexFossilIntent, dexGameCornerIntent:
		return true
	default:
		return false
	}
}

// redFishingRodID resolves the three fishing-only key items without widening
// the generic planner/executor item whitelist. The rods are intentionally
// observation/progression vocabulary, but dex fishing owns their use and may
// therefore translate them at this narrow execution boundary.
func redFishingRodID(id ItemID) (uint8, bool) {
	switch strings.ToLower(strings.TrimSpace(string(id))) {
	case "old rod", "good rod", "super rod":
		spec, ok := ItemEconomy(string(id))
		if !ok {
			return 0, false
		}
		return spec.ID, true
	default:
		return 0, false
	}
}

// safariCatchOpportunistic hunts the objective's species while also accepting
// every other unowned Dex target the same Safari habitat yields. A paid
// session has finite balls and steps, so fleeing a missing species only to pay
// for another session for it later wastes both. A bonus catch never satisfies
// the objective: the hunt continues on the remaining set until the target is
// caught or SafariCatch reports its own bounded exhaustion.
func safariCatchOpportunistic(m *emu.Emu, romData []byte, mapID uint8, o Objective, target uint8) (skill.CatchResult, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	owned, seen := ProjectPokedex(romData, state.DecodePokedex(&mem))
	catalog, err := BuildDexCatalog(romData, owned, seen)
	if err != nil {
		return skill.CatchResult{}, fmt.Errorf("Safari bonus targets: %w", err)
	}
	want := append([]uint8{target}, dexHabitatBonusSpecies(catalog, o.Place, o.Species)...)

	var total skill.CatchResult
	for {
		caught, err := skill.SafariCatch(m, romData, mapID, want, skill.StatAwareMove(romData), 10)
		total.Outcome, total.Species = caught.Outcome, caught.Species
		total.BallsThrown += caught.BallsThrown
		total.Encounters += caught.Encounters
		if err != nil || caught.Outcome != skill.OutcomeCaught || caught.Species == target {
			return total, err
		}
		want = slices.DeleteFunc(want, func(id uint8) bool { return id == caught.Species })
	}
}

// dexHabitatBonusSpecies lists the unowned Dex targets, other than target,
// that the catalog sources from wild grass at place.
func dexHabitatBonusSpecies(catalog DexCatalog, place PlaceID, target SpeciesID) []uint8 {
	var out []uint8
	for _, e := range catalog.Targets {
		if e.Species == target {
			continue
		}
		for _, src := range e.Sources {
			if src.Kind != AcquireWildGrass || src.Place != place {
				continue
			}
			if id, ok := redSpeciesID(e.Species); ok {
				out = append(out, id)
			}
			break
		}
	}
	return out
}

// safariCatchIntentForPlace returns the adapter intent a catch at place must
// run under when the place itself determines the path. A Safari Zone map is
// reachable only through a paid session, so a catch there is a Safari catch
// even when the offer omitted the intent: failure contracts written before
// the intent field joined FailureObjective reconstruct without it, and
// generic travel cannot name a Safari place.
func safariCatchIntentForPlace(place PlaceID) string {
	if mapID, ok := dexPlaceMapID(place); ok && safariRequirement(mapID) {
		return dexSafariIntent
	}
	return ""
}

func executeCatchObjective(m *emu.Emu, romData []byte, o Objective, result ObjectiveResult) (ObjectiveResult, error) {
	species, ok := redSpeciesID(o.Species)
	if !ok {
		return result, fmt.Errorf("agent: %s: unknown Red species %q", o, o.Species)
	}

	// The place can determine the adapter path when the offer did not name it.
	if o.Intent == "" {
		o.Intent = safariCatchIntentForPlace(o.Place)
	}

	// Acquisition storage follows the game mechanic instead of forcing every
	// Dex source through a party deposit. Real captures can overflow a full
	// party into Bill's active box; only scripted direct-party additions require
	// us to make a slot first. NPC and virtual trades replace one party member
	// in place and need neither path.
	if o.Intent != dexTradeIntent && !virtualTradeIntent(o.Intent) {
		var err error
		if catchObjectiveNeedsPartySlot(o) {
			err = skill.EnsurePartySlotForCollection(m, romData, skill.StatAwareMove(romData), species)
		} else {
			err = skill.EnsureCaptureStorage(m, romData, skill.StatAwareMove(romData))
		}
		if err != nil {
			result.Outcome = OutcomeBlocked
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
	}

	// Scripted acquisition sources own the route needed to reach the exact
	// interaction. Generic Place traversal is only appropriate for ordinary
	// wild/fishing/water habitats. In particular, every Dex gift executor owns
	// its travel: Eevee reaches the Celadon Mansion room, Lapras reaches its
	// Silph floor, and the Fighting Dojo gifts reach their prize tiles. Letting
	// only Lapras own travel made Eevee/Hitmon objectives try a bogus generic
	// "catch habitat" route before their scripted executor ever ran.
	ownsTravel := catchObjectiveOwnsTravel(o)
	if o.Place != "" && !ownsTravel {
		dest, ok := skill.Place(string(o.Place))
		if !ok {
			return result, fmt.Errorf("agent: %s: unknown catch habitat %q", o, o.Place)
		}
		// A wild-grass catch needs the encounter component, not map arrival:
		// a broad habitat name resolves to a map-arrival goal, which is a no-op
		// when the player is already on the map but in a component with no
		// grass (Route 10's south seam reaches its grass only via the Rock
		// Tunnel). Refine the goal to the habitat's canonical tile when that
		// tile is inside the grass component, so Travel lands the player where
		// Catch can actually hunt. Scripted executors (fishing, water, ...) own
		// their approach and keep the map goal.
		if plainWildCatchIntent(o.Intent) {
			dest = skill.CatchHabitatDestination(romData, dest)
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
		attachTravelResult(&result, travel)
		if err != nil {
			return result, fmt.Errorf("agent: %s: travel to catch habitat: %w", o, err)
		}
	}

	if virtualTradeIntent(o.Intent) {
		return executeDexVirtualTrade(m, romData, o, result)
	}

	var caught skill.CatchResult
	var err error
	switch o.Intent {
	case dexFishingIntent:
		rod, ok := redFishingRodID(o.Item)
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
		caught, err = safariCatchOpportunistic(m, romData, mapID, o, species)
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
	case dexGameCornerIntent:
		if o.Species != "porygon" {
			return result, fmt.Errorf("agent: %s: Game Corner executor only owns Porygon", o)
		}
		caught, err = skill.ReceivePorygonPrize(m, romData, skill.StatAwareMove(romData))
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
	if caught.BallsThrown > 0 && (caught.Outcome == skill.OutcomeFled || caught.Outcome == skill.OutcomeOutOfBalls) {
		// Balls were thrown at the wanted target and none held: the catch
		// roll lost, which is ordinary stochastic gameplay, not a defect.
		return result, fmt.Errorf("agent: %s: %w: no %s acquired (outcome %s, balls=%d, encounters=%d)",
			o, skill.ErrCatchMissed, strings.ToUpper(string(o.Species)), catchOutcomeName(caught.Outcome), caught.BallsThrown, caught.Encounters)
	}
	return result, fmt.Errorf("agent: %s: no %s acquired (outcome %s, balls=%d, encounters=%d)",
		o, strings.ToUpper(string(o.Species)), catchOutcomeName(caught.Outcome), caught.BallsThrown, caught.Encounters)
}
