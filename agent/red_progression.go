package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// redProgressionExecutor owns the game mechanics for one progression goal.
// It never decides success: a nil return only claims the mechanics finished.
type redProgressionExecutor func(m *emu.Emu, romData []byte, policy skill.MovePolicy) error

// redProgressionExecutors is Red's single ProgressID registry. Every entry is
// executable, and its positive semantic verifier is the same-ID fact that
// redProgressStateFromRAM projects into Observation.Story; the objective
// runtime checks that fact after the transaction and is the only owner of
// final success (#1655). TestRedProgressionRegistryHasVerifiers fails when an
// executor is registered without a projected verifier.
var redProgressionExecutors = map[ProgressID]redProgressionExecutor{
	redProgressMtMoonFossilAcquired:       skill.MtMoonFossil,
	redProgressPokedexAcquired:            skill.OaksParcel,
	redProgressSSTicketAcquired:           skill.Bill,
	redProgressHM01Acquired:               skill.SSAnneHM01,
	redProgressBicycleAcquired:            skill.AcquireBicycle,
	redProgressBoulderBadge:               skill.BoulderProgression,
	redProgressThunderBadge:               skill.SurgeProgression,
	redProgressPostSurgeLavenderReached:   skill.PostSurgeReachLavender,
	redProgressPostSurgeCeladonReady:      skill.PostSurgeReachCeladon,
	redProgressFlyReady:                   skill.PrepareFlyFastTravel,
	redProgressRainbowBadge:               skill.PostSurgeDefeatErika,
	redProgressSilphScopeAcquired:         skill.RocketHideout,
	redProgressPokeFluteAcquired:          skill.PokemonTower,
	redProgressFuchsiaProgressionComplete: skill.FuchsiaProgression,
	ProgressSaffronGateOpen:               skill.OpenSaffronGate,
	ProgressCardKeyOwned:                  skill.AcquireSilphCardKey,
	redProgressSilphRescueComplete:        skill.ClearSilphCo,
	ProgressSecretKeyOwned:                skill.AcquireCinnabarSecretKey,
	redProgressVolcanoBadge:               skill.CinnabarProgression,
	redProgressEarthBadge:                 skill.ViridianProgression,
	ProgressRoute22RivalResolved:          skill.VictoryRoadResolveRival,
	ProgressRoute23BadgeChecks:            skill.VictoryRoadReachCave,
	redProgressVictoryRoadCleared:         skill.VictoryRoadClearCave,
	redProgressIndigoPlateauReady:         skill.VictoryRoadPrepareIndigo,
	ProgressLeagueChallengeStarted:        skill.LeagueStartChallenge,
	redProgressLeagueLoreleiDefeated:      skill.LeagueDefeatLorelei,
	redProgressLeagueBrunoDefeated:        skill.LeagueDefeatBruno,
	redProgressLeagueAgathaDefeated:       skill.LeagueDefeatAgatha,
	redProgressLeagueLanceDefeated:        skill.LeagueDefeatLance,
	ProgressLeagueChampionDefeated:        skill.LeagueDefeatChampion,
	ProgressMainStoryComplete: func(m *emu.Emu, _ []byte, _ skill.MovePolicy) error {
		return skill.LeagueFinishHallOfFame(m)
	},
}

