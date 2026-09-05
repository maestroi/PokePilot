package skill

import (
	"fmt"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	hm01Item       uint8 = 0xC4
	cutMove        uint8 = 0x0F
	cutFieldMove   uint8 = 1
	cutTreeTile    uint8 = 0x3D
	gymCutTreeTile uint8 = 0x50
	vermilionCity  uint8 = 0x05
	vermilionGymX        = 12
	vermilionGymY        = 19
	cutMenuBudget        = 4000
)

func monKnowsMove(mon state.Mon, move uint8) bool {
	for _, id := range mon.Moves {
		if id == move {
			return true
		}
	}
	return false
}

func partyMoveSlot(mem *state.Mem, move uint8) int {
	for i, mon := range state.DecodeParty(mem).Mons {
		if monKnowsMove(mon, move) {
			return i
		}
	}
	return -1
}

func cutScreenHas(m *emu.Emu, marker string) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return strings.Contains(state.ScreenText(&mem), marker)
}

func tmhmPartyMenuUp(m *emu.Emu) bool   { return cutScreenHas(m, "Use TM") }
func normalPartyMenuUp(m *emu.Emu) bool { return cutScreenHas(m, "Choose") }
func fieldMoveMenuUp(m *emu.Emu) bool {
	return cutScreenHas(m, "STATS") && m.Peek8(sym.FieldMoves) != 0
}

func movePartyCursor(m *emu.Emu, index int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	count := int(state.DecodeParty(&mem).Count)
	if index < 0 || index >= count {
		return fmt.Errorf("skill: party slot %d out of range for party of %d", index, count)
	}
	for i := 0; i < 60; i++ {
		cur := int(m.Peek8(sym.CurrentMenuItem))
		if cur == index {
			return nil
		}
		btn := emu.Down
		if cur > index {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		_, _ = m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return int(m.Peek8(sym.CurrentMenuItem)) != cur
		})
	}
	return fmt.Errorf("skill: party cursor at %d, want %d", m.Peek8(sym.CurrentMenuItem), index)
}

func selectTMHMPartySlot(m *emu.Emu, index int) error {
	if err := movePartyCursor(m, index); err != nil {
		return err
	}
	for i := 0; i < 24 && tmhmPartyMenuUp(m); i++ {
		m.Tap(emu.A, 3, 7)
		_, _ = m.StepUntil(25, func(m *emu.Emu) bool { return !tmhmPartyMenuUp(m) })
	}
	if tmhmPartyMenuUp(m) {
		return fmt.Errorf("skill: TM/HM party menu still up after selecting slot %d", index)
	}
	return nil
}

func selectFieldMoveUser(m *emu.Emu, index int) error {
	if err := movePartyCursor(m, index); err != nil {
		return err
	}
	for i := 0; i < 24 && !fieldMoveMenuUp(m); i++ {
		m.Tap(emu.A, 3, 7)
		_, _ = m.StepUntil(25, fieldMoveMenuUp)
	}
	if !fieldMoveMenuUp(m) {
		return fmt.Errorf("skill: field-move menu did not appear after selecting slot %d", index)
	}
	return nil
}

func openStartMenuEntry(m *emu.Emu, entry, wantMax int) error {
	drawn := func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0 && int(m.Peek8(sym.MaxMenuItem)) == wantMax
	}
	for attempt := 0; attempt < 5 && !drawn(m); attempt++ {
		m.Tap(emu.Start, 3, 7)
		_, _ = m.StepUntil(startMenuDrawBudget, drawn)
	}
	if !drawn(m) {
		return fmt.Errorf("skill: start menu did not finish drawing")
	}
	return SelectMenuItem(m, entry)
}

func closeToOverworld(m *emu.Emu) error {
	var mem state.Mem
	for i := 0; i < 80; i++ {
		state.Snapshot(m, &mem)
		if state.Controllable(&mem) && mem.U8(sym.FontLoaded) == 0 {
			return nil
		}
		m.Tap(emu.B, 3, 7)
		m.StepFrames(20)
	}
	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: menus did not close to overworld: screen=%q", state.ScreenText(&mem))
}

