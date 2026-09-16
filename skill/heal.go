package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	redworld "github.com/maestroi/pokepilot/red/worldmap"
	"github.com/maestroi/pokepilot/world"
)

const healMenuBudget = 3000
const healRunBudget = 30000

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

func counterDirection(m *emu.Emu) (world.Step, error) {
	romData := m.ROM()
	cur := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return world.Step{}, fmt.Errorf("skill: Heal: parse map %#04x: %w", cur, err)
	}
	grid, err := redworld.Build(romData, h)
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

	if err := SelectMenuItem(m, 0); err != nil {
		state.Snapshot(m, &mem)
		return fmt.Errorf("skill: Heal: select YES: %w: map=%#04x at (%d,%d) wFontLoaded=%#04x menu=%+v",
			err, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U16BE(sym.FontLoaded), state.DecodeMenu(&mem))
	}

	if err := Cutscene(m, healRunBudget, allPartyCenterRecovered); err != nil {
		return fmt.Errorf("skill: Heal: %w", err)
	}

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
