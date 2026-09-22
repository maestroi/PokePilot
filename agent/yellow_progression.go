package agent

import (
	"github.com/maestroi/pokepilot/gen1"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func yellowProgressionNote(id ProgressID) string {
	switch id {
	case gen1.ProgressPokedexAcquired:
		return "(pick up Oak's Parcel in Viridian Mart, return to Oak's lab, and positively verify the Pokédex)"
	case gen1.ProgressBoulderBadge:
		return "(travel through Viridian Forest to Pewter, defeat Brock, and verify the Boulder Badge)"
	case gen1.ProgressMtMoonFossilAcquired:
		return "(reach Mt. Moon B2F, defeat the Super Nerd, and choose the Dome Fossil)"
	case yellowprofile.ProgressYellowMtMoonExitResolved:
		return "(after taking the fossil, trigger and defeat Yellow's Jessie & James encounter so Mt. Moon's eastern exit is truly resolved)"
	case gen1.ProgressSSTicketAcquired:
		return "(reach Bill's house, run the cell separator sequence, and obtain the S.S. Ticket)"
	case gen1.ProgressHM01Acquired:
		return "(enter the ticket-gated S.S. Anne, resolve the 2F rival, reach the Captain, and receive HM01 Cut)"
	default:
		return ""
	}
}

func yellowProgressionKnown(id ProgressID) bool {
	switch id {
	case gen1.ProgressPokedexAcquired,
		gen1.ProgressBoulderBadge,
		gen1.ProgressMtMoonFossilAcquired,
		yellowprofile.ProgressYellowMtMoonExitResolved,
		gen1.ProgressSSTicketAcquired,
		gen1.ProgressHM01Acquired:
		return true
	default:
		return false
	}
}

// ProgressionObjectives consumes the shared Gen-I early campaign order and
// inserts only the story pivots that are genuinely Yellow-specific.
func (a *yellowObjectiveAdapter) ProgressionObjectives(obs Observation) []Objective {
	if !obs.Story.Has(yellowprofile.ProgressYellowLabRivalResolved) {
		return nil
	}

	next, ok := gen1.FirstIncomplete(obs.Story, gen1.EarlyCampaignStages())
	if !ok {
		return nil
	}
	if next == gen1.ProgressSSTicketAcquired &&
		obs.Story.Has(gen1.ProgressMtMoonFossilAcquired) &&
		!obs.Story.Has(yellowprofile.ProgressYellowMtMoonExitResolved) {
		next = yellowprofile.ProgressYellowMtMoonExitResolved
	}
	return []Objective{{
		Kind:     KindProgress,
		Progress: next,
		Note:     yellowProgressionNote(next),
	}}
}
