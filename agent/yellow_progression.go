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
		gen1.ProgressFuchsiaProgressionComplete,
		gen1.ProgressSecretKeyOwned,
		gen1.ProgressVolcanoBadge,
		gen1.ProgressSaffronGateOpen,
		gen1.ProgressCardKeyOwned,
		gen1.ProgressSilphRescueComplete,
		gen1.ProgressEarthBadge,
		gen1.ProgressRoute22RivalResolved,
		gen1.ProgressRoute23BadgeChecks,
		gen1.ProgressVictoryRoadCleared,
		gen1.ProgressIndigoPlateauReady:
		return true
	default:
		return false
	}
}

func yellowProgressionKnown(id ProgressID) bool {
	return id == yellowprofile.ProgressYellowLabRivalResolved || id == yellowprofile.ProgressYellowMtMoonExitResolved || yellowSharedStoryBeat(id)
}

// yellowSharedProgressionPrerequisites owns ordering where Yellow deliberately
// reuses a shared executor but not Red's campaign policy. The Mansion mechanics
// need Surf; Silph/Sabrina are independent story branches and are therefore
// not prerequisites for Yellow's Cinnabar leg.
func yellowSharedProgressionPrerequisites(id ProgressID, obs Observation) (bool, error) {
	switch id {
	case gen1.ProgressSecretKeyOwned:
		if !obs.Story.Has(gen1.ProgressFuchsiaProgressionComplete) {
			return true, progressionPrerequisiteError([]ProgressID{gen1.ProgressFuchsiaProgressionComplete})
		}
		if !fieldCapabilityUsable(obs, "surf") {
			return true, fieldCapabilityPrerequisiteError([]CapabilityID{"surf"})
		}
		return true, nil
	case gen1.ProgressVolcanoBadge:
		if !obs.Story.Has(gen1.ProgressSecretKeyOwned) {
			return true, progressionPrerequisiteError([]ProgressID{gen1.ProgressSecretKeyOwned})
		}
		return true, nil
	case gen1.ProgressCardKeyOwned:
		if !obs.Story.Has(gen1.ProgressSaffronGateOpen) {
			return true, progressionPrerequisiteError([]ProgressID{gen1.ProgressSaffronGateOpen})
		}
		return true, nil
	case gen1.ProgressSilphRescueComplete:
		missing := make([]ProgressID, 0, 2)
		if !obs.Story.Has(gen1.ProgressSaffronGateOpen) {
			missing = append(missing, gen1.ProgressSaffronGateOpen)
		}
		if !obs.Story.Has(gen1.ProgressCardKeyOwned) {
			missing = append(missing, gen1.ProgressCardKeyOwned)
		}
		if len(missing) != 0 {
			return true, progressionPrerequisiteError(missing)
		}
		return true, nil
	case gen1.ProgressEarthBadge:
		// Yellow deliberately allows Cinnabar before Silph/Sabrina. Giovanni's
		// city script still requires all seven prior badges, so Volcano alone is
		// not a sufficient prerequisite on this campaign ordering.
		missing := make([]ProgressID, 0, 3)
		for _, required := range []ProgressID{
			gen1.ProgressFuchsiaProgressionComplete,
			gen1.ProgressMarshBadge,
			gen1.ProgressVolcanoBadge,
		} {
			if !obs.Story.Has(required) {
				missing = append(missing, required)
			}
		}
		if len(missing) != 0 {
			return true, progressionPrerequisiteError(missing)
		}
		return true, nil
	default:
		return false, nil
	}
}

// yellowMarshBadgeObjectives keeps Sabrina's story ordering Yellow-owned while
// reusing the shared semantic travel and gym executors. Silph must be fully
// rescued first, including Yellow's 11F Jessie/James interruption.
func yellowMarshBadgeObjectives(obs Observation) []Objective {
	if !obs.Story.Has(gen1.ProgressSilphRescueComplete) || obs.Story.Has(gen1.ProgressMarshBadge) {
		return nil
	}
	if obs.Location == PlaceID("saffron gym") {
		return []Objective{{
			Kind:  KindGym,
			Place: "saffron gym",
			Note:  "(Silph is clear; traverse Saffron Gym's warp maze, defeat Sabrina, and verify the Marsh Badge)",
		}}
	}
	return []Objective{{
		Kind:  KindGoTo,
		Place: "saffron gym",
		Note:  "(Silph is clear; travel to Saffron Gym so Sabrina can be defeated for the Marsh Badge)",
	}}
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
	// but not Strength, so repair only while that route is still incomplete.
	// Once Blaine is complete, an unrelated Saffron step must not be blocked by
	// a party reshuffle that temporarily makes Surf unusable.
	if next, ok := gen1.FirstIncomplete(obs.Story, gen1.CinnabarStages()); ok {
		if !fieldCapabilityUsable(obs, "surf") {
			return []Objective{{
				Kind:            KindRepairFieldCapability,
				FieldCapability: "surf",
				Note:            "(prepare the newly acquired Surf field move on a usable party carrier before the Cinnabar leg)",
			}}
		}
		note := map[ProgressID]string{
			gen1.ProgressSecretKeyOwned: "(enter Pokemon Mansion from Cinnabar, solve the live statue-gate topology, and collect the Secret Key)",
			gen1.ProgressVolcanoBadge:   "(unlock Cinnabar Gym with the Secret Key, answer the six ROM-declared quiz gates, defeat Blaine, and verify the Volcano Badge)",
		}[next]
		return []Objective{{
			Kind:     KindProgress,
			Progress: next,
			Note:     note,
		}}
	}

	if next, ok := gen1.FirstIncomplete(obs.Story, gen1.SaffronStages()); ok {
		if next == gen1.ProgressMarshBadge {
			return yellowMarshBadgeObjectives(obs)
		}
		note := map[ProgressID]string{
			gen1.ProgressSaffronGateOpen:     "(buy a guard drink in Celadon and open Saffron's guardhouses through the shared Gen-I route-gate transaction)",
			gen1.ProgressCardKeyOwned:        "(enter Silph Co, follow the stair topology to 5F, and collect the Card Key)",
			gen1.ProgressSilphRescueComplete: "(open the required Silph doors, resolve the rival and Yellow's 11F Jessie/James interruption, defeat Giovanni, and receive the president's Master Ball)",
		}[next]
		return []Objective{{
			Kind:     KindProgress,
			Progress: next,
			Note:     note,
		}}
	}

	if next, ok := gen1.FirstIncomplete(obs.Story, gen1.LeagueApproachStages()); ok {
		note := map[ProgressID]string{
			gen1.ProgressEarthBadge:          "(return to Viridian after all seven prior badges, let the city script open the Gym, traverse the live arrow tiles, defeat Giovanni, and verify the Earth Badge)",
			gen1.ProgressRoute22RivalResolved: "(travel to Route 22, defeat Yellow's final rival team, and settle the complete after-battle exit script)",
			gen1.ProgressRoute23BadgeChecks:   "(prepare Surf, cross Route 23's three live water bands, pass all seven badge gates, and enter Victory Road 1F)",
			gen1.ProgressVictoryRoadCleared:   "(prepare Surf and Strength, solve the live 1F/2F/3F boulder chain, and clear Victory Road)",
			gen1.ProgressIndigoPlateauReady:   "(leave the cleared cave, reach the Indigo Plateau lobby, and fully recover the party before the League)",
		}[next]
		return []Objective{{
			Kind:     KindProgress,
			Progress: next,
			Note:     note,
		}}
	}
	return nil
}
