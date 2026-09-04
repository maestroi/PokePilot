package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// healMenuBudget bounds the wait from the first A press on the nurse to the
// YES/NO prompt: the welcome box, and on a first-ever center visit the
// "Shall we heal your Pokemon?" box, each needing an A press to close, plus
// the frames each box takes to appear. advanceUntil counts iterations, not
// frames (a box-up iteration is a Tap plus talkSettle), so this is the same
// scale as choiceWaitBudget, which covers the same kind of wait in the
// starter flow.
const healMenuBudget = 3000

// healRunBudget bounds the YES path after the prompt is answered: the "I
// need your Pokemon" box, the nurse turning to the healing machine,
// HealParty, the machine animation, the "fighting fit" box, the bow, and
// the farewell box, all before the player is controllable again. Same scale
// as cutsceneBudget.
const healRunBudget = 30000

// allPartyCenterRecovered reports the state a Center visit guarantees that
// matters to the autonomous runner: every party member is at full HP, clear
// of status, and every known move has non-zero current PP. HealParty restores
// HP, clears status, and refills PP from the ROM move table (including PP Up
// bonus PP). We intentionally do not try to reconstruct each move's exact max
// PP here: detecting/recovering an exhausted move is the run-lifecycle problem
// this predicate must make impossible to report as a successful no-op.
func allPartyCenterRecovered(mem *state.Mem) bool {
	party := state.DecodeParty(mem)
	if party.Count == 0 {
		return false
	}
	for _, mon := range party.Mons {
		if mon.HP != mon.MaxHP || mon.Status != 0 {
			return false
		}
		for i, move := range mon.Moves {
			if move != 0 && mon.PP[i] == 0 {
				return false
			}
		}
	}
	return true
}

// counterDirection returns the step toward the center's counter: the unique
// in-bounds non-walkable neighbor of the player's tile in the ROM's collision
// grid. The nurse stands behind the counter, two tiles from the player, which
// is beyond Face's one-tile reach; the game extends the talk range over a
// counter tile in front of the player (pokered/home/overworld.asm,
// IsSpriteOrSignInFrontOfPlayer), so facing the counter is what makes the nurse
// talkable.
func counterDirection(m *emu.Emu) (world.Step, error) {
	romData := m.ROM()
	cur := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return world.Step{}, fmt.Errorf("skill: Heal: parse map %#04x: %w", cur, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return world.Step{}, fmt.Errorf("skill: Heal: build map %#04x: %w", cur, err)
	}
	x, y := playerXY(m)
	var solid []world.Step
	for _, s := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		nx, ny := int(x)+s.DX, int(y)+s.DY
		if !grid.InBounds(nx, ny) {
			continue
		}
		if !grid.Walkable(nx, ny) {
			solid = append(solid, s)
		}
	}
	if len(solid) != 1 {
		return world.Step{}, fmt.Errorf("skill: Heal: map %#04x at (%d,%d): expected exactly one non-walkable neighbor (the counter), found %d",
			cur, x, y, len(solid))
	}
	return solid[0], nil
}

// openNurseMenu presses A on the nurse and advances each text box until the
// YES/NO prompt is up. It returns ErrNoDialogue (wrapped) when A opens no box
// at all, and a descriptive error when the boxes advance but the prompt never
// appears. The wStatusFlags4 (bit 2 = BIT_USED_POKECENTER) in the diagnostics
// distinguishes the first-visit and repeat-visit flows.
func openNurseMenu(m *emu.Emu) error {
	m.Tap(emu.A, 3, 7)
	var mem state.Mem
	if _, err := m.StepUntil(talkOpenBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0
	}); err != nil {
		state.Snapshot(m, &mem)
		return fmt.Errorf("skill: Heal: %w: map=%#04x at (%d,%d) wJoyIgnore=%#04x",
			ErrNoDialogue, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U16BE(sym.JoyIgnore))
	}
	mem = advanceUntil(m, healMenuBudget, func(mem *state.Mem) bool {
		return state.DecodeTwoOptionMenu(mem) != nil
	})
	if state.DecodeTwoOptionMenu(&mem) == nil {
		return fmt.Errorf("skill: Heal: yes/no prompt did not appear within %d iterations: map=%#04x at (%d,%d) wFontLoaded=%#04x wJoyIgnore=%#04x wStatusFlags4=%#04x menu=%+v",
			healMenuBudget, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U16BE(sym.FontLoaded), mem.U16BE(sym.JoyIgnore), mem.U16BE(sym.StatusFlags4),
			state.DecodeMenu(&mem))
	}
	return nil
}