// TeachCut is the compatibility entry point for existing story code. The
// shared field-action framework owns prerequisite checks and generic HM
// teaching now, so Cut cannot drift from Surf/Strength/Flash semantics.
func TeachCut(m *emu.Emu) (int, error) {
	return EnsureFieldMove(m, FieldCut)
}

// finishTeachingCut is retained for older measurement/tests that exercise the
// historical menu path directly. Production teaching goes through TeachTMHM
// via EnsureFieldMove.
func finishTeachingCut(m *emu.Emu, slot int, before [4]uint8) (bool, error) {
	tried := map[uint8]bool{}
	lastForget := -1
	var mem state.Mem
	for frames := 0; frames < cutMenuBudget; frames += 20 {
		state.Snapshot(m, &mem)
		if partyMoveSlot(&mem, cutMove) == slot {
			return true, nil
		}
		text := state.ScreenText(&mem)
		switch {
		case strings.Contains(text, "not compatible"):
			for i := 0; i < 30 && !tmhmPartyMenuUp(m); i++ {
				m.Tap(emu.A, 3, 7)
				m.StepFrames(20)
			}
			return false, nil
		case forgetMenuUp(m):
			if lastForget >= 0 {
				tried[before[lastForget]] = true
				lastForget = -1
			}
			pick := forgetSlot(m.ROM(), before, tried)
			if pick < 0 {
				return false, fmt.Errorf("skill: TeachCut: slot %d has no move that can be replaced", slot)
			}
			if err := selectForgetSlot(m, pick); err != nil {
				return false, fmt.Errorf("skill: TeachCut: choose move to forget: %w", err)
			}
			lastForget = pick
		case strings.Contains(text, "trying to learn"):
			if state.DecodeTwoOptionMenu(&mem) != nil {
				if err := SelectMenuItem(m, 0); err != nil {
					return false, fmt.Errorf("skill: TeachCut: answer replace-move prompt: %w", err)
				}
			} else {
				m.Tap(emu.A, 3, 7)
			}
		case tmhmPartyMenuUp(m):
			return false, nil
		default:
			m.Tap(emu.A, 3, 7)
		}
		m.StepFrames(20)
	}
	return false, fmt.Errorf("skill: TeachCut: slot %d did not settle within %d frames", slot, cutMenuBudget)
}

func cuttableFrontTile(tile uint8) bool { return tile == cutTreeTile || tile == gymCutTreeTile }

// CutAhead is retained as the public compatibility verb, but execution now
// goes through UseFieldMove so Travel, Vermilion Gym, and direct callers all
// share the same badge/learned-move/context/completion rules.
func CutAhead(m *emu.Emu) error {
	_, err := UseFieldMove(m, FieldCut)
	return err
}

type cutCandidate struct{ x, y, d int }

func vermilionCutCandidates(grid *world.Grid) []cutCandidate {
	var out []cutCandidate
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if grid.Walkable(x, y) {
				continue
			}
			d := absInt(x-vermilionGymX) + absInt(y-vermilionGymY)
			if d <= 12 {
				out = append(out, cutCandidate{x: x, y: y, d: d})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].d != out[j].d {
			return out[i].d < out[j].d
		}
		if out[i].y != out[j].y {
			return out[i].y < out[j].y
		}
		return out[i].x < out[j].x
	})
	return out
}

func reachableBeside(grid *world.Grid, sx, sy, tx, ty int, blocked map[[2]int]bool) (Destination, bool) {
	bestLen := int(^uint(0) >> 1)
	var best Destination
	found := false
	for _, s := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		x, y := tx+s.DX, ty+s.DY
		if !grid.InBounds(x, y) || !grid.Walkable(x, y) {
			continue
		}
		steps, err := world.FindPath(grid, sx, sy, x, y, blocked)
		if err == nil && len(steps) < bestLen {
			bestLen, found = len(steps), true
			best = Destination{Map: vermilionCity, X: uint8(x), Y: uint8(y)}
		}
	}
	return best, found
}

