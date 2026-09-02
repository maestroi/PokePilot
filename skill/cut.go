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
	hm01Item        uint8 = 0xC4
	cutMove         uint8 = 0x0F
	cutFieldMove    uint8 = 1
	cutTreeTile     uint8 = 0x3D
	gymCutTreeTile  uint8 = 0x50
	vermilionCity   uint8 = 0x05
	vermilionGymX         = 12
	vermilionGymY         = 19
	cutMenuBudget         = 4000
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

func tmhmPartyMenuUp(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return strings.Contains(state.ScreenText(&mem), "Use TM")
}

func normalPartyMenuUp(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return strings.Contains(state.ScreenText(&mem), "Choose")
}

// selectRawPartySlot drives a party list whose caller owns what happens after
// selection. SelectPartySlot cannot be used for TM/HM teaching because its
// completion predicate predates TMHM_PARTY_MENU and would consider that menu
// already gone before pressing A.
func selectRawPartySlot(m *emu.Emu, index int, menuUp func(*emu.Emu) bool) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	if index < 0 || index >= int(party.Count) {
		return fmt.Errorf("skill: party slot %d out of range for party of %d", index, party.Count)
	}
	for i := 0; i < 60; i++ {
		cur := int(m.Peek8(sym.CurrentMenuItem))
		if cur == index {
			break
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
	if got := int(m.Peek8(sym.CurrentMenuItem)); got != index {
		return fmt.Errorf("skill: party cursor at %d, want %d", got, index)
	}
	for i := 0; i < 24 && menuUp(m); i++ {
		m.Tap(emu.A, 3, 7)
		_, _ = m.StepUntil(25, func(m *emu.Emu) bool { return !menuUp(m) })
	}
	if menuUp(m) {
		return fmt.Errorf("skill: party menu still up after selecting slot %d", index)
	}
	return nil
}

func openStartMenuEntry(m *emu.Emu, entry int, wantMax int) error {
	drawn := func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0 && int(m.Peek8(sym.MaxMenuItem)) == wantMax
	}
	for attempt := 0; attempt < 5; attempt++ {
		if drawn(m) {
			break
		}
		m.Tap(emu.Start, 3, 7)
		if _, err := m.StepUntil(startMenuDrawBudget, drawn); err == nil {
			break
		}
	}
	if !drawn(m) {
		return fmt.Errorf("skill: start menu did not finish drawing")
	}
	if err := SelectMenuItem(m, entry); err != nil {
		return err
	}
	return nil
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

// TeachCut teaches HM01 to the first party member the ROM accepts. The HM
// party menu itself is authoritative about compatibility (CanLearnTM renders
// ABLE/NOT ABLE); rather than duplicating the species compatibility table,
// this tries each slot and treats the game's "not compatible" response as a
// rejection. HMs are not consumed, so rejected slots are safe to probe.
func TeachCut(m *emu.Emu) (int, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if slot := partyMoveSlot(&mem, cutMove); slot >= 0 {
		return slot, nil
	}
	if _, count := bagEntry(&mem, hm01Item); count == 0 {
		return -1, fmt.Errorf("skill: TeachCut: HM01 is not in the bag")
	}
	if !state.DecodeProgress(&mem).Has(state.BadgeCascade) {
		return -1, fmt.Errorf("skill: TeachCut: Cascade Badge is required to use Cut")
	}
	if !state.Controllable(&mem) {
		return -1, fmt.Errorf("skill: TeachCut: player is not controllable")
	}

	wantMax, itemIndex := startMenuShape(&mem)
	if err := openStartMenuEntry(m, itemIndex, wantMax); err != nil {
		return -1, fmt.Errorf("skill: TeachCut: open ITEM: %w", err)
	}
	if _, err := m.StepUntil(bagMenuBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.ListMenuID) == itemListMenuID
	}); err != nil {
		return -1, fmt.Errorf("skill: TeachCut: bag did not open")
	}
	idx, _ := bagEntry(&mem, hm01Item)
	state.Snapshot(m, &mem)
	idx, _ = bagEntry(&mem, hm01Item)
	if idx < 0 {
		return -1, fmt.Errorf("skill: TeachCut: HM01 disappeared from the bag")
	}
	if err := selectBagEntry(m, idx); err != nil {
		return -1, fmt.Errorf("skill: TeachCut: select HM01: %w", err)
	}
	if _, err := m.StepUntil(useTossBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return useTossPrompt(&mem) != nil
	}); err != nil {
		return -1, fmt.Errorf("skill: TeachCut: USE/TOSS prompt did not appear")
	}
	if p := useTossPrompt(&mem); p == nil || p.Index != 0 {
		return -1, fmt.Errorf("skill: TeachCut: USE/TOSS cursor is not on USE")
	}
	m.Tap(emu.A, 3, 7)

	// HM boot text -> contained CUT -> "Teach CUT to a POKEMON?". Advance
	// only until its explicit two-option menu is drawn, then choose YES.
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
		return -1, fmt.Errorf("skill: TeachCut: teach-HM prompt did not appear: %q", state.ScreenText(&mem))
	}
	if err := SelectMenuItem(m, 0); err != nil {
		return -1, fmt.Errorf("skill: TeachCut: answer teach prompt: %w", err)
	}
	if _, err := m.StepUntil(1000, tmhmPartyMenuUp); err != nil {
		return -1, fmt.Errorf("skill: TeachCut: TM/HM party menu did not appear")
	}

	party := state.DecodeParty(&mem)
	for slot := 0; slot < int(party.Count); slot++ {
		state.Snapshot(m, &mem)
		before := state.DecodeParty(&mem).Mons[slot].Moves
		if err := selectRawPartySlot(m, slot, tmhmPartyMenuUp); err != nil {
			return -1, fmt.Errorf("skill: TeachCut: select slot %d: %w", slot, err)
		}

		triedForgets := map[uint8]bool{}
		lastForget := -1
		for frames := 0; frames < cutMenuBudget; frames += 20 {
			state.Snapshot(m, &mem)
			if partyMoveSlot(&mem, cutMove) == slot {
				if err := closeToOverworld(m); err != nil {
					return -1, fmt.Errorf("skill: TeachCut: learned Cut but could not close menus: %w", err)
				}
				return slot, nil
			}
			text := state.ScreenText(&mem)
			switch {
			case strings.Contains(text, "not compatible"):
				for i := 0; i < 30 && !tmhmPartyMenuUp(m); i++ {
					m.Tap(emu.A, 3, 7)
					m.StepFrames(20)
				}
				frames = cutMenuBudget
				continue
			case forgetMenuUp(m):
				if lastForget >= 0 {
					triedForgets[before[lastForget]] = true
					lastForget = -1
				}
				pick := forgetSlot(m.ROM(), before, triedForgets)
				if pick < 0 {
					return -1, fmt.Errorf("skill: TeachCut: slot %d has no move that can be replaced", slot)
				}
				if err := selectForgetSlot(m, pick); err != nil {
					return -1, fmt.Errorf("skill: TeachCut: choose move to forget: %w", err)
				}
				lastForget = pick
			case strings.Contains(text, "trying to learn"):
				if state.DecodeTwoOptionMenu(&mem) != nil {
					if err := SelectMenuItem(m, 0); err != nil {
						return -1, fmt.Errorf("skill: TeachCut: answer replace-move prompt: %w", err)
					}
				} else {
					m.Tap(emu.A, 3, 7)
				}
			case tmhmPartyMenuUp(m):
				frames = cutMenuBudget
				continue
			default:
				m.Tap(emu.A, 3, 7)
			}
			m.StepFrames(20)
		}
		if !tmhmPartyMenuUp(m) {
			if _, err := m.StepUntil(500, tmhmPartyMenuUp); err != nil {
				state.Snapshot(m, &mem)
				return -1, fmt.Errorf("skill: TeachCut: slot %d rejected but party menu did not return: %q", slot, state.ScreenText(&mem))
			}
		}
	}
	_ = closeToOverworld(m)
	return -1, fmt.Errorf("skill: TeachCut: no party member can learn Cut")
}

