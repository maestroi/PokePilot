package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

// OpportunityCost is the explicit cost side of play-style scoring. The
// components describe facts the runtime can observe without inventing a route:
// exact same-map tile distance where available, a conservative cross-map
// travel estimate, battle risk, recent backtracking, route uncertainty,
// deferrability, and pressure from an explicit remaining-round budget.
//
// Raw is the sum of those components. Total applies the active play style's
// detour penalty. Keeping both makes the score inspectable without pretending
// the heuristic is a pathfinder or frame-perfect travel estimate.
type OpportunityCost struct {
	DistanceTiles int
	CrossMap      bool
	Travel        float64
	EncounterRisk float64
	Backtracking  float64
	Uncertainty   float64
	Deferrability float64
	GoalPressure  float64
	Raw           float64
	Total         float64
}

func opportunityCost(obs Observation, o Objective, profile PlayStyleProfile) OpportunityCost {
	c := OpportunityCost{}
	distance, crossMap, located := objectiveDistance(obs, o)
	c.DistanceTiles = distance
	c.CrossMap = crossMap

	switch {
	case located && !crossMap:
		// A human normally takes a two-tile-or-less sidestep for free. Past
		// that, charge gradually rather than introducing a magic cutoff.
		if distance > 2 {
			c.Travel = minFloat(0.55, float64(distance-2)/24.0*0.55)
		}
	case crossMap && strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map"):
		// The offerer already proved this is the current exploration frontier.
		// It is a map transition, but not the same thing as crossing Kanto.
		c.Travel = 0.12
	case crossMap && located:
		c.Travel = 0.42
	case crossMap:
		// Unknown semantic place. Keep it possible, but make the uncertainty
		// visible rather than treating an unmeasurable trip as free.
		c.Travel = 0.30
		c.Uncertainty += 0.16
	}

	c.EncounterRisk += opportunityEncounterRisk(obs, o, crossMap)
	c.Backtracking += opportunityBacktrack(obs, o)
	c.Uncertainty += opportunityRouteUncertainty(obs, o, crossMap)
	c.Deferrability = opportunityDeferrability(o)
	c.GoalPressure = opportunityGoalPressure(obs, o)

	c.Raw = c.Travel + c.EncounterRisk + c.Backtracking + c.Uncertainty + c.Deferrability + c.GoalPressure
	c.Total = c.Raw * opportunityPenalty(profile)
	return c
}

// objectiveDistance returns the exact lower-bound walking distance available
// from planner state. Same-map objects and semantic destinations have concrete
// coordinates. Cross-map routes deliberately do not claim a fake tile count:
// the route planner owns the actual path and opportunityCost uses a bounded
// cross-map travel estimate instead.
func objectiveDistance(obs Observation, o Objective) (distance int, crossMap bool, located bool) {
	switch o.Kind {
	case KindTalk, KindTrainer, KindPickup:
		// These interactions happen from an adjacent tile.
		return maxInt(0, manhattan(int(obs.X), int(obs.Y), int(o.X), int(o.Y))-1), false, true
	case KindGoTo, KindGym:
		return namedObjectiveDistance(obs, o.Place)
	case KindHeal:
		if o.Place == "" {
			return 0, false, true
		}
		return namedObjectiveDistance(obs, o.Place)
	default:
		// Catch, Train, Buy, UseItem, Starter and local Progress actions are
		// performed where the player already is.
		return 0, false, true
	}
}

func namedObjectiveDistance(obs Observation, place PlaceID) (distance int, crossMap bool, located bool) {
	if place == "" {
		return 0, false, true
	}
	d, ok := skill.Place(string(place))
	if !ok {
		return 0, true, false
	}
	if d.Map != obs.Map {
		return 0, true, true
	}
	return manhattan(int(obs.X), int(obs.Y), int(d.X), int(d.Y)), false, true
}

func opportunityEncounterRisk(obs Observation, o Objective, crossMap bool) float64 {
	risk := 0.0
	switch o.Kind {
	case KindCatch:
		risk = 0.12
	case KindTrain:
		risk = 0.16
	case KindTrainer:
		risk = 0.18
	case KindGym:
		risk = 0.22
	case KindGoTo, KindHeal:
		if crossMap || obs.HasGrass {
			if o.Flee {
				risk = 0.03
			} else {
				risk = 0.10
			}
		}
	}
	if risk > 0 && (partyHurt(obs) || leadOutOfPP(obs)) {
		risk *= 1.5
	}
	return risk
}

