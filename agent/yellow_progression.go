package agent

import yellowprofile "github.com/maestroi/pokepilot/yellow/profile"

func yellowProgressionKnown(id ProgressID) bool {
	return id == yellowprofile.ProgressYellowLabRivalResolved
}

// ProgressionObjectives owns only the Yellow story steps that are executable on
// this adapter. The fresh opening is offered by the starter provider while the
// party is empty. If a checkpoint already contains Pikachu but the lab rival
// transaction has not reached its durable boundary, progression resumes that
// same state-driven opening controller rather than replaying Red's story.
func (a *yellowObjectiveAdapter) ProgressionObjectives(obs Observation) []Objective {
	if obs.Story.Has(yellowprofile.ProgressYellowLabRivalResolved) {
		return nil
	}
	if obs.PartyCount == 0 && !obs.Story.Has(yellowprofile.ProgressYellowStarterReceived) {
		return nil
	}
	return []Objective{{
		Kind:     KindProgress,
		Progress: yellowprofile.ProgressYellowLabRivalResolved,
		Note:     "(resume Yellow's scripted Pikachu opening and finish the lab rival sequence at a stable boundary)",
	}}
}