// redProgressionKnown is the adapter-owned vocabulary accepted by Red. The
// generic runtime treats ProgressID as opaque and only verifies that the
// requested fact became true.
func redProgressionKnown(id ProgressID) bool {
	_, ok := redProgressionExecutors[id]
	return ok
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

// redFlyFieldUnlocked is the recovery half of Fly progression. Once HM02 is
// in the bag, a failed/paused preparation transaction must remain offerable
// even if it stopped outside the geographic Celadon-ready checkpoint.
func redFlyFieldUnlocked(obs Observation) bool {
	fly, ok := observedFieldCapability(obs, "fly")
	return ok && fly.BadgeOwned && fly.HMOwned
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
	if obs.Story.Has(redProgressPokedexAcquired) && !hasBadge(obs, state.BadgeBoulder) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressBoulderBadge,
			Note:     "(travel through Viridian Forest to Pewter, challenge Brock, and positively verify the Boulder Badge before Route 3)",
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
	// Celadon's only west/south exits to Vermilion run through Saffron City,
	// and Saffron's four guardhouses are shut until a guard accepts a drink
	// (BIT_GAVE_SAFFRON_GUARDS_DRINK). The drink itself is an ordinary
	// ¥200 Celadon-roof vending purchase that needs no badge, HM, or story
	// fact, so this prerequisite must be offered BEFORE the Thunder Badge it
	// unblocks. It used to sit behind redProgressFuchsiaProgressionComplete,
	// which is derived from the Soul Badge + Surf + Strength — and Surf is
	// only reachable past the Thunder Badge via Rock Tunnel. Celadon, Route 8
	// and Lavender are in between, so a run that had already crossed into
	// Celadon with two badges could never route back to Vermilion: every
	// "return to Vermilion" attempt died on
	// transition "red:saffron_guard_drink" missing can_enter_saffron while
	// the only action that satisfies it stayed unoffered
	// (run-jxh8lk19wv6on, run-1biaubd9xooqm). Serve the small always-available
	// prerequisite first; Fuchsia keeps its own later ordering below.
	if obs.Story.Has(redProgressHM01Acquired) && !obs.Story.Has(ProgressSaffronGateOpen) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: ProgressSaffronGateOpen,
			Note:     "(reopen Saffron's guardhouses with a drink: buy Fresh Water from the Celadon roof vending machine and walk into the Route 7 gate guard trigger; this is the only route between Celadon and Vermilion)",
		})
	}
	if obs.Story.Has(redProgressHM01Acquired) && redCutFieldUnlocked(obs) &&
		obs.Story.Has(ProgressSaffronGateOpen) && !hasBadge(obs, state.BadgeThunder) {
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
	if hasBadge(obs, state.BadgeThunder) {
		switch {
		case !obs.Story.Has(redProgressRainbowBadge) && !obs.Story.Has(redProgressPostSurgeLavenderReached):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressPostSurgeLavenderReached,
				Note:     "(with the declared Cut prerequisite usable, travel through Cerulean and Route 9, then cross Rock Tunnel to the Lavender checkpoint; Flash is optional for ROM-driven navigation)",
			})
		case !obs.Story.Has(redProgressRainbowBadge) && !obs.Story.Has(redProgressPostSurgeCeladonReady):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressPostSurgeCeladonReady,
				Note:     "(from Lavender or later, continue through Route 8/7's Underground Path to the Celadon Pokemon Center and fully recover the party before Erika)",
			})
		case !obs.Story.Has(redProgressRainbowBadge):
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressRainbowBadge,
				Note:     "(from a recovered Celadon state with the declared Cut prerequisite usable, take the short gym approach, defeat Erika, and verify the Rainbow Badge)",
			})
		}

		// Fly is a speed optimization, not a correctness gate. Once Celadon is
		// ready and the Poke Flute makes the Route 16 detour reversible, offer
		// Fly alongside the mandatory story step instead of replacing it.
		if !obs.Story.Has(redProgressFlyReady) &&
			obs.Story.Has(redProgressPostSurgeCeladonReady) &&
			obs.Story.Has(redProgressPokeFluteAcquired) {
			out = append(out, Objective{
				Kind:     KindProgress,
				Progress: redProgressFlyReady,
				Note:     "(optional fast-travel setup: use the declared Cut/Fly prerequisites around the Route 16 HM02 handoff; skip this optimization if roster recovery is not worthwhile)",
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
	run, ok := redProgressionExecutors[o.Progress]
	if !ok {
		return fmt.Errorf("agent: %s: unknown Red progression goal %q", o, o.Progress)
	}
	return run(m, romData, skill.StatAwareMove(romData))
}
