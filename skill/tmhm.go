package skill

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TMHMDecision is a deterministic plan for teaching one owned machine.
// ReplaceSlot is -1 when an empty move slot is available. Existing is true
// when the selected party member already knows the machine move, making the
// operation an idempotent no-op.
type TMHMDecision struct {
	Machine     rom.Machine
	PartySlot   int
	ReplaceSlot int
	Existing    bool
	BeforeScore int
	AfterScore  int
	Reason      string
}

// TMHMResult is the positively verified result of TeachTMHM.
type TMHMResult struct {
	Decision TMHMDecision
	Consumed bool // true only when a consumable TM quantity fell by one
}

// DecideTMHM chooses the best compatible recipient and, for a full move set,
// the best replaceable slot. When required is false, a machine is recommended
// only when it improves the whole move-set score materially. Required is for
// progression capabilities such as Cut/Surf/Strength: compatibility and HM
// permanence still apply, but a field requirement may justify a score loss.
func DecideTMHM(romData []byte, party state.PartyState, item uint8, required bool) (TMHMDecision, error) {
	machine, err := rom.LookupTMHM(romData, item)
	if err != nil {
		return TMHMDecision{}, err
	}

	best := TMHMDecision{Machine: machine, PartySlot: -1, ReplaceSlot: -1}
	bestGain := -1 << 30
	compatible := 0
	for slot, mon := range party.Mons {
		canLearn, err := rom.CanLearnTMHM(romData, mon.Species, item)
		if err != nil {
			return TMHMDecision{}, fmt.Errorf("skill: DecideTMHM: party slot %d species %#02x: %w", slot, mon.Species, err)
		}
		if !canLearn {
			continue
		}
		compatible++

		for _, known := range mon.Moves {
			if known == machine.Move {
				return TMHMDecision{
					Machine: machine, PartySlot: slot, ReplaceSlot: -1, Existing: true,
					BeforeScore: moveSetScore(romData, mon.Type1, mon.Type2, mon.Moves),
					AfterScore:  moveSetScore(romData, mon.Type1, mon.Type2, mon.Moves),
					Reason:      fmt.Sprintf("party slot %d already knows move %d", slot, machine.Move),
				}, nil
			}
		}

		before := moveSetScore(romData, mon.Type1, mon.Type2, mon.Moves)
		candidateSlot := -2 // -1 means empty slot; -2 means no legal placement
		candidateAfter := -1 << 30
		for moveSlot, known := range mon.Moves {
			if known != 0 {
				continue
			}
			next := mon.Moves
			next[moveSlot] = machine.Move
			candidateSlot = -1
			candidateAfter = moveSetScore(romData, mon.Type1, mon.Type2, next)
			break
		}
		if candidateSlot == -2 {
			for moveSlot, known := range mon.Moves {
				isHM, err := rom.IsHMMove(romData, known)
				if err != nil {
					return TMHMDecision{}, fmt.Errorf("skill: DecideTMHM: inspect move %d in party slot %d: %w", known, slot, err)
				}
				if isHM {
					continue // Red refuses to forget HMs; never propose an impossible replacement.
				}
				next := mon.Moves
				next[moveSlot] = machine.Move
				after := moveSetScore(romData, mon.Type1, mon.Type2, next)
				if candidateSlot == -2 || after > candidateAfter {
					candidateSlot, candidateAfter = moveSlot, after
				}
			}
		}
		if candidateSlot == -2 {
			continue
		}

		gain := candidateAfter - before
		if !required && gain < minMoveLearningImprovement {
			continue
		}
		if best.PartySlot < 0 || gain > bestGain {
			bestGain = gain
			best = TMHMDecision{
				Machine: machine, PartySlot: slot, ReplaceSlot: candidateSlot,
				BeforeScore: before, AfterScore: candidateAfter,
			}
		}
	}

	if best.PartySlot < 0 {
		if compatible == 0 {
			return TMHMDecision{}, fmt.Errorf("skill: DecideTMHM: no party member is compatible with item %#02x move %d", item, machine.Move)
		}
		if required {
			return TMHMDecision{}, fmt.Errorf("skill: DecideTMHM: compatible party has no replaceable move slot for required item %#02x move %d", item, machine.Move)
		}
		return TMHMDecision{}, fmt.Errorf("skill: DecideTMHM: item %#02x move %d offers no material move-set improvement", item, machine.Move)
	}

	placement := fmt.Sprintf("replacing move slot %d", best.ReplaceSlot)
	if best.ReplaceSlot < 0 {
		placement = "using an empty move slot"
	}
	why := "material move-set improvement"
	if required {
		why = "required progression capability"
	}
	best.Reason = fmt.Sprintf("%s: party slot %d score %d->%d, %s", why, best.PartySlot, best.BeforeScore, best.AfterScore, placement)
	return best, nil
}

