package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// redProgressionKnown is the adapter-owned vocabulary accepted by Red. The
// generic runtime treats ProgressID as opaque and only verifies that the
// requested fact became true.
func redProgressionKnown(id ProgressID) bool {
	switch id {
	case redProgressPokedexAcquired,
		redProgressMtMoonFossilAcquired,
		redProgressSSTicketAcquired,
		redProgressHM01Acquired,
		redProgressThunderBadge,
		redProgressPostSurgeLavenderReached,
		redProgressPostSurgeCeladonReady,
		redProgressRainbowBadge,
		redProgressSilphScopeAcquired,
		redProgressPokeFluteAcquired,
		redProgressFuchsiaProgressionComplete,
		redProgressSilphRescueComplete,
		redProgressVolcanoBadge,
		redProgressEarthBadge,
		ProgressRoute22RivalResolved,
		ProgressRoute23BadgeChecks,
		redProgressVictoryRoadCleared,
		redProgressIndigoPlateauReady,
		ProgressLeagueChallengeStarted,
		redProgressLeagueLoreleiDefeated,
		redProgressLeagueBrunoDefeated,
		redProgressLeagueAgathaDefeated,
		redProgressLeagueLanceDefeated,
		ProgressLeagueChampionDefeated,
		ProgressMainStoryComplete,
		ProgressSaffronGateOpen,
		ProgressCardKeyOwned,
		ProgressSecretKeyOwned:
		return true
	default:
		return false
	}
}

func routeBlockedOn(obs Observation, destination PlaceID, capability CapabilityID) bool {
	for _, blockage := range obs.RouteBlockages {
		if blockage.Destination != destination {
			continue
		}
		for _, missing := range blockage.Missing {
			if missing == capability {
				return true
			}
		}
	}
	return false
}

func observedFieldCapability(obs Observation, name CapabilityID) (FieldCapability, bool) {
	for _, cap := range obs.FieldCapabilities {
		if cap.Name == name {
			return cap, true
		}
	}
	return FieldCapability{}, false
}

// redVermilionGymRecoveryAvailable closes the gap between route legality and
// roster recovery. Generic Offer must hide a gym behind a missing semantic
// capability, but Red can still surface the atomic Surge challenge when Cut is
// already unlocked and the party/PC/catch repair path can prepare a carrier.
func redVermilionGymRecoveryAvailable(obs Observation) bool {
	if obs.Map != 0x05 || hasBadge(obs, state.BadgeThunder) {
		return false
	}
	if !routeBlockedOn(obs, "vermilion gym", "can_cut") {
		return false
	}
	return redCutFieldUnlocked(obs)
}

// redCutFieldUnlocked is the portable Cut gate: badge plus HM, not a named
// gym order. Thunder Badge progression and Vermilion gym recovery both need
// it; without the badge, Cut cannot be prepared or used.
func redCutFieldUnlocked(obs Observation) bool {
	cut, ok := observedFieldCapability(obs, "cut")
	return ok && cut.BadgeOwned && cut.HMOwned
}

