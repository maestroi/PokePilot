package agent

import "github.com/maestroi/pokepilot/skill"

// Offer narrows the objectives to the ones that make sense right now.
//
// A menu that still lists what has already been done invites the planner
// to do it again: GetStarter is idempotent, so "take a starter" succeeds
// instantly forever, the observation never changes, and the run stops as
// stuck having achieved nothing. That is not a bad model — it is a bad
// question, and the fix belongs here rather than in the prompt.
//
// It says what is POSSIBLE, never what is WISE. An objective that is
// legal but unwise stays on the list: choosing badly is the planner's
// mistake to make, and filtering on judgement would mean we are playing
// the game rather than measuring whether it can.
func Offer(obs Observation, all []Objective) []Objective {
	out := make([]Objective, 0, len(all))
	for _, o := range all {
		switch o.Kind {
		case KindStarter:
			if obs.PartyCount > 0 {
				continue // already has one; the game offers no second
			}
		case KindGoTo:
			if d, ok := skill.Place(o.Place); ok &&
				d.Map == obs.Map && d.X == obs.X && d.Y == obs.Y {
				continue // already standing on it
			}
		}
		out = append(out, o)
	}
	return out
}
