package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

var ErrEvolutionItemNoEffect = errors.New("skill: evolution item did not produce the expected species")

const evolutionItemBudget = 9000

// UseEvolutionItem uses a stone/item on one party member and deliberately
// avoids the generic field-item result driver. Generic medicine closes text
// with B; B is also the cancel input during Pokemon evolution. This path pages
// forward with A until the expected species and Pokédex-owned bit are both
// visible, then closes only ordinary leftover menus.
func UseEvolutionItem(m *emu.Emu, romData []byte, item uint8, slot int, wantSpecies uint8) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: UseEvolutionItem: player not controllable")
	}
	party := state.DecodeParty(&mem)
	if slot < 0 || slot >= len(party.Mons) {
		return fmt.Errorf("skill: UseEvolutionItem: slot %d out of range for party of %d", slot, len(party.Mons))
	}
	if party.Mons[slot].Species == wantSpecies {
		return nil
	}
	wantDex, err := rom.InternalSpeciesDexNumber(romData, wantSpecies)
	if err != nil {
		return fmt.Errorf("skill: UseEvolutionItem: target species %#02x: %w", wantSpecies, err)
	}
	idx, bagBefore := bagEntry(&mem, item)
	if idx < 0 || bagBefore <= 0 {
		return fmt.Errorf("skill: UseEvolutionItem: %w (id %#02x)", ErrNotInBag, item)
	}

	wantMax, itemIndex := startMenuShape(&mem)
	drawn := func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0 && int(m.Peek8(sym.MaxMenuItem)) == wantMax
	}
	for attempt := 0; attempt < 5; attempt++ {
		if _, stepErr := m.StepUntil(10, drawn); stepErr == nil {
			break
		}
		m.Tap(emu.Start, 3, 7)
		if _, stepErr := m.StepUntil(startMenuDrawBudget, drawn); stepErr == nil {
			break
		}
	}
	if !drawn(m) {
		return fmt.Errorf("skill: UseEvolutionItem: start menu did not finish drawing")
	}
	if err := SelectMenuItem(m, itemIndex); err != nil {
		return fmt.Errorf("skill: UseEvolutionItem: select ITEM: %w", err)
	}
	if _, err := m.StepUntil(bagMenuBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.ListMenuID) == itemListMenuID
	}); err != nil {
		return fmt.Errorf("skill: UseEvolutionItem: bag list did not open: %w", err)
	}
	if err := selectBagEntry(m, idx); err != nil {
		return fmt.Errorf("skill: UseEvolutionItem: select bag entry: %w", err)
	}
	if _, err := m.StepUntil(useTossBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return useTossPrompt(&mem) != nil
	}); err != nil {
		return fmt.Errorf("skill: UseEvolutionItem: USE/TOSS prompt did not appear: %w", err)
	}
	state.Snapshot(m, &mem)
	if p := useTossPrompt(&mem); p == nil || p.Index != 0 {
		return fmt.Errorf("skill: UseEvolutionItem: USE/TOSS cursor is not on USE")
	}
	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(itemUsePartyBudget, func(m *emu.Emu) bool { return useItemPartyMenuUp(m) }); err != nil {
		return fmt.Errorf("skill: UseEvolutionItem: party menu did not appear: %w", err)
	}
	if err := SelectPartySlot(m, slot); err != nil {
		return fmt.Errorf("skill: UseEvolutionItem: select party slot %d: %w", slot, err)
	}

	start := m.FrameCount()
	changed := false
	for int(m.FrameCount()-start) <= evolutionItemBudget {
		state.Snapshot(m, &mem)
		party = state.DecodeParty(&mem)
		changed = slot < len(party.Mons) && party.Mons[slot].Species == wantSpecies
		owned := pokedexContains(state.DecodePokedex(&mem).Owned, wantDex)
		if changed && owned {
			if state.Controllable(&mem) && !state.MenuUp(&mem) && mem.U8(sym.FontLoaded) == 0 {
				break
			}
			if DismissableObjectiveMenu(&mem) {
				if err := CloseOpenMenuToOverworld(m); err != nil {
					return fmt.Errorf("skill: UseEvolutionItem: close post-evolution menu: %w", err)
				}
				break
			}
		}
		if state.DecodeBattle(&mem) != nil {
			return fmt.Errorf("skill: UseEvolutionItem: unexpected battle while evolving")
		}
		if state.DecodeTwoOptionMenu(&mem) != nil {
			return fmt.Errorf("skill: UseEvolutionItem: unexpected choice prompt while evolving; refusing blind input")
		}
		if mem.U8(sym.FontLoaded) != 0 || state.DecodeDialogue(&mem) != nil {
			// A advances evolution/result text but cannot cancel an evolution.
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrames(4)
		}
	}

	state.Snapshot(m, &mem)
	party = state.DecodeParty(&mem)
	if slot >= len(party.Mons) || party.Mons[slot].Species != wantSpecies {
		return fmt.Errorf("%w: wanted species %#02x in slot %d", ErrEvolutionItemNoEffect, wantSpecies, slot)
	}
	if !pokedexContains(state.DecodePokedex(&mem).Owned, wantDex) {
		return fmt.Errorf("%w: species changed but Pokédex #%d owned bit is not set", ErrEvolutionItemNoEffect, wantDex)
	}
	_, bagAfter := bagEntry(&mem, item)
	if bagAfter != bagBefore-1 {
		return fmt.Errorf("skill: UseEvolutionItem: item count changed %d -> %d, want exactly one consumed", bagBefore, bagAfter)
	}
	return nil
}

func pokedexContains(numbers []uint8, want uint8) bool {
	for _, n := range numbers {
		if n == want {
			return true
		}
	}
	return false
}
