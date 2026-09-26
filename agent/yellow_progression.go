package agent

import (
	"github.com/maestroi/pokepilot/gen1"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

// yellowSharedStoryBeat identifies Gen-I story transactions whose Yellow
// scripts are intentionally compatible with the shared executor. Keep this
// list narrow: later Yellow routes add Jessie/James or other version-specific
// sequencing and must stay Yellow-owned until ported deliberately.
func yellowSharedStoryBeat(id ProgressID) bool {
	switch id {
	case gen1.ProgressPokedexAcquired, gen1.ProgressBoulderBadge, gen1.ProgressMtMoonFossilAcquired:
		return true
	default:
		return false
	}
}

func yellowProgressionKnown(id ProgressID) bool {
	return id == yellowprofile.ProgressYellowLabRivalResolved || id == yellowprofile.ProgressYellowMtMoonExitResolved || yellowSharedStoryBeat(id)
}

// ProgressionObjectives exposes only Yellow story steps that the current
// adapter can execute. The scripted Pikachu opening remains Yellow-owned.
// Oak's parcel/Pokedex and Brock are shared Gen-I transactions whose Yellow
// scripts and semantic postconditions match, so Yellow may reuse those exact
// executors without opting into later Red story assumptions.
func (a *yellowObjectiveAdapter) ProgressionObjectives(obs Observation) []Objective {
	if !obs.Story.Has(yellowprofile.ProgressYellowLabRivalResolved) {
		if obs.PartyCount == 0 && !obs.Story.Has(yellowprofile.ProgressYellowStarterReceived) {
			return nil
		}
		return []Objective{{
			Kind:     KindProgress,
			Progress: yellowprofile.ProgressYellowLabRivalResolved,
			Note:     "(resume Yellow's scripted Pikachu opening and finish the lab rival sequence at a stable boundary)",
		}}
	}

	if !obs.Story.Has(gen1.ProgressPokedexAcquired) {
		return []Objective{{
			Kind:     KindProgress,
			Progress: gen1.ProgressPokedexAcquired,
			Note:     "(deliver Oak's parcel and acquire the Pokedex)",
		}}
	}
	if !obs.Story.Has(gen1.ProgressBoulderBadge) {
		return []Objective{{
			Kind:     KindProgress,
			Progress: gen1.ProgressBoulderBadge,
			Note:     "(travel through Viridian Forest to Pewter, challenge Brock, and verify the Boulder Badge)",
		}}
	}
	if !obs.Story.Has(gen1.ProgressMtMoonFossilAcquired) {
		return []Objective{{
			Kind:     KindProgress,
			Progress: gen1.ProgressMtMoonFossilAcquired,
			Note:     "(cross Mt. Moon, defeat the Super Nerd, and take the Dome Fossil before Yellow's exit encounter)",
		}}
	}
	if !obs.Story.Has(yellowprofile.ProgressYellowMtMoonExitResolved) {
		return []Objective{{
			Kind:     KindProgress,
			Progress: yellowprofile.ProgressYellowMtMoonExitResolved,
			Note:     "(trigger and defeat Yellow's Jessie & James encounter after the fossil so Mt. Moon's east exit is resolved)",
		}}
	}
	return nil
}
