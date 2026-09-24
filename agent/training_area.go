package agent

import (
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/skill"
)

// TrainingAreaKnowledge is learned run evidence about one visited wild habitat.
// It deliberately stores only semantic geography and observed level bands; the
// planner may use it to choose where to train without receiving undiscovered
// encounter tables or game-specific route wisdom.
type TrainingAreaKnowledge struct {
	Location LocationID `json:"location"`
	Place    PlaceID    `json:"place"`
	MinLevel uint8      `json:"min_level"`
	MaxLevel uint8      `json:"max_level"`
}

// TrainingAreaAssessment is recomputed from current party/resources and the
// active adapter's route/encounter data. Knowledge stores only visited habitats;
// these dynamic costs are never persisted as world truth.
type TrainingAreaAssessment struct {
	Place            PlaceID          `json:"place"`
	Location         LocationID       `json:"location"`
	MinLevel         uint8            `json:"min_level,omitempty"`
	MaxLevel         uint8            `json:"max_level,omitempty"`
	Selected         bool             `json:"selected,omitempty"`
	Routable         bool             `json:"routable"`
	RecoveryKnown    bool             `json:"recovery_known,omitempty"`
	TravelCost       int              `json:"travel_cost,omitempty"`
	RecoveryCost     int              `json:"recovery_cost,omitempty"`
	TotalCost        int              `json:"total_cost,omitempty"`
	FastTravel       bool             `json:"fast_travel,omitempty"`
	FastTravelMethod string           `json:"fast_travel_method,omitempty"`
	Estimate         TrainingEstimate `json:"estimate"`
	Reason           string           `json:"reason,omitempty"`
}

// rememberTrainingArea records a habitat only after the run has observed it.
// The travel-facing Place is resolved through the active game's objective
// catalog so later recovery can construct an ordinary GoTo objective rather
// than inventing a native-map destination.
func rememberTrainingArea(k *Knowledge, obs Observation) {
	if k == nil || !obs.HasGrass || len(obs.WildGrass) == 0 {
		return
	}
	location := observationLocation(obs, k)
	if location == "" {
		return
	}
	minLevel, maxLevel, ok := wildLevelBand(obs.WildGrass)
	if !ok {
		return
	}
	place := trainingPlaceForLocation(objectiveCatalogForObservation(obs), location)
	if place == "" {
		return
	}
	if k.TrainingAreas == nil {
		k.TrainingAreas = map[LocationID]TrainingAreaKnowledge{}
	}
	k.TrainingAreas[location] = TrainingAreaKnowledge{
		Location: location,
		Place:    place,
		MinLevel: minLevel,
		MaxLevel: maxLevel,
	}
}

func wildLevelBand(wild []WildSpecies) (uint8, uint8, bool) {
	var minLevel, maxLevel uint8
	for _, encounter := range wild {
		if encounter.MinLevel == 0 && encounter.MaxLevel == 0 {
			continue
		}
		lo, hi := encounter.MinLevel, encounter.MaxLevel
		if lo == 0 {
			lo = hi
		}
		if hi == 0 {
			hi = lo
		}
		if minLevel == 0 || lo < minLevel {
			minLevel = lo
		}
		if hi > maxLevel {
			maxLevel = hi
		}
	}
	return minLevel, maxLevel, maxLevel > 0
}

func trainingPlaceForLocation(catalog ObjectiveCatalog, location LocationID) PlaceID {
	var fallback PlaceID
	for _, destination := range catalog.Destinations {
		if destination.Location != location {
			continue
		}
		if destination.Kind == skill.DestinationMap {
			return destination.Place
		}
		if fallback == "" {
			fallback = destination.Place
		}
	}
	return fallback
}

type trainingAreaChoice struct {
	Area         TrainingAreaKnowledge
	Method       TrainingMethod
	Carry        uint8
	Hops         int
	Estimate     *TrainingEstimate
	TravelCost   int
	RecoveryCost int
	TotalCost    int
	FastTravel   bool
}