// redProgressionObjectives exposes Red story opportunities as one portable
// objective shape. Availability is Red knowledge; the planner only sees the
// semantic state change requested by each objective.
func redProgressionObjectives(obs Observation) []Objective {
	out := make([]Objective, 0, 16)
	if skill.MtMoonProgressionAvailable(obs.Map) && !obs.Story.Has(redProgressMtMoonFossilAcquired) {
		out = append(out, Objective{Kind: KindProgress, Progress: redProgressMtMoonFossilAcquired, Note: "(defeat Mt. Moon's Super Nerd and choose the Dome Fossil to open the eastern exit)"})
	}
	if observedEvent(obs, state.EventBattledRivalInOaksLab.String()) &&
		!obs.Story.Has(redProgressPokedexAcquired) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressPokedexAcquired,
			Note:     "(deliver Oak's parcel and acquire the Pokedex)",
		})
	}
	if skill.BillProgressionAvailable(obs.Map) && !obs.Story.Has(redProgressSSTicketAcquired) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressSSTicketAcquired,
			Note:     "(help Bill at the end of Route 25 and obtain the S.S. Ticket, opening Cerulean's robbed-house route south)",
		})
	}
	if obs.Story.Has(redProgressSSTicketAcquired) && !obs.Story.Has(redProgressHM01Acquired) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressHM01Acquired,
			Note:     "(go to Vermilion, board the S.S. Anne with the ticket, defeat the scripted rival on 2F, and receive HM01 Cut from the Captain)",
		})
	}
	if obs.Story.Has(redProgressHM01Acquired) && redCutFieldUnlocked(obs) && !hasBadge(obs, state.BadgeThunder) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressThunderBadge,
			Note:     "(return to Vermilion from wherever the run currently is, repair or obtain a Cut carrier, clear the gym tree, solve the trash-can switches, defeat Lt. Surge, and verify the Thunder Badge before taking the Rock Tunnel/Lavender story leg)",
		})
	}
	if redVermilionGymRecoveryAvailable(obs) {
		out = append(out, Objective{
			Kind:  KindGym,
			Place: "vermilion gym",
			Note:  "(prepare a compatible Cut carrier through the party/PC/catch recovery path, clear the exterior tree, and challenge Lt. Surge)",
		})
	}
	if hasBadge(obs, state.BadgeThunder) && !obs.Story.Has(redProgressRainbowBadge) {
		switch {
		case !obs.Story.Has(redProgressPostSurgeLavenderReached):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressPostSurgeLavenderReached,
				Note:     "(repair or retain a Cut carrier, travel through Cerulean and Route 9, then cross Rock Tunnel to the Lavender checkpoint; Flash is optional for ROM-driven navigation)",
			})
		case !obs.Story.Has(redProgressPostSurgeCeladonReady):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressPostSurgeCeladonReady,
				Note:     "(from Lavender or later, continue through Route 8/7's Underground Path to the Celadon Pokemon Center and fully recover the party before Erika)",
			})
		default:
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressRainbowBadge,
				Note:     "(from a recovered Celadon state, revalidate the Cut carrier, take the short gym approach, defeat Erika, and verify the Rainbow Badge)",
			})
		}
	}
	// Silph Scope and the Poké Flute are later than Surge. The intended
	// critical path is Thunder Badge -> Rock Tunnel/Lavender/Celadon ->
	// Rocket Hideout/Silph Scope -> Pokémon Tower/Poké Flute -> Route 12 ->
	// Fuchsia, where Surf and Strength are finally acquired. Keeping these
	// facts ordered prevents the strategist from treating Surf as a Route 12
	// prerequisite or wandering into the sleeping Snorlax before the Flute.
	if hasBadge(obs, state.BadgeThunder) {
		if skill.RocketHideoutAvailable(obs.Map) && !obs.Story.Has(redProgressSilphScopeAcquired) {
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressSilphScopeAcquired,
				Note:     "(clear the Rocket Hideout and acquire the Silph Scope)",
			})
		}
		if obs.Story.Has(redProgressSilphScopeAcquired) &&
			skill.PokemonTowerAvailable(obs.Map) &&
			!obs.Story.Has(redProgressPokeFluteAcquired) {
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressPokeFluteAcquired,
				Note:     "(clear Pokemon Tower in Lavender and acquire the Poke Flute; this is what opens Route 12's Snorlax corridor)",
			})
		}
	}
	if skill.FuchsiaProgressionAvailable(obs.Map) &&
		obs.Story.Has(redProgressPokeFluteAcquired) &&
		!obs.Story.Has(redProgressFuchsiaProgressionComplete) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressFuchsiaProgressionComplete,
			Note:     "(use the Poke Flute on Route 12, reach Fuchsia, earn Soul Badge, then acquire Surf + Strength in the Safari/Warden story)",
		})
	}
	if obs.Story.Has(redProgressFuchsiaProgressionComplete) &&
		!obs.Story.Has(ProgressSaffronGateOpen) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: ProgressSaffronGateOpen,
			Note:     "(open Saffron access; reuse a guard drink or buy Fresh Water from the Celadon roof vending machine and give it to the Route 7 guard)",
		})
	}
	if obs.Story.Has(redProgressFuchsiaProgressionComplete) &&
		obs.Story.Has(ProgressSaffronGateOpen) &&
		!obs.Story.Has(ProgressCardKeyOwned) &&
		!obs.Story.Has(ProgressSilphCoCleared) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: ProgressCardKeyOwned,
			Note:     "(enter Silph Co, follow the stair topology to 5F, and collect the Card Key for the locked-door phase)",
		})
	}
	if obs.Story.Has(redProgressFuchsiaProgressionComplete) &&
		obs.Story.Has(ProgressSaffronGateOpen) &&
		(obs.Story.Has(ProgressCardKeyOwned) || obs.Story.Has(ProgressSilphCoCleared)) &&
		!obs.Story.Has(redProgressSilphRescueComplete) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressSilphRescueComplete,
			Note:     "(open the required Silph doors, take the 3F/7F warp route, defeat the rival and Giovanni, then receive the president's Master Ball reward)",
		})
	}
	if obs.Story.Has(redProgressSilphRescueComplete) &&
		hasBadge(obs, state.BadgeMarsh) &&
		!obs.Story.Has(ProgressSecretKeyOwned) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: ProgressSecretKeyOwned,
			Note:     "(Surf south from Pallet through Route 21, enter Pokemon Mansion, solve its live statue-gate topology, and collect the Secret Key)",
		})
	}
	if obs.Story.Has(ProgressSecretKeyOwned) && !obs.Story.Has(redProgressVolcanoBadge) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressVolcanoBadge,
			Note:     "(unlock Cinnabar Gym with the Secret Key, answer the six quiz gates from their ROM-declared YES/NO semantics, defeat Blaine, and verify the Volcano Badge)",
		})
	}
	if obs.Story.Has(redProgressVolcanoBadge) && !obs.Story.Has(redProgressEarthBadge) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressEarthBadge,
			Note:     "(return to Viridian so the seven-badge story gate opens the Gym, traverse its forced arrow tiles, defeat Giovanni, and verify all eight badges)",
		})
	}
	if obs.Story.Has(redProgressEarthBadge) && !obs.Story.Has(redProgressIndigoPlateauReady) {
		switch {
		case !obs.Story.Has(ProgressRoute22RivalResolved):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: ProgressRoute22RivalResolved,
				Note:     "(travel to Route 22, defeat the final rival, and positively verify the second Route 22 rival event before entering the League approach)",
			})
		case !obs.Story.Has(ProgressRoute23BadgeChecks):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: ProgressRoute23BadgeChecks,
				Note:     "(repair Surf, enter Route 23, cross its three live water bands, pass all seven badge checks, and end at the Victory Road 1F entry)",
			})
		case !obs.Story.Has(redProgressVictoryRoadCleared):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressVictoryRoadCleared,
				Note:     "(repair Strength, solve Victory Road's live 1F/2F/3F boulder sequence, and finish with the final 2F east switch positively set)",
			})
		default:
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressIndigoPlateauReady,
				Note:     "(leave the cleared cave, reach the Indigo Plateau lobby nurse, and fully recover HP, status, and PP before committing to the League)",
			})
		}
	}
	if obs.Story.Has(redProgressIndigoPlateauReady) && !obs.Story.Has(ProgressMainStoryComplete) {
		switch {
		case !obs.Story.Has(ProgressLeagueChallengeStarted):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: ProgressLeagueChallengeStarted,
				Note:     "(from the recovered Indigo checkpoint, enter Lorelei's room and positively commit the League challenge before any Elite Four battle)",
			})
		case !obs.Story.Has(redProgressLeagueLoreleiDefeated):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressLeagueLoreleiDefeated,
				Note:     "(defeat Lorelei and stop once her room-completion fact is committed)",
			})
		case !obs.Story.Has(redProgressLeagueBrunoDefeated):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressLeagueBrunoDefeated,
				Note:     "(advance from the completed Lorelei room, defeat Bruno, and stop at Bruno's committed completion fact)",
			})
		case !obs.Story.Has(redProgressLeagueAgathaDefeated):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressLeagueAgathaDefeated,
				Note:     "(advance from the completed Bruno room, defeat Agatha, and stop at Agatha's committed completion fact)",
			})
		case !obs.Story.Has(redProgressLeagueLanceDefeated):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressLeagueLanceDefeated,
				Note:     "(advance from the completed Agatha room, defeat Lance, and stop at Lance's committed completion fact)",
			})
		case !obs.Story.Has(ProgressLeagueChampionDefeated):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: ProgressLeagueChampionDefeated,
				Note:     "(advance from Lance's completed room, defeat the Champion, and positively verify the Champion victory event without folding the ending into this battle transaction)",
			})
		default:
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: ProgressMainStoryComplete,
				Note:     "(advance only the post-Champion Oak and Hall of Fame scripts until the durable main-story completion bit is recorded)",
			})
		}
	}
	return out
}

