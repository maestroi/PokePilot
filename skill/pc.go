package skill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	pokemonCenterPCX       uint8 = 13
	pokemonCenterPCY       uint8 = 3
	pokemonCenterPCFaceY   uint8 = 2
	pcTravelBattles              = 80
	pcTransitionBudget           = 3000
	pcCloseBudget                = 120
	gen1PartyCapacity            = 6
	gen1BoxCapacity              = 20
)

var (
	ErrPCPartyFull      = errors.New("skill: Bill's PC: party is full")
	ErrPCBoxFull        = errors.New("skill: Bill's PC: active box is full")
	ErrPCLastPartyMon   = errors.New("skill: Bill's PC: cannot deposit the last party Pokemon")
	ErrPCNoPokemon      = errors.New("skill: Bill's PC: requested Pokemon is not available")
	ErrPCNoKnownCenter  = errors.New("skill: Bill's PC: no reachable known Pokemon Center")
)

// nearestPokemonCenter chooses the known Center requiring the fewest map
// transitions from fromMap. PlaceNames is sorted, so equal-length ties are
// deterministic. Tile-level reachability is still verified by TravelFlee.
func nearestPokemonCenter(romData []byte, fromMap uint8) (Destination, string, error) {
	g, err := world.BuildGraph(romData)
	if err != nil {
		return Destination{}, "", err
	}
	bestLen := int(^uint(0) >> 1)
	var best Destination
	bestName := ""
	for _, name := range PlaceNames() {
		if !strings.HasSuffix(name, "pokemon center") {
			continue
		}
		d, ok := Place(name)
		if !ok {
			continue
		}
		route, err := world.FindRoute(g, fromMap, d.Map)
		if err != nil {
			continue
		}
		if len(route) < bestLen {
			bestLen, best, bestName = len(route), d, name
		}
	}
	if bestName == "" {
		return Destination{}, "", fmt.Errorf("%w from map %#04x", ErrPCNoKnownCenter, fromMap)
	}
	return best, bestName, nil
}

func knownPokemonCenterMap(mapID uint8) bool {
	for _, name := range PlaceNames() {
		if !strings.HasSuffix(name, "pokemon center") {
			continue
		}
		if d, ok := Place(name); ok && d.Map == mapID {
			return true
		}
	}
	return false
}

func ensureAtPokemonCenterPC(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: Bill's PC: nil move policy")
	}
	cur := m.Peek8(sym.CurMap)
	if !knownPokemonCenterMap(cur) {
		center, name, err := nearestPokemonCenter(romData, cur)
		if err != nil {
			return err
		}
		if _, err := TravelFlee(m, romData, center, policy, pcTravelBattles); err != nil {
			return fmt.Errorf("skill: Bill's PC: reach %s: %w", name, err)
		}
	}

	pc := Destination{Map: m.Peek8(sym.CurMap), X: pokemonCenterPCX, Y: pokemonCenterPCY}
	if _, err := TravelFlee(m, romData, pc, policy, 4); err != nil {
		return fmt.Errorf("skill: Bill's PC: reach PC tile: %w", err)
	}
	if err := Face(m, pokemonCenterPCX, pokemonCenterPCFaceY); err != nil {
		return fmt.Errorf("skill: Bill's PC: face PC: %w", err)
	}
	return nil
}

func pcMainMenuUp(mem *state.Mem) bool {
	text := state.ScreenText(mem)
	max := int(mem.U8(sym.MaxMenuItem))
	return state.MenuUp(mem) && mem.U8(sym.TopMenuItemX) == 1 && mem.U8(sym.TopMenuItemY) == 2 &&
		max >= 2 && max <= 4 && strings.Contains(text, "LOG OFF") && strings.Contains(text, "PC")
}

func billsPCMenuUp(mem *state.Mem) bool {
	text := state.ScreenText(mem)
	return state.MenuUp(mem) && mem.U8(sym.TopMenuItemX) == 1 && mem.U8(sym.TopMenuItemY) == 2 &&
		mem.U8(sym.MaxMenuItem) == 4 && strings.Contains(text, "WITHDRAW") && strings.Contains(text, "DEPOSIT")
}