// bestKnownTrainingPlace ranks only already-observed, currently offered travel
// destinations. Higher encounter bands are preferred, while travel distance
// and switch-training overhead prevent "highest level anywhere" from winning
// blindly. Unsafe habitats with no viable carry are rejected.
func bestKnownTrainingPlace(obs Observation, known *Knowledge, placeNames []string, catalog ObjectiveCatalog) (trainingAreaChoice, bool) {
	if known == nil || len(known.TrainingAreas) == 0 || len(obs.Party) == 0 || len(placeNames) == 0 {
		return trainingAreaChoice{}, false
	}
	allowed := make(map[PlaceID]bool, len(placeNames))
	for _, name := range placeNames {
		allowed[PlaceID(name)] = true
	}

	current := observationLocation(obs, known)
	hops := mapHops(known.Adjacency, current)
	_, currentMax, _ := wildLevelBand(obs.WildGrass)

	if len(obs.TrainingAreaChoices) > 0 {
		for _, assessment := range obs.TrainingAreaChoices {
			if !assessment.Selected || !assessment.Routable || !allowed[assessment.Place] {
				continue
			}
			area, ok := known.TrainingAreas[assessment.Location]
			if !ok {
				continue
			}
			estimate := assessment.Estimate
			distance := hops[assessment.Location]
			return trainingAreaChoice{
				Area: area, Method: estimate.Method, Carry: estimate.CarryLevel, Hops: distance,
				Estimate: &estimate, TravelCost: assessment.TravelCost, RecoveryCost: assessment.RecoveryCost,
				TotalCost: assessment.TotalCost, FastTravel: assessment.FastTravel,
			}, true
		}
	}

	choices := make([]trainingAreaChoice, 0, len(known.TrainingAreas))
	for _, area := range known.TrainingAreas {
		if area.Location == "" || area.Location == current || area.Place == "" || !allowed[area.Place] {
			continue
		}
		if trainingUnviableHere(obs) && currentMax > 0 && area.MaxLevel <= currentMax {
			// If the current habitat already proved too slow, moving to an equal
			// or weaker band cannot solve the measured problem.
			continue
		}
		if int(area.MaxLevel)+8 < int(obs.Party[0].Level) {
			// Safety alone is not usefulness: a L50 lead can safely stomp L15
			// encounters forever. Reject bands that are materially below the
			// trainee before spending travel time on them.
			continue
		}
		method, carry, safe := trainingMethodForObservedParty(obs.Party, area.MaxLevel)
		if !safe {
			continue
		}
		distance, reachable := hops[area.Location]
		if !reachable && len(known.Adjacency) > 0 {
			continue
		}
		choices = append(choices, trainingAreaChoice{Area: area, Method: method, Carry: carry, Hops: distance})
	}
	if len(choices) == 0 {
		return trainingAreaChoice{}, false
	}
	sort.SliceStable(choices, func(i, j int) bool {
		left, right := trainingAreaUtility(choices[i]), trainingAreaUtility(choices[j])
		if left != right {
			return left > right
		}
		if choices[i].Hops != choices[j].Hops {
			return choices[i].Hops < choices[j].Hops
		}
		return choices[i].Area.Place < choices[j].Area.Place
	})
	return choices[0], true
}

func trainingAreaUtility(choice trainingAreaChoice) int {
	// Level is an intentionally conservative XP proxy here. Exact local XP
	// estimates still come from TrainingEstimate after arrival, where the ROM
	// encounter table and species base yields are available.
	score := int(choice.Area.MaxLevel)*12 + int(choice.Area.MinLevel)*3 - choice.Hops*5
	if choice.Method == TrainingSwitch {
		score -= 10
	}
	return score
}

func trainingMethodForObservedParty(party []PartyMon, wildMax uint8) (TrainingMethod, uint8, bool) {
	if len(party) == 0 || wildMax == 0 {
		return TrainingDirect, 0, false
	}
	target := party[0]
	if target.HP == 0 {
		return TrainingDirect, 0, false
	}
	if int(wildMax) <= int(target.Level)+directTrainingWildGap {
		return TrainingDirect, 0, true
	}

	minCarry := minimumTrainingCarryLevel(target.Level, wildMax)
	var carry uint8
	for i := 1; i < len(party); i++ {
		mon := party[i]
		if mon.HP == 0 || mon.Level < minCarry || mon.Status == "frozen" {
			continue
		}
		if mon.MaxHP > 0 && mon.HP*2 < mon.MaxHP {
			continue
		}
		if mon.Level > carry {
			carry = mon.Level
		}
	}
	if carry == 0 {
		return TrainingDirect, 0, false
	}
	return TrainingSwitch, carry, true
}

func trainingAreaJourneyNote(choice trainingAreaChoice) string {
	method := "direct training"
	if choice.Method == TrainingSwitch {
		method = fmt.Sprintf("switch training via L%d carry", choice.Carry)
	}
	if choice.Estimate != nil {
		fast := ""
		if choice.FastTravel {
			fast = "; legal fast travel lowers route cost"
		}
		return fmt.Sprintf("(best known training area: %s; travel cost %d; recovery cost %d; total %d%s)",
			choice.Estimate.Diagnostic(), choice.TravelCost, choice.RecoveryCost, choice.TotalCost, fast)
	}
	return fmt.Sprintf("(best known training area: observed wilds L%d-L%d; %s; %d map hop(s))",
		choice.Area.MinLevel, choice.Area.MaxLevel, method, choice.Hops)
}

func bestKnownTrainingJourney(obs Observation, known *Knowledge, offered []Objective) (Objective, trainingAreaChoice, bool) {
	placeNames := make([]string, 0, len(offered))
	byPlace := map[PlaceID]Objective{}
	for _, objective := range offered {
		if objective.Kind != KindGoTo || objective.Place == "" {
			continue
		}
		place := objective.Place
		placeNames = append(placeNames, string(place))
		current, exists := byPlace[place]
		// Prefer the fleeing journey so repositioning for preparation does not
		// burn HP/PP on incidental encounters before the training session.
		if !exists || (!current.Flee && objective.Flee) {
			byPlace[place] = objective
		}
	}
	choice, ok := bestKnownTrainingPlace(obs, known, placeNames, objectiveCatalogForObservation(obs))
	if !ok {
		return Objective{}, trainingAreaChoice{}, false
	}
	objective, ok := byPlace[choice.Area.Place]
	if !ok {
		return Objective{}, trainingAreaChoice{}, false
	}
	objective = appendObjectiveNote(objective, trainingAreaJourneyNote(choice))
	return objective, choice, true
}