// executeRedProgression owns the game-specific mechanics for satisfying a
// semantic progression goal. Success is still decided later by the generic
// positive postcondition over Observation.Story, never by nil alone.
func executeRedProgression(m *emu.Emu, romData []byte, o Objective) error {
	policy := skill.StatAwareMove(romData)
	switch o.Progress {
	case redProgressMtMoonFossilAcquired:
		return skill.MtMoonFossil(m, romData, policy)
	case redProgressPokedexAcquired:
		return skill.OaksParcel(m, romData, policy)
	case redProgressSSTicketAcquired:
		return skill.Bill(m, romData, policy)
	case redProgressHM01Acquired:
		return skill.SSAnneHM01(m, romData, policy)
	case redProgressThunderBadge:
		return skill.SurgeProgression(m, romData, policy)
	case redProgressPostSurgeLavenderReached:
		return skill.PostSurgeReachLavender(m, romData, policy)
	case redProgressPostSurgeCeladonReady:
		return skill.PostSurgeReachCeladon(m, romData, policy)
	case redProgressRainbowBadge:
		return skill.PostSurgeDefeatErika(m, romData, policy)
	case redProgressSilphScopeAcquired:
		return skill.RocketHideout(m, romData, policy)
	case redProgressPokeFluteAcquired:
		return skill.PokemonTower(m, romData, policy)
	case redProgressFuchsiaProgressionComplete:
		return skill.FuchsiaProgression(m, romData, policy)
	case ProgressSaffronGateOpen:
		return skill.OpenSaffronGate(m, romData, policy)
	case ProgressCardKeyOwned:
		return skill.AcquireSilphCardKey(m, romData, policy)
	case redProgressSilphRescueComplete:
		return skill.ClearSilphCo(m, romData, policy)
	case ProgressSecretKeyOwned:
		return skill.AcquireCinnabarSecretKey(m, romData, policy)
	case redProgressVolcanoBadge:
		return skill.CinnabarProgression(m, romData, policy)
	case redProgressEarthBadge:
		return skill.ViridianProgression(m, romData, policy)
	case ProgressRoute22RivalResolved:
		return skill.VictoryRoadResolveRival(m, romData, policy)
	case ProgressRoute23BadgeChecks:
		return skill.VictoryRoadReachCave(m, romData, policy)
	case redProgressVictoryRoadCleared:
		return skill.VictoryRoadClearCave(m, romData, policy)
	case redProgressIndigoPlateauReady:
		return skill.VictoryRoadPrepareIndigo(m, romData, policy)
	case ProgressLeagueChallengeStarted:
		return skill.LeagueStartChallenge(m, romData, policy)
	case redProgressLeagueLoreleiDefeated:
		return skill.LeagueDefeatLorelei(m, romData, policy)
	case redProgressLeagueBrunoDefeated:
		return skill.LeagueDefeatBruno(m, romData, policy)
	case redProgressLeagueAgathaDefeated:
		return skill.LeagueDefeatAgatha(m, romData, policy)
	case redProgressLeagueLanceDefeated:
		return skill.LeagueDefeatLance(m, romData, policy)
	case ProgressLeagueChampionDefeated:
		return skill.LeagueDefeatChampion(m, romData, policy)
	case ProgressMainStoryComplete:
		return skill.LeagueFinishHallOfFame(m)
	default:
		return fmt.Errorf("agent: %s: unknown Red progression goal %q", o, o.Progress)
	}
}