func pcPokemonListUp(mem *state.Mem) bool {
	return state.MenuUp(mem) && mem.U8(sym.ListMenuID) == 0 &&
		mem.U8(sym.TopMenuItemX) == 5 && mem.U8(sym.TopMenuItemY) == 4 &&
		mem.U8(sym.MenuWatchedKeys) == watchListOrQty
}

func pcTransferConfirmUp(mem *state.Mem, action string) bool {
	text := state.ScreenText(mem)
	return state.MenuUp(mem) && mem.U8(sym.TopMenuItemX) == 10 && mem.U8(sym.TopMenuItemY) == 12 &&
		mem.U8(sym.MaxMenuItem) == 2 && strings.Contains(text, action) && strings.Contains(text, "STATS")
}

// pcAdvanceUntil advances only ordinary PC text while waiting for a known
// semantic menu/state. It never presses A into an unknown menu.
func pcAdvanceUntil(m *emu.Emu, pred func(*state.Mem) bool, what string) error {
	var mem state.Mem
	for spent := 0; spent < pcTransitionBudget; spent += talkSettle {
		state.Snapshot(m, &mem)
		if pred(&mem) {
			return nil
		}
		switch {
		case state.MenuUp(&mem):
			m.StepFrames(talkSettle)
		case mem.U8(sym.FontLoaded) != 0:
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
		default:
			m.StepFrames(talkSettle)
		}
	}
	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: Bill's PC: %s did not appear: menu=%t screen=%q", what, state.MenuUp(&mem), state.ScreenText(&mem))
}

func openBillsPC(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if err := ensureAtPokemonCenterPC(m, romData, policy); err != nil {
		return err
	}
	m.Tap(emu.A, 3, 7)
	if err := pcAdvanceUntil(m, pcMainMenuUp, "PC main menu"); err != nil {
		return err
	}
	if err := SelectMenuItem(m, 0); err != nil {
		return fmt.Errorf("skill: Bill's PC: select BILL/SOMEONE'S PC: %w", err)
	}
	if err := pcAdvanceUntil(m, billsPCMenuUp, "Bill's PC menu"); err != nil {
		return err
	}
	return nil
}

// closePCToOverworld backs out according to the live screen until the player
// is stably controllable. B closes menus; A pages ordinary text.
func closePCToOverworld(m *emu.Emu) error {
	var mem state.Mem
	for i := 0; i < pcCloseBudget; i++ {
		state.Snapshot(m, &mem)
		switch {
		case state.Controllable(&mem):
			m.StepFrames(talkSettle)
			state.Snapshot(m, &mem)
			if state.Controllable(&mem) {
				return nil
			}
		case state.MenuUp(&mem):
			m.Tap(emu.B, 3, 7)
			m.StepFrames(talkSettle)
		case mem.U8(sym.FontLoaded) != 0:
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
		default:
			m.StepFrames(talkSettle)
		}
	}
	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: Bill's PC: failed to close to overworld: menu=%t screen=%q", state.MenuUp(&mem), state.ScreenText(&mem))
}

