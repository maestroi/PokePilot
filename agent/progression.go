package agent

// ProgressionPlanner is the game-specific half of progression planning. Core
// offering knows only that these are semantic state changes; a concrete game
// decides which goals are currently meaningful and executable.
type ProgressionPlanner interface {
	ProgressionObjectives(Observation) []Objective
}

// withoutKnownUnroutableJourneys removes only travel objectives the live route
// planner has positively rejected. Generic Offer deliberately fails open when
// every journey is reported unroutable because a bad live-map overlay must not
// be able to empty the menu. Once a real progression objective exists, however,
// keeping those rejected journeys just gives the strategist known-bad choices
// instead of the semantic state change that can actually unlock the route.
func withoutKnownUnroutableJourneys(out []Objective, unroutable []string) []Objective {
	if len(unroutable) == 0 {
		return out
	}
	blocked := make(map[PlaceID]bool, len(unroutable))
	for _, name := range unroutable {
		blocked[PlaceID(name)] = true
	}
	filtered := make([]Objective, 0, len(out))
	for _, o := range out {
		if o.Kind == KindGoTo && blocked[o.Place] {
			continue
		}
		filtered = append(filtered, o)
	}
	return filtered
}

// OfferWithProgression composes generic actions with game-owned progression
// goals. Offer keeps journeys last; progression is a local verb, so insert it
// immediately before the first journey instead of appending it after travel.
func OfferWithProgression(obs Observation, known *Knowledge, p ProgressionPlanner) []Objective {
	out := Offer(obs, known)
	if p == nil {
		return out
	}
	progress := annotate(p.ProgressionObjectives(obs), known)
	if len(progress) == 0 {
		return out
	}
	out = withoutKnownUnroutableJourneys(out, obs.Unroutable)
	journeyAt := len(out)
	for i, o := range out {
		if o.Kind == KindGoTo {
			journeyAt = i
			break
		}
	}
	combined := make([]Objective, 0, len(out)+len(progress))
	combined = append(combined, out[:journeyAt]...)
	combined = append(combined, progress...)
	combined = append(combined, out[journeyAt:]...)
	return combined
}

func (a *redObjectiveAdapter) ProgressionObjectives(obs Observation) []Objective {
	return redProgressionObjectives(obs)
}
