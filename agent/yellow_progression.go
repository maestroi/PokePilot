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
	case gen1.ProgressPokedexAcquired,
		gen1.ProgressBoulderBadge,
		gen1.ProgressMtMoonFossilAcquired,
		gen1.ProgressSSTicketAcquired,
		gen1.ProgressHM01Acquired,
		gen1.ProgressThunderBadge,
		gen1.ProgressPostSurgeLavenderReached,
		gen1.ProgressPostSurgeCeladonReady,
		gen1.ProgressRainbowBadge,
		gen1.ProgressSilphScopeAcquired,
		gen1.ProgressPokeFluteAcquired,
		gen1.ProgressFuchsiaProgressionComplete:
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
	if !obs.Story.Has(gen1.ProgressSSTicketAcquired) {
		return []Objective{{
			Kind:     KindProgress,
			Progress: gen1.ProgressSSTicketAcquired,
			Note:     "(help Bill at the end of Route 25 and obtain the S.S. Ticket)",
		}}
	}
	if !obs.Story.Has(gen1.ProgressHM01Acquired) {
		return []Objective{{
			Kind:     KindProgress,
			Progress: gen1.ProgressHM01Acquired,
			Note:     "(board the S.S. Anne, resolve its rival sequence, and receive HM01 Cut from the Captain)",
		}}
	}
	if cascade := gen1CascadeBadgeObjectives(obs); len(cascade) != 0 {
		return cascade
	}

	next, ok := gen1.FirstIncomplete(obs.Story, gen1.MiddleCampaignStages())
	if !ok {
		next, ok = gen1.FirstIncomplete(obs.Story, gen1.RocketTowerStages())
	}
	if !ok {
		next, ok = gen1.FirstIncomplete(obs.Story, gen1.FuchsiaStages())
	}
	if ok {
		note := map[ProgressID]string{
			gen1.ProgressThunderBadge:               "(prepare Cut, enter Vermilion Gym, solve the live trash-can switches, defeat Lt. Surge, and verify the Thunder Badge)",
			gen1.ProgressPostSurgeLavenderReached:   "(cross Route 9 and Rock Tunnel to Lavender; Flash is optional because navigation is ROM-driven)",
			gen1.ProgressPostSurgeCeladonReady:      "(continue from Lavender through Route 8/7's Underground Path to Celadon Pokemon Center and fully recover)",
			gen1.ProgressRainbowBadge:               "(use Cut for the Celadon Gym approach, defeat Erika, and verify the Rainbow Badge)",
			gen1.ProgressSilphScopeAcquired:         "(clear the Celadon Rocket Hideout; Yellow's B4F Jessie/James interruption is resolved on the shared live-topology route before Giovanni and the Silph Scope)",
			gen1.ProgressPokeFluteAcquired:          "(return to Lavender, clear Pokémon Tower including Yellow's 7F Jessie/James interruption, rescue Mr. Fuji, and receive the Poké Flute)",
			gen1.ProgressFuchsiaProgressionComplete: "(wake Route 12 Snorlax with the Poké Flute, reach Fuchsia, defeat Koga for Soul, then collect HM03 Surf and HM04 Strength through the shared Safari/Warden transaction)",
		}[next]
		return []Objective{{
			Kind:     KindProgress,
			Progress: next,
			Note:     note,
		}}
	}

	// Fuchsia owns the durable HM03/HM04 handoff; the generic field-capability
	// repair engine owns teaching/rearranging carriers. Cinnabar requires Surf,
	// but not Strength, so repair only the capability this route actually uses.
	// HM04 remains owned and can be prepared lazily when a later route needs it.
	if !fieldCapabilityUsable(obs, "surf") {
		return []Objective{{
			Kind:            KindRepairFieldCapability,
			FieldCapability: "surf",
			Note:            "(prepare the newly acquired Surf field move on a usable party carrier before the Cinnabar leg)",
		}}
	}

	const cinnabarCenter PlaceID = "cinnabar pokemon center"
	if !progressionAtPlaceLocation(obs, cinnabarCenter) {
		return []Objective{{
			Kind:  KindGoTo,
			Place: cinnabarCenter,
			Note:  "(use the shared Gen-I route graph with Surf/Strength prepared and establish Cinnabar as the next stable campaign handoff)",
		}}
	}
	return nil
}

func progressionAtPlaceLocation(obs Observation, place PlaceID) bool {
	destination, ok := objectiveCatalogForObservation(obs).destination(place)
	return ok && destination.Location != "" && destination.Location == LocationID(obs.Location)
}