// DepositPartyMon deposits one current party slot into the active Bill's PC
// box through normal gameplay. It positively verifies party/box counts and
// the appended boxed species before returning to the overworld.
func DepositPartyMon(m *emu.Emu, romData []byte, policy MovePolicy, slot int) error {
	var before state.Mem
	state.Snapshot(m, &before)
	party := state.DecodeParty(&before)
	box := state.DecodeBox(&before)
	if slot < 0 || slot >= len(party.Mons) {
		return fmt.Errorf("skill: Bill's PC: deposit slot %d out of range for party of %d", slot, party.Count)
	}
	if party.Count <= 1 {
		return ErrPCLastPartyMon
	}
	if box.Count >= gen1BoxCapacity {
		return fmt.Errorf("%w: box %d has %d Pokemon", ErrPCBoxFull, box.Number+1, box.Count)
	}
	want := party.Mons[slot].Species
	partyBefore, boxBefore := party.Count, box.Count

	if err := openBillsPC(m, romData, policy); err != nil {
		return err
	}
	if err := SelectMenuItem(m, 1); err != nil { // DEPOSIT
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Bill's PC: select DEPOSIT: %w", err)
	}
	if err := pcAdvanceUntil(m, pcPokemonListUp, "party list for deposit"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}
	if err := selectListEntry(m, slot); err != nil {
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Bill's PC: select party slot %d for deposit: %w", slot, err)
	}
	if err := pcAdvanceUntil(m, func(mem *state.Mem) bool { return pcTransferConfirmUp(mem, "DEPOSIT") }, "DEPOSIT confirmation"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}
	if err := SelectMenuItem(m, 0); err != nil {
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Bill's PC: confirm DEPOSIT: %w", err)
	}
	if err := pcAdvanceUntil(m, func(mem *state.Mem) bool {
		p, b := state.DecodeParty(mem), state.DecodeBox(mem)
		return billsPCMenuUp(mem) && p.Count+1 == partyBefore && b.Count == boxBefore+1
	}, "completed deposit"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}

	var after state.Mem
	state.Snapshot(m, &after)
	afterParty, afterBox := state.DecodeParty(&after), state.DecodeBox(&after)
	if afterParty.Count != partyBefore-1 || afterBox.Count != boxBefore+1 || len(afterBox.Mons) == 0 || afterBox.Mons[len(afterBox.Mons)-1].Species != want {
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Bill's PC: deposit postcondition failed for species %#02x: party %d->%d box %d->%d", want, partyBefore, afterParty.Count, boxBefore, afterBox.Count)
	}
	if err := closePCToOverworld(m); err != nil {
		return err
	}
	return nil
}

// WithdrawBoxMon withdraws one Pokémon from the active Bill's PC box through
// normal gameplay and verifies it was appended to the party.
func WithdrawBoxMon(m *emu.Emu, romData []byte, policy MovePolicy, boxIndex int) error {
	var before state.Mem
	state.Snapshot(m, &before)
	party := state.DecodeParty(&before)
	box := state.DecodeBox(&before)
	if party.Count >= gen1PartyCapacity {
		return ErrPCPartyFull
	}
	if boxIndex < 0 || boxIndex >= len(box.Mons) {
		return fmt.Errorf("%w: box index %d out of range for box %d with %d Pokemon", ErrPCNoPokemon, boxIndex, box.Number+1, box.Count)
	}
	want := box.Mons[boxIndex].Species
	partyBefore, boxBefore := party.Count, box.Count

	if err := openBillsPC(m, romData, policy); err != nil {
		return err
	}
	if err := SelectMenuItem(m, 0); err != nil { // WITHDRAW
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Bill's PC: select WITHDRAW: %w", err)
	}
	if err := pcAdvanceUntil(m, pcPokemonListUp, "box list for withdraw"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}
	if err := selectListEntry(m, boxIndex); err != nil {
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Bill's PC: select box index %d for withdraw: %w", boxIndex, err)
	}
	if err := pcAdvanceUntil(m, func(mem *state.Mem) bool { return pcTransferConfirmUp(mem, "WITHDRAW") }, "WITHDRAW confirmation"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}
	if err := SelectMenuItem(m, 0); err != nil {
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Bill's PC: confirm WITHDRAW: %w", err)
	}
	if err := pcAdvanceUntil(m, func(mem *state.Mem) bool {
		p, b := state.DecodeParty(mem), state.DecodeBox(mem)
		return billsPCMenuUp(mem) && p.Count == partyBefore+1 && b.Count+1 == boxBefore
	}, "completed withdraw"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}

	var after state.Mem
	state.Snapshot(m, &after)
	afterParty, afterBox := state.DecodeParty(&after), state.DecodeBox(&after)
	if afterParty.Count != partyBefore+1 || afterBox.Count != boxBefore-1 || len(afterParty.Mons) == 0 || afterParty.Mons[len(afterParty.Mons)-1].Species != want {
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Bill's PC: withdraw postcondition failed for species %#02x: party %d->%d box %d->%d", want, partyBefore, afterParty.Count, boxBefore, afterBox.Count)
	}
	if err := closePCToOverworld(m); err != nil {
		return err
	}
	return nil
}
