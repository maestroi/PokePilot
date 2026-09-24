package agent

import "sort"

const (
	recoveryMapHopCost       = 100
	recoveryActiveCheckpoint = 100
)

// RecoveryCheckpointAssessment is structured telemetry for one known healing
// candidate. Costs are route-policy units, not emulator frames.
type RecoveryCheckpointAssessment struct {
	Place            PlaceID    `json:"place"`
	Location         LocationID `json:"location,omitempty"`
	Selected         bool       `json:"selected,omitempty"`
	ActiveCheckpoint bool       `json:"active_checkpoint,omitempty"`
	Routable         bool       `json:"routable"`
	CurrentCost      int        `json:"current_cost,omitempty"`
	ReturnCost       int        `json:"return_cost,omitempty"`
	TotalCost        int        `json:"total_cost,omitempty"`
	FastTravel       bool       `json:"fast_travel,omitempty"`
	FastTravelMethod string     `json:"fast_travel_method,omitempty"`
	Reason           string     `json:"reason,omitempty"`
}

func recoveryObjectiveLocation(o Objective, catalog ObjectiveCatalog) (LocationID, bool) {
	if o.Location != "" {
		return o.Location, true
	}
	if o.Place != "" {
		if destination, ok := catalog.destination(o.Place); ok && destination.Location != "" {
			return destination.Location, true
		}
		for _, challenge := range catalog.Challenges {
			if challenge.Place == o.Place && challenge.Location != "" {
				return challenge.Location, true
			}
		}
	}
	return "", false
}

// recoveryResumeLocation finds the active typed combat blocker whose objective
// should be cheap to resume after a healing detour. This is intentionally
// structural: generic recovery never parses objective/error prose.
func recoveryResumeLocation(known *Knowledge, catalog ObjectiveCatalog) (LocationID, bool) {
	if known == nil {
		return "", false
	}
	type candidate struct {
		location LocationID
		times    int
		target   int
		key      string
	}
	var best candidate
	found := false
	for storage, failure := range known.Failures {
		key, mode, ok := parseFailureStorageKey(storage)
		if !ok {
			continue
		}
		switch mode {
		case failureModeCombatLoss, legacyFailureModeTrainerLoss, legacyFailureModeGymLoss:
		default:
			continue
		}
		location, ok := recoveryObjectiveLocation(combatRecoveryObjective(key.Objective()), catalog)
		if !ok {
			continue
		}
		next := candidate{location: location, times: failure.Times, target: failure.ReadinessTarget, key: storage}
		if !found || next.times > best.times ||
			(next.times == best.times && next.target > best.target) ||
			(next.times == best.times && next.target == best.target && next.key < best.key) {
			best, found = next, true
		}
	}
	return best.location, found
}

func recoveryFallbackTravelCost(current, target LocationID, known *Knowledge) (int, bool) {
	if current == target {
		return 0, true
	}
	if known == nil || len(known.Adjacency) == 0 {
		return 0, false
	}
	hops, ok := mapHops(known.Adjacency, current)[target]
	if !ok {
		return 0, false
	}
	return hops * recoveryMapHopCost, true
}

func recoveryReturnCost(center, resume LocationID, known *Knowledge) (int, bool) {
	if resume == "" || center == resume {
		return 0, true
	}
	if known == nil || len(known.Adjacency) == 0 {
		// Adapters without durable topology cannot price the return leg; keep
		// the candidate rather than inventing an unreachable route.
		return 0, true
	}
	hops, ok := mapHops(known.Adjacency, center)[resume]
	if !ok {
		return 0, false
	}
	return hops * recoveryMapHopCost, true
}

func rankRecoveryCheckpoints(
	obs Observation,
	known *Knowledge,
	knownLocations map[LocationID]bool,
	catalog ObjectiveCatalog,
	unroutable map[string]bool,
) []RecoveryCheckpointAssessment {
	current := observationLocation(obs, known)
	resume, hasResume := recoveryResumeLocation(known, catalog)
	out := make([]RecoveryCheckpointAssessment, 0, 4)

	for _, destination := range catalog.Destinations {
		if !destination.Center || destination.Location == "" {
			continue
		}
		active := destination.Place == obs.RecoveryCheckpoint
		local := catalog.CurrentCenter && destination.Location == current
		if !active && !local && !knownLocations[destination.Location] {
			continue
		}

		assessment := RecoveryCheckpointAssessment{
			Place:            destination.Place,
			Location:         destination.Location,
			ActiveCheckpoint: active,
			Routable:         true,
			FastTravel:       destination.FastTravel,
			FastTravelMethod: destination.FastTravelMethod,
		}
		if unroutable[string(destination.Place)] {
			assessment.Routable = false
			assessment.Reason = "live route planner rejected this Center"
			out = append(out, assessment)
			continue
		}
		if destination.TravelCostChecked && !destination.TravelCostKnown && !local {
			assessment.Routable = false
			assessment.Reason = "adapter could not find a legal route with current capabilities"
			out = append(out, assessment)
			continue
		}

		switch {
		case local:
			assessment.CurrentCost = 0
		case destination.TravelCostKnown:
			assessment.CurrentCost = destination.TravelCost
		default:
			cost, ok := recoveryFallbackTravelCost(current, destination.Location, known)
			if !ok {
				if active && len(known.Adjacency) == 0 {
					// The cartridge proves the checkpoint is legitimate even if
					// a compatibility fixture has no topology to price it.
					cost = 0
				} else {
					assessment.Routable = false
					assessment.Reason = "no known route to Center"
					out = append(out, assessment)
					continue
				}
			}
			assessment.CurrentCost = cost
		}

		if hasResume {
			returnCost, ok := recoveryReturnCost(destination.Location, resume, known)
			if !ok {
				assessment.Routable = false
				assessment.Reason = "Center cannot return to interrupted combat objective"
				out = append(out, assessment)
				continue
			}
			assessment.ReturnCost = returnCost
		}
		assessment.TotalCost = assessment.CurrentCost + assessment.ReturnCost
		if active {
			assessment.TotalCost -= recoveryActiveCheckpoint
			if assessment.TotalCost < 0 {
				assessment.TotalCost = 0
			}
		}
		if hasResume {
			assessment.Reason = "ranked by recovery route plus return-to-objective cost"
		} else {
			assessment.Reason = "ranked by recovery route cost"
		}
		out = append(out, assessment)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Routable != out[j].Routable {
			return out[i].Routable
		}
		if out[i].Routable && out[i].TotalCost != out[j].TotalCost {
			return out[i].TotalCost < out[j].TotalCost
		}
		if out[i].ActiveCheckpoint != out[j].ActiveCheckpoint {
			return out[i].ActiveCheckpoint
		}
		return out[i].Place < out[j].Place
	})
	for i := range out {
		if out[i].Routable {
			out[i].Selected = true
			break
		}
	}
	return out
}

func recoveryCheckpointPlaces(ranked []RecoveryCheckpointAssessment) []PlaceID {
	out := make([]PlaceID, 0, len(ranked))
	for _, assessment := range ranked {
		if assessment.Routable {
			out = append(out, assessment.Place)
		}
	}
	return out
}
