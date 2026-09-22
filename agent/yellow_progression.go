package agent

import (
	"github.com/maestroi/pokepilot/gen1"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func yellowProgressionNote(id ProgressID) string {
	switch id {
	case yellowprofile.ProgressYellowLabRivalResolved:
		return "(resume Yellow's opening after Pikachu is received and finish the lab rival sequence at a stable boundary)"
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
	case gen1.ProgressCascadeBadge:
		return "(return to Cerulean, defeat Misty, and verify the Cascade Badge so HM01 Cut is legal)"
	case gen1.ProgressThunderBadge:
		return "(prepare Cut, enter Vermilion Gym, solve Yellow's live trash-can switches, defeat Lt. Surge, and verify the Thunder Badge)"
	case gen1.ProgressPostSurgeLavenderReached:
		return "(travel through Route 9 and Rock Tunnel to Lavender; use Flash automatically when available, but do not require it for ROM-driven navigation)"
	case gen1.ProgressPostSurgeCeladonReady:
		return "(continue from Lavender through the Underground Path to Celadon Pokemon Center and fully recover the party)"
	case gen1.ProgressRainbowBadge:
		return "(prepare Cut for the Celadon Gym approach, defeat Erika, and verify the Rainbow Badge)"
	default:
		return ""
	}
}

func yellowProgressionKnown(id ProgressID) bool {
	switch id {
	case yellowprofile.ProgressYellowLabRivalResolved,
		gen1.ProgressPokedexAcquired,
		gen1.ProgressBoulderBadge,
		gen1.ProgressMtMoonFossilAcquired,
		yellowprofile.ProgressYellowMtMoonExitResolved,
		gen1.ProgressSSTicketAcquired,
		gen1.ProgressHM01Acquired,
		gen1.ProgressCascadeBadge,
		gen1.ProgressThunderBadge,
		gen1.ProgressPostSurgeLavenderReached,
		gen1.ProgressPostSurgeCeladonReady,
		gen1.ProgressRainbowBadge:
		return true
	default:
		return false
	}
}

// ProgressionObjectives consumes the shared Gen-I early campaign order and
// inserts only the story pivots that are genuinely Yellow-specific.
func (a *yellowObjectiveAdapter) ProgressionObjectives(obs Observation) []Objective {
	if !obs.Story.Has(yellowprofile.ProgressYellowLabRivalResolved) {
		if obs.PartyCount == 0 && !obs.Story.Has(yellowprofile.ProgressYellowStarterReceived) {
			return nil
		}
		return []Objective{{
			Kind:     KindProgress,
			Progress: yellowprofile.ProgressYellowLabRivalResolved,
			Note:     yellowProgressionNote(yellowprofile.ProgressYellowLabRivalResolved),
		}}
	}

	next, ok := gen1.FirstIncomplete(obs.Story, gen1.EarlyCampaignStages())
	if ok {
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

	next, ok = gen1.FirstIncomplete(obs.Story, gen1.MiddleCampaignStages())
	if !ok {
		return nil
	}
	return []Objective{{
		Kind:     KindProgress,
		Progress: next,
		Note:     yellowProgressionNote(next),
	}}
}