func cuttableFrontTile(tile uint8) bool {
	return tile == cutTreeTile || tile == gymCutTreeTile
}

// CutAhead uses the party member that knows Cut, teaching HM01 first when
// necessary. Success is the game's own action result plus a return to the
// overworld; the caller never assumes that selecting CUT removed anything.
func CutAhead(m *emu.Emu) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.DecodeProgress(&mem).Has(state.BadgeCascade) {
		return fmt.Errorf("skill: CutAhead: Cascade Badge is required")
	}
	if tile := mem.U8(sym.TileInFrontOfPlayer); !cuttableFrontTile(tile) {
		return fmt.Errorf("skill: CutAhead: tile in front is %#02x, not a cut tree", tile)
	}
	slot := partyMoveSlot(&mem, cutMove)
	if slot < 0 {
		var err error
		slot, err = TeachCut(m)
		if err != nil {
			return fmt.Errorf("skill: CutAhead: %w", err)
		}
		state.Snapshot(m, &mem)
	}

	wantMax, itemIndex := startMenuShape(&mem)
	pokemonIndex := itemIndex - 1
	if err := openStartMenuEntry(m, pokemonIndex, wantMax); err != nil {
		return fmt.Errorf("skill: CutAhead: open POKEMON: %w", err)
	}
	if _, err := m.StepUntil(1000, normalPartyMenuUp); err != nil {
		return fmt.Errorf("skill: CutAhead: party menu did not appear")
	}
	if err := selectRawPartySlot(m, slot, normalPartyMenuUp); err != nil {
		return fmt.Errorf("skill: CutAhead: select Cut user: %w", err)
	}

	// The field move menu stores name indices in wFieldMoves. CUT is index 1
	// in FieldMoveDisplayData; field moves occupy the menu entries before
	// STATS/SWITCH/CANCEL in the same order.
	cutIndex := -1
	for i := 0; i < 4; i++ {
		if m.Peek8(sym.FieldMoves+uint16(i)) == cutFieldMove {
			cutIndex = i
			break
		}
	}
	if cutIndex < 0 {
		return fmt.Errorf("skill: CutAhead: selected party slot %d knows Cut but CUT is absent from wFieldMoves", slot)
	}
	if err := SelectMenuItem(m, cutIndex); err != nil {
		return fmt.Errorf("skill: CutAhead: select CUT: %w", err)
	}

	m.StepFrames(30) // let UsedCut clear and then set ActionResult itself
	if _, err := m.StepUntil(3000, func(m *emu.Emu) bool {
		var s state.Mem
		state.Snapshot(m, &s)
		return s.U8(sym.ActionResult) == 1 && state.Controllable(&s)
	}); err != nil {
		state.Snapshot(m, &mem)
		return fmt.Errorf("skill: CutAhead: Cut did not complete: action=%d screen=%q", mem.U8(sym.ActionResult), state.ScreenText(&mem))
	}
	return nil
}

