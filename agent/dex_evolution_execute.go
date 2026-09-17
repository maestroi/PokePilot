package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func executeDexEvolutionTraining(m *emu.Emu, romData []byte, o Objective, result ObjectiveResult) (ObjectiveResult, error) {
	from, ok := redSpeciesID(o.Species)
	if !ok {
		return result, fmt.Errorf("agent: %s: unknown Red species %q", o, o.Species)
	}
	to, ok := levelEvolutionTarget(romData, from)
	if !ok {
		return result, fmt.Errorf("agent: %s: species %q has no level evolution", o, o.Species)
	}

	trained, err := executeTrainingObjective(m, romData, o, result)
	if err != nil {
		return trained, err
	}
	wantDex, err := rom.InternalSpeciesDexNumber(romData, to)
	if err != nil {
		return trained, fmt.Errorf("agent: %s: evolution target %#02x: %w", o, to, err)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !dexNumberOwned(skill.AddressesFor(m).DecodePokedex(&mem).Owned, wantDex) {
		return trained, fmt.Errorf("agent: %s: reached level %d but expected evolution Pokédex #%d is not owned", o, o.Level, wantDex)
	}
	return trained, nil
}

func executeDexEvolutionItem(m *emu.Emu, romData []byte, o Objective, result ObjectiveResult) (ObjectiveResult, error) {
	adapter := newRedObjectiveAdapter(m, romData)
	item, ok := adapter.resolveItemID(o.Item)
	if !ok {
		return result, fmt.Errorf("agent: %s: unknown Red item %q", o, o.Item)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	party := skill.AddressesFor(m).DecodeParty(&mem)
	if o.Slot < 0 || o.Slot >= len(party.Mons) {
		return result, fmt.Errorf("agent: %s: party slot %d out of range for party of %d", o, o.Slot, len(party.Mons))
	}
	from := party.Mons[o.Slot].Species
	to, ok := itemEvolutionTarget(romData, from, item)
	if !ok {
		return result, fmt.Errorf("agent: %s: item %q does not evolve species %#02x", o, o.Item, from)
	}
	if err := skill.UseEvolutionItem(m, romData, item, o.Slot, to); err != nil {
		return result, fmt.Errorf("agent: %s: %w", o, err)
	}
	return result, nil
}

func levelEvolutionTarget(romData []byte, from uint8) (uint8, bool) {
	evos, err := rom.Evolutions(romData)
	if err != nil {
		return 0, false
	}
	for _, evo := range evos {
		if evo.Method == rom.EvoLevel && evo.From == from {
			return evo.To, true
		}
	}
	return 0, false
}

func itemEvolutionTarget(romData []byte, from, item uint8) (uint8, bool) {
	evos, err := rom.Evolutions(romData)
	if err != nil {
		return 0, false
	}
	for _, evo := range evos {
		if evo.Method == rom.EvoItem && evo.From == from && evo.Item == item {
			return evo.To, true
		}
	}
	return 0, false
}

func dexNumberOwned(owned []uint8, want uint8) bool {
	for _, dex := range owned {
		if dex == want {
			return true
		}
	}
	return false
}