func opportunityBacktrack(obs Observation, o Objective) float64 {
	place, ok := opportunityTargetPlace(o)
	if !ok || place == "" || place == obs.Location {
		return 0
	}
	// Recovery is not a frivolous backtrack. The safety drive already decides
	// whether the heal is urgent; do not charge it again for returning home.
	if o.Kind == KindHeal && (partyHurt(obs) || leadOutOfPP(obs)) {
		return 0
	}

	needle := strings.ToLower(string(place))
	// History is capped and oldest-first. Revisiting a place from the last few
	// rounds is a useful signal for A<->B route oscillation, but a small one:
	// story progression legitimately revisits towns too.
	for i := len(obs.History) - 1; i >= 0 && i >= len(obs.History)-4; i-- {
		line := strings.ToLower(obs.History[i].Objective)
		if strings.HasPrefix(line, "go to "+needle) || strings.Contains(line, " at "+needle) {
			age := len(obs.History) - 1 - i
			return maxFloat(0.05, 0.14-float64(age)*0.03)
		}
	}
	return 0
}

func opportunityTargetPlace(o Objective) (PlaceID, bool) {
	switch o.Kind {
	case KindGoTo, KindGym:
		return o.Place, o.Place != ""
	case KindHeal:
		return o.Place, o.Place != ""
	default:
		return "", false
	}
}

func opportunityRouteUncertainty(obs Observation, o Objective, crossMap bool) float64 {
	place, ok := opportunityTargetPlace(o)
	if !ok || place == "" {
		return 0
	}
	for _, name := range obs.Unroutable {
		if strings.EqualFold(name, string(place)) {
			return 0.35
		}
	}
	for _, blockage := range obs.RouteBlockages {
		if blockage.Destination == place && len(blockage.Missing) > 0 {
			return 0.35
		}
	}
	if crossMap && !strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map") {
		return 0.07
	}
	return 0
}

func opportunityDeferrability(o Objective) float64 {
	// Optional local actions can usually be done later. Keep this deliberately
	// small: the whole point of Adventure is that a cheap nearby opportunity
	// should still beat postponing it indefinitely.
	switch o.Kind {
	case KindTalk, KindPickup:
		return 0.02
	case KindCatch:
		return 0.05
	case KindTrain, KindTrainer:
		return 0.04
	case KindBuy:
		return 0.02
	case KindGoTo:
		if strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map") {
			return 0.03
		}
	}
	return 0
}

func opportunityGoalPressure(obs Observation, o Objective) float64 {
	if obs.RoundsLeft <= 0 || obs.RoundsLeft > 12 || !optionalOpportunity(o) {
		return 0
	}
	// A fixed-budget experiment near its ceiling should become less willing
	// to spend rounds on optional detours. Unlimited normal runs have
	// RoundsLeft == 0 and therefore get no artificial urgency.
	return float64(13-obs.RoundsLeft) / 12.0 * 0.25
}

func optionalOpportunity(o Objective) bool {
	switch o.Kind {
	case KindTalk, KindPickup, KindCatch, KindTrain, KindTrainer, KindBuy:
		return true
	case KindGoTo:
		return strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map")
	default:
		return false
	}
}

func opportunityPenalty(profile PlayStyleProfile) float64 {
	switch profile.Name {
	case "adventure":
		return 0.70
	case "speedrun", "":
		return 1.00
	default:
		return 0.85
	}
}

// highImpactItem lets reward value, rather than a special-case exemption from
// cost, justify a larger detour. TMs/HMs are durable party/preparation assets;
// ordinary consumables keep the baseline item value.
func highImpactItem(item ItemID) bool {
	name := strings.ToLower(strings.TrimSpace(string(item)))
	return strings.HasPrefix(name, "tm") || strings.HasPrefix(name, "hm")
}

func opportunityCostSummary(c OpportunityCost) string {
	type component struct {
		name  string
		value float64
	}
	parts := []component{
		{"travel", c.Travel},
		{"risk", c.EncounterRisk},
		{"backtrack", c.Backtracking},
		{"uncertain", c.Uncertainty},
		{"later", c.Deferrability},
		{"deadline", c.GoalPressure},
	}
	first, second := component{}, component{}
	for _, p := range parts {
		if p.value > first.value {
			second = first
			first = p
		} else if p.value > second.value {
			second = p
		}
	}
	if first.value == 0 {
		return "cost 0"
	}
	if second.value == 0 {
		return fmt.Sprintf("cost %.2f %s", c.Total, first.name)
	}
	return fmt.Sprintf("cost %.2f %s+%s", c.Total, first.name, second.name)
}

func manhattan(x1, y1, x2, y2 int) int {
	return absInt(x1-x2) + absInt(y1-y2)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
