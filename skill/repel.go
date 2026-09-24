package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	ItemRepel      uint8 = 0x1E
	ItemSuperRepel uint8 = 0x38
	ItemMaxRepel   uint8 = 0x39

	repelUseSettleBudget = 1200
)

// RepelDuration returns the Gen 1 overworld step budget granted by a Repel item.
func RepelDuration(item uint8) (int, bool) {
	switch item {
	case ItemRepel:
		return 100, true
	case ItemSuperRepel:
		return 200, true
	case ItemMaxRepel:
		return 250, true
	default:
		return 0, false
	}
}

// UseRepel activates a Repel-family item from the overworld and proves both
// sides of the effect: the bag stack decreases by one and
// wRepelRemainingSteps is loaded with the item's exact duration.
func UseRepel(m *emu.Emu, item uint8) error {
	duration, ok := RepelDuration(item)
	if !ok {
		return fmt.Errorf("skill: UseRepel: item %#02x is not a Repel-family item", item)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: UseRepel: player not controllable on map %#04x at (%d,%d)",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
	if active := int(mem.U8(sym.RepelRemainingSteps)); active > 0 {
		return fmt.Errorf("skill: UseRepel: repel is already active for %d more steps", active)
	}
	idx, bagBefore := bagEntry(&mem, item)
	if idx < 0 {
		return fmt.Errorf("skill: UseRepel: %w (id %#02x)", ErrNotInBag, item)
	}

	_, itemIndex := startMenuShape(&mem)
	if err := openStartMenuEntry(m, itemIndex); err != nil {
		return fmt.Errorf("skill: UseRepel: open ITEM: %w", err)
	}
	if _, err := m.StepUntil(bagMenuBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.ListMenuID) == itemListMenuID
	}); err != nil {
		return fmt.Errorf("skill: UseRepel: bag list did not open: %w", err)
	}
	if err := selectBagEntry(m, idx); err != nil {
		return fmt.Errorf("skill: UseRepel: select bag entry: %w", err)
	}
	if _, err := m.StepUntil(useTossBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return useTossPrompt(&mem) != nil
	}); err != nil {
		return fmt.Errorf("skill: UseRepel: USE/TOSS prompt did not open: %w", err)
	}
	state.Snapshot(m, &mem)
	if p := useTossPrompt(&mem); p == nil || p.Index != 0 {
		return fmt.Errorf("skill: UseRepel: USE/TOSS cursor is not on USE")
	}

	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(repelUseSettleBudget, func(m *emu.Emu) bool {
		return int(m.Peek8(sym.RepelRemainingSteps)) == duration
	}); err != nil {
		state.Snapshot(m, &mem)
		return fmt.Errorf("skill: UseRepel: effect did not load %d steps: now=%d screen=%q",
			duration, mem.U8(sym.RepelRemainingSteps), state.ScreenText(&mem))
	}
	// The counter is written immediately before the ROM enters the generic
	// item-success path. Let the owned USE/TOSS surface disappear before
	// treating any later two-option prompt as unexpected gameplay.
	_, _ = m.StepUntil(useTossBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return useTossPrompt(&mem) == nil
	})

	// Repel's success message is dialogue, followed by the bag/start-menu
	// stack. This verb owns that known result text, so page it with B while
	// still refusing any unexpected two-option gameplay choice.
	start := m.FrameCount()
	for {
		state.Snapshot(m, &mem)
		interaction := state.DecodeInteraction(&mem)
		if state.Controllable(&mem) && interaction.Kind == state.InteractionNone {
			break
		}
		if state.DecodeBattle(&mem) != nil {
			return fmt.Errorf("skill: UseRepel: unexpected battle while closing item UI")
		}
		if interaction.Kind == state.InteractionTwoOption {
			return fmt.Errorf("skill: UseRepel: unexpected choice while closing item UI: %q", interaction.Text)
		}
		if int(m.FrameCount()-start) > fieldResultTextBudget {
			return fmt.Errorf("skill: UseRepel: item UI did not close: interaction=%s screen=%q", interaction.Kind, state.ScreenText(&mem))
		}
		m.Tap(emu.B, 3, 7)
	}

	state.Snapshot(m, &mem)
	if got := int(mem.U8(sym.RepelRemainingSteps)); got != duration {
		return fmt.Errorf("skill: UseRepel: active steps changed unexpectedly: got %d want %d", got, duration)
	}
	if _, bagAfter := bagEntry(&mem, item); bagAfter != bagBefore-1 {
		return fmt.Errorf("skill: UseRepel: bag count for %#02x did not drop from %d (now %d)", item, bagBefore, bagAfter)
	}
	return nil
}

// UseBestRepel activates the longest-duration Repel stack currently available.
// It is a no-op while an effect is already active or when the bag has no
// Repel-family item. The caller owns the policy decision to use encounter
// suppression; this helper only performs the deterministic bag choice.
func UseBestRepel(m *emu.Emu) (bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if mem.U8(sym.RepelRemainingSteps) > 0 {
		return false, nil
	}
	for _, item := range []uint8{ItemMaxRepel, ItemSuperRepel, ItemRepel} {
		if _, quantity := bagEntry(&mem, item); quantity > 0 {
			if err := UseRepel(m, item); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}