type cutCandidate struct {
	x, y int
	d    int
}

func vermilionCutCandidates(grid *world.Grid) []cutCandidate {
	out := make([]cutCandidate, 0)
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if grid.Walkable(x, y) {
				continue
			}
			d := absInt(x-vermilionGymX) + absInt(y-vermilionGymY)
			if d > 12 {
				continue
			}
			out = append(out, cutCandidate{x: x, y: y, d: d})
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
			bestLen = len(steps)
			best = Destination{Map: vermilionCity, X: uint8(x), Y: uint8(y)}
			found = true
		}
	}
	return best, found
}

// EnterVermilionGym handles the only exterior prerequisite of the third gym.
// It discovers the actual tree from live RAM rather than pinning a map-block
// guess, cuts it, patches only that proven cell in a temporary collision grid,
// and walks through the door warp. On return the player is controllable on
// VERMILION_GYM; OpenVermilionGym owns the separate trash-can gate inside.
func EnterVermilionGym(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != vermilionCity {
		return fmt.Errorf("skill: EnterVermilionGym: on map %#04x, want Vermilion City %#04x", m.Peek8(sym.CurMap), vermilionCity)
	}
	if policy == nil {
		return fmt.Errorf("skill: EnterVermilionGym: nil policy")
	}
	if _, err := TeachCut(m); err != nil {
		return fmt.Errorf("skill: EnterVermilionGym: %w", err)
	}

	h, err := rom.ParseMap(romData, vermilionCity)
	if err != nil {
		return fmt.Errorf("skill: EnterVermilionGym: parse Vermilion City: %w", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return fmt.Errorf("skill: EnterVermilionGym: build Vermilion City: %w", err)
	}

	var tree cutCandidate
	found := false
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
			tree, found = c, true
			break
		}
	}
	if !found {
		return fmt.Errorf("skill: EnterVermilionGym: no reachable Cut tree found near gym warp (%d,%d)", vermilionGymX, vermilionGymY)
	}
	if err := CutAhead(m); err != nil {
		return fmt.Errorf("skill: EnterVermilionGym: cut tree at (%d,%d): %w", tree.x, tree.y, err)
	}

	// The ROM-backed grid still contains the pre-Cut tree. For this one walk,
	// make only the tile the game just proved/cut passable.
	grid.Set(tree.x, tree.y, true)
	sx, sy := playerXY(m)
	var steps []world.Step
	var push world.Step
	for _, s := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		x, y := vermilionGymX+s.DX, vermilionGymY+s.DY
		if !grid.InBounds(x, y) || !grid.Walkable(x, y) {
			continue
		}
		p, err := world.FindPath(grid, int(sx), int(sy), x, y, spriteBlockers(m))
		if err == nil && (steps == nil || len(p) < len(steps)) {
			steps = p
			push = world.Step{DX: -s.DX, DY: -s.DY}
		}
	}
	if steps == nil {
		return fmt.Errorf("skill: EnterVermilionGym: no path through cut tree to gym door")
	}
	if err := WalkPath(m, steps); err != nil {
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
