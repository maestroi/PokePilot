package agent

// ProgressionPlanner is the game-specific half of progression planning. Core
// offering knows only that these are semantic state changes; a concrete game
// decides which goals are currently meaningful and executable.
type ProgressionPlanner interface {
	ProgressionObjectives(Observation) []Objective
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
