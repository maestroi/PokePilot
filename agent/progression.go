package agent

// ProgressionPlanner is the game-specific half of progression planning. Core
// offering knows only that these are semantic state changes; a concrete game
// decides which goals are currently meaningful and executable.
type ProgressionPlanner interface {
	ProgressionObjectives(Observation) []Objective
}

// OfferWithProgression composes the generic action menu with game-owned
// progression goals. Progression objectives use the same run-history annotation
// as every other choice without teaching Offer any named game story facts.
func OfferWithProgression(obs Observation, known *Knowledge, p ProgressionPlanner) []Objective {
	out := Offer(obs, known)
	if p == nil {
		return out
	}
	progress := p.ProgressionObjectives(obs)
	if len(progress) == 0 {
		return out
	}
	return append(out, annotate(progress, known)...)
}

func (a *redObjectiveAdapter) ProgressionObjectives(obs Observation) []Objective {
	return redProgressionObjectives(obs)
}