// Heal restores the party at a Pokemon Center's nurse. It requires the
// player to stand on the counter approach tile — the floor tile directly
// adjacent to the counter the nurse stands behind; for the Viridian Center
// that is Place("viridian pokemon center"). It faces the counter, opens the
// nurse's dialogue, answers the prompt YES, and lets the heal run to
// completion.
//
// The flow follows the decomp (pokered/engine/events/pokecenter.asm,
// DisplayPokemonCenterDialogue_): the welcome box; on a first-ever center
// visit one more box; then the yes/no prompt (pokered/home/yes_no.asm,
// YesNoChoicePokeCenter) — an ordinary two-item menu with index 0 = YES and
// 1 = NO, the same shape SelectMenuItem drives in the starter flow. Choosing
// YES runs HealParty (pokered/engine/events/heal_party.asm), which restores HP
// and PP and clears status, behind the healing-machine animation and a few
// more boxes, before control returns to the player.
//
// The nurse is not addressed by coordinates: Face reaches only one tile, and
// the counter between the player and the nurse is found as the unique
// non-walkable neighbor of the player's tile in the ROM's collision grid.
//
// Success requires the Center recovery postcondition plus controllability.
// This deliberately includes exhausted PP: a healthy party with a 0-PP move
// must not let a PP-recovery objective succeed without actually using the
// nurse.
func Heal(m *emu.Emu) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: Heal: player not controllable: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U16BE(sym.JoyIgnore), mem.U16BE(sym.FontLoaded))
	}
	if state.DecodeParty(&mem).Count == 0 {
		return fmt.Errorf("skill: Heal: no party to heal: map=%#04x at (%d,%d)",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}

	step, err := counterDirection(m)
	if err != nil {
		return err
	}
	x, y := playerXY(m)
	if err := Face(m, uint8(int(x)+step.DX), uint8(int(y)+step.DY)); err != nil {
		return fmt.Errorf("skill: Heal: face the counter %s from (%d,%d): %w", step, x, y, err)
	}

	if err := openNurseMenu(m); err != nil {
		return err
	}

	// YES is index 0. The cursor starts at 0, so SelectMenuItem presses A once
	// it has asserted the index; the decomp branches on the index
	// (pokecenter.asm: 0 continues to HealParty, 1 declines).
	if err := SelectMenuItem(m, 0); err != nil {
		state.Snapshot(m, &mem)
		return fmt.Errorf("skill: Heal: select YES: %w: map=%#04x at (%d,%d) wFontLoaded=%#04x menu=%+v",
			err, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U16BE(sym.FontLoaded), state.DecodeMenu(&mem))
	}

	if err := Cutscene(m, healRunBudget, allPartyCenterRecovered); err != nil {
		return fmt.Errorf("skill: Heal: %w", err)
	}

	// Cutscene's return already means the positive recovery predicate and
	// Controllable both hold, but re-asserting them keeps Heal's contract
	// explicit to its callers.
	state.Snapshot(m, &mem)
	if !allPartyCenterRecovered(&mem) {
		return fmt.Errorf("skill: Heal: party not fully recovered after the heal: %+v (map=%#04x at (%d,%d) wJoyIgnore=%#04x)",
			state.DecodeParty(&mem).Mons, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U16BE(sym.JoyIgnore))
	}
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: Heal: not controllable after the heal: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U16BE(sym.JoyIgnore), mem.U16BE(sym.FontLoaded))
	}
	return nil
}