func EnterVermilionGym(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != vermilionCity {
		return fmt.Errorf("skill: EnterVermilionGym: on map %#04x, want %#04x", m.Peek8(sym.CurMap), vermilionCity)
	}
	if policy == nil {
		return fmt.Errorf("skill: EnterVermilionGym: nil policy")
	}
	if _, err := TeachCut(m); err != nil {
		return fmt.Errorf("skill: EnterVermilionGym: %w", err)
	}

	h, err := rom.ParseMap(romData, vermilionCity)
	if err != nil {
		return fmt.Errorf("skill: EnterVermilionGym: parse city: %w", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return fmt.Errorf("skill: EnterVermilionGym: build city: %w", err)
	}

	tree, err := findVermilionGymTree(m, romData, grid, policy)
	if err != nil {
		return err
	}
	if err := CutAhead(m); err != nil {
		return fmt.Errorf("skill: EnterVermilionGym: cut tree at (%d,%d): %w", tree.x, tree.y, err)
	}
	grid.Set(tree.x, tree.y, true)
	return crossVermilionGymDoor(m, grid)
}

func findVermilionGymTree(m *emu.Emu, romData []byte, grid *world.Grid, policy MovePolicy) (cutCandidate, error) {
	for _, c := range vermilionCutCandidates(grid) {
		sx, sy := playerXY(m)
		stand, ok := reachableBeside(grid, int(sx), int(sy), c.x, c.y, spriteBlockers(m))
		if !ok {
			continue
		}
		if _, err := Travel(m, romData, stand, policy, 10); err != nil {
			continue
		}
		if err := Face(m, uint8(c.x), uint8(c.y)); err != nil {
			continue
		}
		m.StepFrames(2)
		if m.Peek8(sym.TileInFrontOfPlayer) == cutTreeTile {
			return c, nil
		}
	}
	return cutCandidate{}, fmt.Errorf("skill: EnterVermilionGym: no reachable Cut tree found near gym warp (%d,%d)", vermilionGymX, vermilionGymY)
}

func crossVermilionGymDoor(m *emu.Emu, grid *world.Grid) error {
	sx, sy := playerXY(m)
	var best []world.Step
	var push world.Step
	for _, s := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		x, y := vermilionGymX+s.DX, vermilionGymY+s.DY
		if !grid.InBounds(x, y) || !grid.Walkable(x, y) {
			continue
		}
		p, err := world.FindPath(grid, int(sx), int(sy), x, y, spriteBlockers(m))
		if err == nil && (best == nil || len(p) < len(best)) {
			best, push = p, world.Step{DX: -s.DX, DY: -s.DY}
		}
	}
	if best == nil {
		return fmt.Errorf("skill: EnterVermilionGym: no path through cut tree to gym door")
	}
	if err := WalkPath(m, best); err != nil {
		return fmt.Errorf("skill: EnterVermilionGym: walk through cut tree: %w", err)
	}
	btn, ok := buttonFor(push)
	if !ok {
		return fmt.Errorf("skill: EnterVermilionGym: invalid door push %s", push)
	}
	m.Press(btn)
	crossed := false
	for i := 0; i < crossBudget; i++ {
		if m.Peek8(sym.CurMap) != vermilionCity {
			crossed = true
			break
		}
		m.StepFrame()
	}
	m.Release(btn)
	if !crossed || m.Peek8(sym.CurMap) != vermilionGymMap {
		x, y := playerXY(m)
		return fmt.Errorf("skill: EnterVermilionGym: door did not enter gym; map=%#04x at (%d,%d)", m.Peek8(sym.CurMap), x, y)
	}
	if _, err := m.StepUntil(arriveBudget, func(m *emu.Emu) bool {
		var s state.Mem
		state.Snapshot(m, &s)
		return state.Controllable(&s)
	}); err != nil {
		return fmt.Errorf("skill: EnterVermilionGym: gym loaded but player did not become controllable")
	}
	return nil
}
