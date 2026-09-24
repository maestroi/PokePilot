package agent

import (
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

const (
	trainingAreaEncounterCost       = 100
	trainingAreaSwitchPenalty       = 50
	trainingAreaNoRecoveryPenalty   = 300
	trainingAreaRecoveryCostDivisor = 2
)

// redSafariTrainingMap identifies the four outdoor Safari habitats. Their wild
// encounters use the Safari Game capture rules and never run ordinary battles,
// so they cannot be used as XP-training areas even though the ROM has grass
// encounter tables for them.
func redSafariTrainingMap(mapID uint8) bool {
	return mapID >= safariZoneEastMap && mapID <= safariZoneCenterMap
}

// filterRedSafariTrainingObjectives keeps the generic training provider from
// offering ordinary XP grinding during an active Safari session. The dedicated
// Fuchsia/Safari verbs own the paid session, finite step budget and exit path.
func filterRedSafariTrainingObjectives(obs Observation, out []Objective) []Objective {
	if !redSafariTrainingMap(obs.Map) {
		return out
	}
	kept := out[:0]
	for _, objective := range out {
		if objective.Kind != KindTrain {
			kept = append(kept, objective)
		}
	}
	return kept
}

func trainingAreaTargetLevel(obs Observation, known *Knowledge) int {
	if len(obs.Party) == 0 {
		return 0
	}
	current := int(obs.Party[0].Level)
	if current >= 100 {
		return 100
	}
	gain := trainStep
	if prep := combatPreparationFor(known, obs); prep.Active && prep.Target > prep.Current {
		needed := (prep.Target - prep.Current + 3) / 4
		if needed > gain {
			gain = needed
		}
	}
	target := current + gain
	if target > 100 {
		target = 100
	}
	return target
}

func trainingAreaRecoveryCost(location LocationID, obs Observation, known *Knowledge, catalog ObjectiveCatalog) (int, bool) {
	if known == nil {
		return 0, false
	}
	current := observationLocation(obs, known)
	best := 0
	found := false
	hops := mapHops(known.Adjacency, location)
	for _, destination := range catalog.Destinations {
		if !destination.Center || destination.Location == "" {
			continue
		}
		knownCenter := destination.Place == obs.RecoveryCheckpoint ||
			known.Visited[destination.Location] ||
			(catalog.CurrentCenter && destination.Location == current)
		if !knownCenter {
			continue
		}
		cost := 0
		if destination.Location != location {
			distance, ok := hops[destination.Location]
			if !ok {
				continue
			}
			cost = distance * recoveryMapHopCost
		}
		if !found || cost < best {
			best, found = cost, true
		}
	}
	return best, found
}

func finalizeTrainingAreaCost(assessment *TrainingAreaAssessment) {
	if assessment == nil || !assessment.Routable {
		return
	}
	if assessment.Estimate.Viability == TrainingOutsideBudget || assessment.Estimate.Viability == TrainingSatisfied {
		return
	}
	total := assessment.TravelCost + assessment.Estimate.EstimatedEncounters*trainingAreaEncounterCost
	if assessment.RecoveryKnown {
		total += assessment.RecoveryCost / trainingAreaRecoveryCostDivisor
	} else {
		total += trainingAreaNoRecoveryPenalty
	}
	if assessment.Estimate.Method == TrainingSwitch {
		total += trainingAreaSwitchPenalty
	}
	assessment.TotalCost = total
}

func trainingAreaUsable(assessment TrainingAreaAssessment) bool {
	return assessment.Routable &&
		assessment.Estimate.Viability != TrainingOutsideBudget &&
		assessment.Estimate.Viability != TrainingSatisfied &&
		assessment.Estimate.XPPerEncounter > 0
}

func rankTrainingAreaAssessments(in []TrainingAreaAssessment) []TrainingAreaAssessment {
	out := append([]TrainingAreaAssessment(nil), in...)
	for i := range out {
		out[i].Selected = false
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := trainingAreaUsable(out[i]), trainingAreaUsable(out[j])
		if left != right {
			return left
		}
		if left && out[i].TotalCost != out[j].TotalCost {
			return out[i].TotalCost < out[j].TotalCost
		}
		if left && out[i].Estimate.EstimatedEncounters != out[j].Estimate.EstimatedEncounters {
			return out[i].Estimate.EstimatedEncounters < out[j].Estimate.EstimatedEncounters
		}
		if out[i].TravelCost != out[j].TravelCost {
			return out[i].TravelCost < out[j].TravelCost
		}
		return out[i].Place < out[j].Place
	})
	for i := range out {
		if trainingAreaUsable(out[i]) {
			out[i].Selected = true
			break
		}
	}
	return out
}

