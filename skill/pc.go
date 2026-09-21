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

// pokemonCenterPCY is the tile the player stands on to use the PC; the PC
// terminal itself sits one row up at pokemonCenterPCFaceY and is solid.
// MEASURED on run-39nani97cwabo2y8cozb42u2ca's Vermilion Pokecenter (map
// 0x59): (13,3) decodes as collision tile 0x52 (impassable), while (13,4) is
// walkable and carries the distinct "facing PC" field tile.
const (
	pokemonCenterPCX     uint8 = 13
	pokemonCenterPCY     uint8 = 4
	pokemonCenterPCFaceY uint8 = 3
	pcTravelBattles            = 80
	pcTransitionBudget         = 3000
	pcCloseBudget              = 120
	pcCenterEscapeLimit        = 4
	gen1PartyCapacity          = 6
	gen1BoxCapacity            = 20
)

var (
	ErrPCPartyFull     = errors.New("skill: Bill's PC: party is full")
	ErrPCBoxFull       = errors.New("skill: Bill's PC: active box is full")
	ErrPCLastPartyMon  = errors.New("skill: Bill's PC: cannot deposit the last party Pokemon")
	ErrPCNoPokemon     = errors.New("skill: Bill's PC: requested Pokemon is not available")
	ErrPCNoKnownCenter = errors.New("skill: Bill's PC: no reachable known Pokemon Center")
)

// nearestPokemonCenter chooses the known Center requiring the fewest map
// transitions from fromMap. PlaceNames is sorted, so equal-length ties are
// deterministic. Tile-level reachability is still verified by TravelFlee.
func nearestPokemonCenter(romData []byte, fromMap uint8) (Destination, string, error) {
	g, err := world.BuildGraph(romData)
	if err != nil {
		return Destination{}, "", err
	}
	return nearestPokemonCenterInGraph(g, fromMap)
}

