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
		redProgressSilphScopeAcquired,
		redProgressPokeFluteAcquired,
		redProgressFuchsiaProgressionComplete:
		return true
	default:
		return false
	}
}

// redProgressionObjectives exposes Red story opportunities as one portable
// objective shape. Availability is Red knowledge; the planner only sees the
// semantic state change requested by each objective.
func redProgressionObjectives(obs Observation) []Objective {
	out := make([]Objective, 0, 5)
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
	if skill.RocketHideoutAvailable(obs.Map) && !obs.Story.Has(redProgressSilphScopeAcquired) {
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
	case redProgressSilphScopeAcquired:
		return skill.RocketHideout(m, romData, policy)
	case redProgressPokeFluteAcquired:
		return skill.PokemonTower(m, romData, policy)
	case redProgressFuchsiaProgressionComplete:
		return skill.FuchsiaProgression(m, romData, policy)
	default:
		return fmt.Errorf("agent: %s: unknown Red progression goal %q", o, o.Progress)
	}
}