// backfillVisitedTrainingAreas seeds TrainingAreas once from maps the run has
// already visited. Knowledge written before habitats were learned (resumed
// endless lineages) otherwise carries none, and combat preparation then has
// no area to route to while every Fly lands in a grassless town. A visited
// map is an observed habitat; its band is the same ROM encounter data
// rememberTrainingArea records on arrival. Reachability is not proven here:
// forgetUnreachedTrainingArea drops a seeded area whose journey lands without
// grass, and the one-shot flag keeps it from being seeded again.
func backfillVisitedTrainingAreas(romData []byte, obs Observation, known *Knowledge) {
	if known == nil || known.TrainingAreasBackfilled || len(romData) == 0 {
		return
	}
	known.TrainingAreasBackfilled = true
	catalog := objectiveCatalogForObservation(obs)
	for mapID, location := range known.nativeLocations {
		if !known.Visited[location] {
			continue
		}
		if _, ok := known.TrainingAreas[location]; ok {
			continue
		}
		if grass, err := skill.HasGrass(romData, mapID); err != nil || !grass {
			continue
		}
		wild, err := skill.WildGrass(romData, mapID)
		if err != nil {
			continue
		}
		band := make([]WildSpecies, 0, len(wild))
		for _, w := range wild {
			band = append(band, WildSpecies{MinLevel: w.MinLevel, MaxLevel: w.MaxLevel})
		}
		minLevel, maxLevel, ok := wildLevelBand(band)
		place := trainingPlaceForLocation(catalog, location)
		if !ok || place == "" {
			continue
		}
		if known.TrainingAreas == nil {
			known.TrainingAreas = map[LocationID]TrainingAreaKnowledge{}
		}
		known.TrainingAreas[location] = TrainingAreaKnowledge{
			Location: location, Place: place, MinLevel: minLevel, MaxLevel: maxLevel,
		}
	}
}

// redTrainingAreaAssessments re-prices every legitimately learned habitat from
// the current party state. Exact XP comes from the existing ROM encounter/base
// yield estimator; route cost comes from the same live Travel/Fly stack used by
// execution. Nothing is cached, so levels, carries, field moves and new world
// knowledge immediately change the next round's ranking.
func redTrainingAreaAssessments(m *emu.Emu, romData []byte, obs Observation, known *Knowledge) []TrainingAreaAssessment {
	backfillVisitedTrainingAreas(romData, obs, known)
	if m == nil || known == nil || len(known.TrainingAreas) == 0 || len(obs.Party) == 0 {
		return nil
	}
	targetLevel := trainingAreaTargetLevel(obs, known)
	if targetLevel <= int(obs.Party[0].Level) {
		return nil
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	catalog := objectiveCatalogForObservation(obs)
	current := observationLocation(obs, known)

	locations := make([]LocationID, 0, len(known.TrainingAreas))
	for location := range known.TrainingAreas {
		locations = append(locations, location)
	}
	sort.Slice(locations, func(i, j int) bool { return locations[i] < locations[j] })

	out := make([]TrainingAreaAssessment, 0, len(locations))
	for _, location := range locations {
		area := known.TrainingAreas[location]
		assessment := TrainingAreaAssessment{
			Place: area.Place, Location: area.Location, MinLevel: area.MinLevel, MaxLevel: area.MaxLevel,
		}
		destination, ok := skill.Place(area.Place)
		if !ok {
			assessment.Reason = "learned habitat has no adapter travel destination"
			out = append(out, assessment)
			continue
		}
		if redSafariTrainingMap(destination.Map) {
			assessment.Reason = "Safari Game habitats use capture-only encounters and cannot award ordinary training XP"
			out = append(out, assessment)
			continue
		}

		estimate, err := currentPartyTrainingEstimate(&mem, romData, destination.Map, 0, targetLevel, trainSessionBattleBudget)
		if err != nil {
			assessment.Reason = fmt.Sprintf("training estimate unavailable: %v", err)
			out = append(out, assessment)
			continue
		}
		assessment.Estimate = estimate

		if location == current {
			assessment.Routable = true
		} else if travel, ok := skill.EstimateTravelCost(m, romData, destination); ok {
			assessment.Routable = true
			assessment.TravelCost = travel.Cost
			assessment.FastTravel = travel.FastTravel
			assessment.FastTravelMethod = travel.Method
		} else {
			assessment.Reason = "no legal route with current capabilities"
			out = append(out, assessment)
			continue
		}

		assessment.RecoveryCost, assessment.RecoveryKnown = trainingAreaRecoveryCost(location, obs, known, catalog)
		finalizeTrainingAreaCost(&assessment)

		switch assessment.Estimate.Viability {
		case TrainingOutsideBudget:
			assessment.Reason = "exact XP/safety estimate is outside the bounded training session"
		case TrainingSatisfied:
			assessment.Reason = "training target is already satisfied"
		default:
			method := "direct"
			if assessment.Estimate.Method == TrainingSwitch {
				method = fmt.Sprintf("switch via L%d carry", assessment.Estimate.CarryLevel)
			}
			recovery := "no known recovery hub"
			if assessment.RecoveryKnown {
				recovery = fmt.Sprintf("recovery cost %d", assessment.RecoveryCost)
			}
			assessment.Reason = fmt.Sprintf(
				"%s; ~%d encounters at ~%d XP/encounter; travel cost %d; %s; total %d",
				method, assessment.Estimate.EstimatedEncounters, assessment.Estimate.XPPerEncounter,
				assessment.TravelCost, recovery, assessment.TotalCost,
			)
		}
		out = append(out, assessment)
	}
	return rankTrainingAreaAssessments(out)
}