func nearestPokemonCenterInGraph(g *world.Graph, fromMap uint8) (Destination, string, error) {
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

// singleDestinationWarpExit reports the mandatory first hop of a room whose
// graph exits are all warp tiles to one external map. Paired door tiles are
// therefore one logical exit. This deliberately excludes connections and
// multi-neighbor interiors: only a hop every cross-map journey must take is
// safe to execute before the full destination becomes routable.
//
// Route 16's Fly House is the measured case: its LAST_MAP doors correctly
// resolve to Route 16, but they land inside the Cut-gated Fly pocket. Static
// component routing cannot prove a complete path from inside the house to any
// Pokemon Center, even though the only legal first action is to walk outside.
// Once outside, TravelFlee's live field-path bridge can Cut out of the pocket.
func singleDestinationWarpExit(g *world.Graph, fromMap uint8) (world.Edge, bool) {
	if g == nil {
		return world.Edge{}, false
	}
	edges := g.Edges[fromMap]
	if len(edges) == 0 {
		return world.Edge{}, false
	}
	to := edges[0].To
	if to == fromMap {
		return world.Edge{}, false
	}
	for _, e := range edges {
		if e.Kind != world.EdgeWarp || e.To != to {
			return world.Edge{}, false
		}
	}
	return edges[0], true
}

// reachNearestPokemonCenter owns the whole "get to a Center" preflight shared
// by Bill's PC and VirtualTrade. Normally it selects a Center and delegates to
// TravelFlee. If component-aware routing cannot name any Center while the
// player is inside a single-destination warp room, it takes that mandatory
// room exit first and re-runs selection from the measured landing. This is
// bounded and generic; no map id or Route 16 coordinate is special-cased.
func reachNearestPokemonCenter(m *emu.Emu, romData []byte, policy MovePolicy, maxBattles int) (TravelResult, string, error) {
	g, err := world.BuildGraph(romData)
	if err != nil {
		return TravelResult{}, "", err
	}
	for escaped := 0; ; escaped++ {
		cur := m.Peek8(sym.CurMap)
		if knownPokemonCenterMap(cur) {
			return TravelResult{}, "", nil
		}

		center, name, selectErr := nearestPokemonCenterInGraph(g, cur)
		if selectErr == nil {
			travel, travelErr := TravelFlee(m, romData, center, policy, maxBattles)
			return travel, name, travelErr
		}
		if !errors.Is(selectErr, ErrPCNoKnownCenter) {
			return TravelResult{}, "", selectErr
		}
		if escaped >= pcCenterEscapeLimit {
			return TravelResult{}, "", selectErr
		}

		exit, ok := singleDestinationWarpExit(g, cur)
		if !ok {
			return TravelResult{}, "", selectErr
		}
		// A room can have several non-equivalent doors to the same outside
		// map (Cerulean's Badge House is the canonical shape). Prefer the
		// edge the component-aware router can reach from the player's actual
		// tile; Traverse still owns live sprite blockers and paired-door
		// candidate selection at execution time.
		x, y := playerXY(m)
		if route, routeErr := world.FindRouteAt(g, cur, exit.To, int(x), int(y), nil); routeErr == nil && len(route) > 0 {
			exit = route[0]
		}
		if err := Traverse(m, romData, exit); err != nil {
			return TravelResult{}, "", fmt.Errorf("skill: Pokemon Center travel: leave mandatory warp room %#04x toward %#04x: %w", cur, exit.To, err)
		}
	}
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
		_, name, err := reachNearestPokemonCenter(m, romData, policy, pcTravelBattles)
		if err != nil {
			if name != "" {
				return fmt.Errorf("skill: Bill's PC: reach %s: %w", name, err)
			}
			return err
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

// pcMainMenuScreen is pcMainMenuUp without the live-cursor requirement; see
// billsPCMenuScreen for why that requirement can be unreliable on a screen
// reached by backing out of a nested menu rather than opening it fresh.
func pcMainMenuScreen(mem *state.Mem) bool {
	text := state.ScreenText(mem)
	max := int(mem.U8(sym.MaxMenuItem))
	return mem.U8(sym.TopMenuItemX) == 1 && mem.U8(sym.TopMenuItemY) == 2 &&
		max >= 2 && max <= 4 && strings.Contains(text, "LOG OFF") && strings.Contains(text, "PC")
}

func pcMainMenuUp(mem *state.Mem) bool {
	return state.MenuUp(mem) && pcMainMenuScreen(mem)
}

// billsPCMenuScreen is billsPCMenuUp without the live-cursor requirement.
// MEASURED: returning to this menu from B-cancelling the party/box list after
// a completed transfer leaves TopMenuItemX/Y, MaxMenuItem and the menu text
// all correct and stable, but the cursor glyph state.MenuUp looks for is not
// drawn for a long time (2900+ frames, no change) — the game only redraws it
// on the next directional input, not merely because the frame count moved
// on. pcCancelUntil uses this relaxed form as its arrival target so it does
// not mistake "arrived, cursor not yet redrawn" for "still need to cancel
// out further" and keep pressing B straight past the destination.
func billsPCMenuScreen(mem *state.Mem) bool {
	text := state.ScreenText(mem)
	return mem.U8(sym.TopMenuItemX) == 1 && mem.U8(sym.TopMenuItemY) == 2 &&
		mem.U8(sym.MaxMenuItem) == 4 && strings.Contains(text, "WITHDRAW") && strings.Contains(text, "DEPOSIT")
}

func billsPCMenuUp(mem *state.Mem) bool {
	return state.MenuUp(mem) && billsPCMenuScreen(mem)
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
// semantic menu/state. It never presses A into an unknown menu — except when
// from reports that the screen is still the one we departed from: a PC
// confirmation message ("Accessed BILL's PC.", "... was deposited.") overlays
// its text box without erasing the previous screen's item list, so its
// cursor glyph stays on the tilemap and state.MenuUp keeps reporting a menu
// is up long after there is only a dismissible message left to page. MEASURED
// on the 321-run Vermilion Gym farm failure: after SelectMenuItem(0) chose
// BILL's PC from the main list, the screen sat on "Accessed BILL's PC." with
// pcMainMenuUp's own cursor/text still intact underneath, so the MenuUp
// branch waited passively forever instead of paging the confirmation.
// from may be nil when there is no known prior PC screen to distinguish from
// (e.g. the very first wait, right after tapping the PC in the overworld).
func pcAdvanceUntil(m *emu.Emu, from, pred func(*state.Mem) bool, what string) error {
	var mem state.Mem
	for spent := 0; spent < pcTransitionBudget; spent += talkSettle {
		state.Snapshot(m, &mem)
		if pred(&mem) {
			return nil
		}
		switch {
		case mem.U8(sym.FontLoaded) != 0 && from != nil && from(&mem):
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
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

// pcCancelUntil backs out of real, currently-interactive PC menus with B
// until pred matches. Unlike pcAdvanceUntil's from-gated A-press (for paging
// a dismissible confirmation message stuck over a stale background list),
// this is for a screen that is itself a live menu awaiting a choice: pressing
// A there selects whatever is highlighted (e.g. reopens the transfer's
// WITHDRAW/DEPOSIT/STATS/CANCEL popup on the party list), so only B is safe.
func pcCancelUntil(m *emu.Emu, pred func(*state.Mem) bool, what string) error {
	var mem state.Mem
	for spent := 0; spent < pcTransitionBudget; spent += talkSettle {
		state.Snapshot(m, &mem)
		if pred(&mem) {
			return nil
		}
		switch {
		case state.MenuUp(&mem):
			m.Tap(emu.B, 3, 7)
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
	if err := pcAdvanceUntil(m, nil, pcMainMenuUp, "PC main menu"); err != nil {
		return err
	}
	if err := SelectMenuItem(m, 0); err != nil {
		return fmt.Errorf("skill: Bill's PC: select BILL/SOMEONE'S PC: %w", err)
	}
	if err := pcAdvanceUntil(m, pcMainMenuUp, billsPCMenuUp, "Bill's PC menu"); err != nil {
		return err
	}
	return nil
}

// closePCToOverworld backs out according to the live screen until the player
// is stably controllable. B closes menus; A pages ordinary text. A known PC
// menu screen (pcMainMenuScreen/billsPCMenuScreen/playersPCMenuScreen) is checked ahead of the
// bare FontLoaded/press-A fallback even when state.MenuUp's cursor glyph
// is not currently drawn there — see billsPCMenuScreen — so a menu reached by
// backing out of a nested screen is closed with B rather than mistaken for
// ordinary dismissible text and paged with A, which would select whatever
// item the menu defaults to instead of leaving it.
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
		case state.MenuUp(&mem), pcMainMenuScreen(&mem), billsPCMenuScreen(&mem), playersPCMenuScreen(&mem):
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
	if err := pcAdvanceUntil(m, billsPCMenuUp, pcPokemonListUp, "party list for deposit"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}
	if err := selectListEntry(m, slot); err != nil {
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Bill's PC: select party slot %d for deposit: %w", slot, err)
	}
	depositConfirmUp := func(mem *state.Mem) bool { return pcTransferConfirmUp(mem, "DEPOSIT") }
	if err := pcAdvanceUntil(m, pcPokemonListUp, depositConfirmUp, "DEPOSIT confirmation"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}
	if err := SelectMenuItem(m, 0); err != nil {
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Bill's PC: confirm DEPOSIT: %w", err)
	}
	// A successful transfer usually does not return to the WITHDRAW/DEPOSIT/
	// RELEASE menu directly: the game leaves the party list up (so another
	// deposit can be picked without reopening it) and B is what backs out
	// from there. But depositing down to the last remaining party Pokemon
	// leaves nothing more that could be deposited, so — exactly like
	// WithdrawBoxMon's withdrawnUp for an emptied box — the game skips the
	// list and returns straight to billsPCMenuScreen instead. MEASURED via
	// skill/zz_repro_scratch_test.go: a 2-Pokemon party depositing down to 1
	// landed on billsPCMenuScreen with "You can't deposit the last POKéMON!"
	// stuck on screen forever, because this predicate only accepted the list.
	depositedListUp := func(mem *state.Mem) bool {
		p, b := state.DecodeParty(mem), state.DecodeBox(mem)
		if p.Count+1 != partyBefore || b.Count != boxBefore+1 {
			return false
		}
		return pcPokemonListUp(mem) || billsPCMenuScreen(mem)
	}
	if err := pcAdvanceUntil(m, depositConfirmUp, depositedListUp, "post-deposit party list"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}
	if err := pcCancelUntil(m, billsPCMenuScreen, "completed deposit"); err != nil {
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
	if err := pcAdvanceUntil(m, billsPCMenuUp, pcPokemonListUp, "box list for withdraw"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}
	if err := selectListEntry(m, boxIndex); err != nil {
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Bill's PC: select box index %d for withdraw: %w", boxIndex, err)
	}
	withdrawConfirmUp := func(mem *state.Mem) bool { return pcTransferConfirmUp(mem, "WITHDRAW") }
	if err := pcAdvanceUntil(m, pcPokemonListUp, withdrawConfirmUp, "WITHDRAW confirmation"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}
	if err := SelectMenuItem(m, 0); err != nil {
		_ = closePCToOverworld(m)
		return fmt.Errorf("skill: Bill's PC: confirm WITHDRAW: %w", err)
	}
	// Withdrawing the box's last Pokemon leaves nothing for the list to show,
	// so the game skips straight back to billsPCMenuUp instead of reopening
	// the (now empty) list the way every other transfer count does.
	withdrawnUp := func(mem *state.Mem) bool {
		p, b := state.DecodeParty(mem), state.DecodeBox(mem)
		if p.Count != partyBefore+1 || b.Count+1 != boxBefore {
			return false
		}
		return pcPokemonListUp(mem) || billsPCMenuScreen(mem)
	}
	if err := pcAdvanceUntil(m, withdrawConfirmUp, withdrawnUp, "post-withdraw state"); err != nil {
		_ = closePCToOverworld(m)
		return err
	}
	if err := pcCancelUntil(m, billsPCMenuScreen, "completed withdraw"); err != nil {
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

// EnsurePartySlot makes room for one incoming catch when the party is full.
// It deposits the safest surplus member into the active box and keeps every
// core field-move the save has already unlocked. A full box is a structured
// blockage, not a deposit attempt that would lose the catch.
func EnsurePartySlot(m *emu.Emu, romData []byte, policy MovePolicy, incoming uint8) error {
	if policy == nil {
		return fmt.Errorf("skill: Bill's PC: nil move policy")
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	slot, err := planPartySlot(romData, state.DecodeParty(&mem), state.DecodeBox(&mem), state.Mon{Species: incoming}, OwnedCoreProgressionFieldMoves(&mem))
	if err != nil {
		return err
	}
	if slot < 0 {
		return nil
	}
	return DepositPartyMon(m, romData, policy, slot)
}

func planPartySlot(romData []byte, party state.PartyState, box state.BoxState, incoming state.Mon, required []FieldMove) (int, error) {
	if int(party.Count) < gen1PartyCapacity {
		return -1, nil
	}
	if int(box.Count) >= gen1BoxCapacity {
		return -1, fmt.Errorf("%w: box %d has %d Pokemon", ErrPCBoxFull, box.Number+1, box.Count)
	}
	slot, ok, err := chooseDepositSlotForIncoming(romData, party, incoming, required)
	if err != nil {
		return -1, err
	}
	if !ok || slot < 0 {
		return -1, fmt.Errorf("%w: depositing any party member would drop a required field move", ErrFieldRosterNoRecovery)
	}
	return slot, nil
}