// TeachTMHM teaches one machine through Red's real item and party menus. It
// first computes a ROM-derived compatible recipient and replacement decision,
// then verifies the resulting move directly from party RAM. For TMs it also
// verifies that exactly one copy was consumed; for HMs it verifies the item
// remains in the bag.
func TeachTMHM(m *emu.Emu, item uint8, required bool) (TMHMResult, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	decision, err := DecideTMHM(m.ROM(), party, item, required)
	if err != nil {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: %w", err)
	}
	result := TMHMResult{Decision: decision}
	if decision.Existing {
		return result, nil
	}
	if !state.Controllable(&mem) {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: player is not controllable")
	}
	_, beforeQty := bagEntry(&mem, item)
	if beforeQty == 0 {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: item %#02x is not in the bag", item)
	}

	wantMax, itemIndex := startMenuShape(&mem)
	if err := openStartMenuEntry(m, itemIndex, wantMax); err != nil {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: open ITEM: %w", err)
	}
	if _, err := m.StepUntil(bagMenuBudget, func(m *emu.Emu) bool { return m.Peek8(sym.ListMenuID) == itemListMenuID }); err != nil {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: bag did not open")
	}
	state.Snapshot(m, &mem)
	idx, _ := bagEntry(&mem, item)
	if idx < 0 {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: item %#02x disappeared from the bag", item)
	}
	if err := selectBagEntry(m, idx); err != nil {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: select item %#02x: %w", item, err)
	}
	if _, err := m.StepUntil(useTossBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return useTossPrompt(&mem) != nil
	}); err != nil {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: USE/TOSS prompt did not appear")
	}
	if p := useTossPrompt(&mem); p == nil || p.Index != 0 {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: USE/TOSS cursor is not on USE")
	}
	m.Tap(emu.A, 3, 7)

	for i := 0; i < 100; i++ {
		state.Snapshot(m, &mem)
		if strings.Contains(state.ScreenText(&mem), "Teach") && state.DecodeTwoOptionMenu(&mem) != nil {
			break
		}
		m.Tap(emu.A, 3, 7)
		m.StepFrames(20)
	}
	state.Snapshot(m, &mem)
	if !(strings.Contains(state.ScreenText(&mem), "Teach") && state.DecodeTwoOptionMenu(&mem) != nil) {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: teach prompt did not appear: %q", state.ScreenText(&mem))
	}
	if err := SelectMenuItem(m, 0); err != nil {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: answer teach prompt: %w", err)
	}
	if _, err := m.StepUntil(1000, tmhmPartyMenuUp); err != nil {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: TM/HM party menu did not appear")
	}
	if err := selectTMHMPartySlot(m, decision.PartySlot); err != nil {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: select party slot %d: %w", decision.PartySlot, err)
	}
	if err := finishTeachingTMHM(m, decision); err != nil {
		return TMHMResult{}, err
	}
	if err := closeToOverworld(m); err != nil {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: close menus: %w", err)
	}

	state.Snapshot(m, &mem)
	afterParty := state.DecodeParty(&mem)
	if decision.PartySlot >= len(afterParty.Mons) || !monKnowsMove(afterParty.Mons[decision.PartySlot], decision.Machine.Move) {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: move %d was not verified in party slot %d", decision.Machine.Move, decision.PartySlot)
	}
	_, afterQty := bagEntry(&mem, item)
	if decision.Machine.Consumable() {
		if afterQty != beforeQty-1 {
			return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: TM %#02x quantity %d->%d, want exactly one consumed", item, beforeQty, afterQty)
		}
		result.Consumed = true
	} else if afterQty != beforeQty {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHM: HM %#02x quantity changed %d->%d", item, beforeQty, afterQty)
	}
	return result, nil
}

func finishTeachingTMHM(m *emu.Emu, decision TMHMDecision) error {
	var mem state.Mem
	for frames := 0; frames < cutMenuBudget; frames += 20 {
		state.Snapshot(m, &mem)
		party := state.DecodeParty(&mem)
		if decision.PartySlot < len(party.Mons) && monKnowsMove(party.Mons[decision.PartySlot], decision.Machine.Move) {
			return nil
		}
		text := state.ScreenText(&mem)
		switch {
		case strings.Contains(text, "not compatible"):
			return fmt.Errorf("skill: TeachTMHM: ROM compatibility said party slot %d can learn move %d, but the game rejected it: %q", decision.PartySlot, decision.Machine.Move, text)
		case forgetMenuUp(m):
			if decision.ReplaceSlot < 0 {
				return fmt.Errorf("skill: TeachTMHM: game requested a replacement for move %d despite an empty slot in the pre-teach state", decision.Machine.Move)
			}
			if err := selectForgetSlot(m, decision.ReplaceSlot); err != nil {
				return fmt.Errorf("skill: TeachTMHM: choose move slot %d to forget: %w", decision.ReplaceSlot, err)
			}
		case strings.Contains(text, "trying to learn"):
			if state.DecodeTwoOptionMenu(&mem) != nil {
				if err := SelectMenuItem(m, 0); err != nil {
					return fmt.Errorf("skill: TeachTMHM: answer replace-move prompt: %w", err)
				}
			} else {
				m.Tap(emu.A, 3, 7)
			}
		case tmhmPartyMenuUp(m):
			return fmt.Errorf("skill: TeachTMHM: teaching move %d returned to party menu without changing party slot %d", decision.Machine.Move, decision.PartySlot)
		default:
			m.Tap(emu.A, 3, 7)
		}
		m.StepFrames(20)
	}
	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: TeachTMHM: timed out teaching move %d to party slot %d: %q", decision.Machine.Move, decision.PartySlot, state.ScreenText(&mem))
}
