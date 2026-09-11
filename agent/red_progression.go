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
		redProgressRainbowBadge,
		redProgressSilphScopeAcquired,
		redProgressPokeFluteAcquired,
		redProgressFuchsiaProgressionComplete,
		redProgressSilphRescueComplete,
		redProgressVolcanoBadge,
		redProgressEarthBadge,
		redProgressIndigoPlateauReady,
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
// already unlocked and #107's party/PC/catch repair path can prepare a carrier.
func redVermilionGymRecoveryAvailable(obs Observation) bool {
	if obs.Map != 0x05 || hasBadge(obs, state.BadgeThunder) {
		return false
	}
	if !routeBlockedOn(obs, "vermilion gym", "can_cut") {
		return false
	}
	cut, ok := observedFieldCapability(obs, "cut")
	return ok && cut.BadgeOwned && cut.HMOwned
}

// redProgressionObjectives exposes Red story opportunities as one portable
// objective shape. Availability is Red knowledge; the planner only sees the
// semantic state change requested by each objective.
func redProgressionObjectives(obs Observation) []Objective {
	out := make([]Objective, 0, 15)
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
	if redVermilionGymRecoveryAvailable(obs) {
		out = append(out, Objective{
			Kind:  KindGym,
			Place: "vermilion gym",
			Note:  "(prepare a compatible Cut carrier through the party/PC/catch recovery path, clear the exterior tree, and challenge Lt. Surge)",
		})
	}
	if hasBadge(obs, state.BadgeThunder) && !obs.Story.Has(redProgressRainbowBadge) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressRainbowBadge,
			Note:     "(repair or retain a Cut carrier, travel from Vermilion through Cerulean, Route 9, Rock Tunnel, Lavender, and the Underground Path to Celadon, heal, then defeat Erika for the Rainbow Badge; Flash is optional)",
		})
	}
	if obs.Story.Has(redProgressRainbowBadge) && skill.RocketHideoutAvailable(obs.Map) && !obs.Story.Has(redProgressSilphScopeAcquired) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressSilphScopeAcquired,
			Note:     "(clear the Rocket Hideout and acquire the Silph Scope)",
		})
	}
	if skill.PokemonTowerAvailable(obs.Map) &&
		obs.Story.Has(redProgressSilphScopeAcquired) &&
		!obs.Story.Has(redProgressPokeFluteAcquired) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressPokeFluteAcquired,
			Note:     "(clear Pokemon Tower and acquire the Poke Flute)",
		})
	}
	if skill.FuchsiaProgressionAvailable(obs.Map) &&
		obs.Story.Has(redProgressPokeFluteAcquired) &&
		!obs.Story.Has(redProgressFuchsiaProgressionComplete) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressFuchsiaProgressionComplete,
			Note:     "(reach Fuchsia, earn Soul Badge, and acquire Surf + Strength)",
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
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: redProgressIndigoPlateauReady,
			Note:     "(defeat the final Route 22 rival, pass Route 23's seven badge checks and three Surf bands, solve Victory Road's live Strength puzzles, then heal in the Indigo Plateau lobby)",
		})
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
	case redProgressRainbowBadge:
		return skill.PostSurgeCeladonProgression(m, romData, policy)
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
	case redProgressIndigoPlateauReady:
		return skill.VictoryRoadProgression(m, romData, policy)
	default:
		return fmt.Errorf("agent: %s: unknown Red progression goal %q", o, o.Progress)
	}
}
