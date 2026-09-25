package skill

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const tradeCenterMapID uint8 = 0xef
const linkReceptionBudget = 12_000
const linkMenuBudget = 12_000
const linkMenuSelectBudget = 600
const linkRoomBudget = 12_000
const linkExchangeBudget = 50_000
const linkTradeMenuBudget = 50_000
const linkTradeConfirmationBudget = 12_000
const linkTradeExitBudget = 12_000
const linkTravelBattles = 80

// LinkTradeResult records the gameplay work done by VirtualTrade. The serial
// peer owns protocol bytes; this result owns only what the unmodified ROM did.
type LinkTradeResult struct {
	Travel    TravelResult
	Trades    int
	Tradeback bool
}

// linkStallTimeout is how long VirtualTrade tolerates the emulator making no
// frame progress. A master-clocked serial bit blocks inside StepFrame until
// the network peer replies (up to the link's per-bit timeout), so a dead peer
// freezes one frame indefinitely. Progress, not total wall time, is the
// signal: a healthy trade is ~6.5k frames, which is 3s flat out, ~30s over
// a 1ms-latency link and ~110s paced at 60fps, all with identical emulated
// behaviour. emu.WithFrameDeadline can't help: it only checks between frames.
const linkStallTimeout = 25 * time.Second

// ErrLinkStalled means the emulator made no frame progress for
// linkStallTimeout. The goroutine stepping m may still be blocked inside a
// frame when this is returned: m must not be reused afterward. The caller
// should end the run, not retry with the same emulator.
var ErrLinkStalled = errors.New("skill: VirtualTrade: link exchange stalled")

// VirtualTrade enters a normal Gen-I Cable Club Trade Center and trades the
// requested player party slot with broker slot zero. With tradeback=true it
// immediately trades the received placeholder back, causing the returned
// player Pokemon to pass through the ROM's real TryEvolvingMon path.
//
// The caller must attach a live GomeBoy link before calling this function.
func VirtualTrade(m *emu.Emu, romData []byte, playerSlot int, tradeback bool, policy MovePolicy) (LinkTradeResult, error) {
	done := make(chan linkTradeOutcome, 1)
	go func() {
		result, err := virtualTrade(m, romData, playerSlot, tradeback, policy)
		done <- linkTradeOutcome{result, err}
	}()
	o, err := awaitLinkProgress(m.Progress, done, linkStallTimeout, time.Second)
	if err != nil {
		return LinkTradeResult{Tradeback: tradeback}, err
	}
	return o.result, o.err
}

type linkTradeOutcome struct {
	result LinkTradeResult
	err    error
}

// awaitLinkProgress waits for done, failing only when progress stays
// unchanged for stall.
func awaitLinkProgress(progress func() uint64, done <-chan linkTradeOutcome, stall, poll time.Duration) (linkTradeOutcome, error) {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	last, lastAt := progress(), time.Now()
	for {
		select {
		case o := <-done:
			return o, nil
		case now := <-ticker.C:
			if p := progress(); p != last {
				last, lastAt = p, now
			} else if now.Sub(lastAt) >= stall {
				return linkTradeOutcome{}, linkExchangeStalled(last)
			}
		}
	}
}

// linkExchangeStalled is the portable poison signal for one stalled link.
// ErrLinkStalled keeps the farm's existing exit check. game.ErrMachineUnusable
// tells the objective transaction not to step or save the emulator again:
// the goroutine inside Step may still hold the frame lock.
func linkExchangeStalled(frame uint64) error {
	return fmt.Errorf("%w: %w: no frame progress for %s at frame %d",
		game.ErrMachineUnusable, ErrLinkStalled, linkStallTimeout, frame)
}

func virtualTrade(m *emu.Emu, romData []byte, playerSlot int, tradeback bool, policy MovePolicy) (LinkTradeResult, error) {
	result := LinkTradeResult{Tradeback: tradeback}
	if policy == nil {
		return result, fmt.Errorf("skill: VirtualTrade: nil move policy")
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	if playerSlot < 0 || playerSlot >= int(party.Count) {
		return result, fmt.Errorf("skill: VirtualTrade: party slot %d outside current party of %d", playerSlot, party.Count)
	}
	if party.Count < 1 {
		return result, fmt.Errorf("skill: VirtualTrade: empty party")
	}

	travel, err := reachCableClubReceptionist(m, romData, policy)
	result.Travel = travel
	if err != nil {
		return result, err
	}
	if err := enterTradeCenter(m); err != nil {
		return result, err
	}
	if err := activateTradeCenterConsole(m); err != nil {
		return result, err
	}
	if err := waitForTradeSelectionMenu(m); err != nil {
		return result, err
	}

	if err := executeCableTrade(m, playerSlot); err != nil {
		return result, fmt.Errorf("skill: VirtualTrade: first trade: %w", err)
	}
	result.Trades++

	if tradeback {
		// TradeCenter_Trade removes the selected mon, then AddEnemyMonToPlayerParty
		// appends the received one. The placeholder is therefore the last slot,
		// regardless of which original slot was traded away.
		state.Snapshot(m, &mem)
		count := int(state.DecodeParty(&mem).Count)
		if count < 1 {
			return result, fmt.Errorf("skill: VirtualTrade: party empty after first trade")
		}
		if err := executeCableTrade(m, count-1); err != nil {
			return result, fmt.Errorf("skill: VirtualTrade: tradeback: %w", err)
		}
		result.Trades++
	}

	if err := leaveTradeCenter(m); err != nil {
		return result, err
	}
	return result, nil
}

func reachCableClubReceptionist(m *emu.Emu, romData []byte, policy MovePolicy) (TravelResult, error) {
	var total TravelResult
	cur := m.Peek8(sym.CurMap)
	if !knownPokemonCenterMap(cur) {
		travel, name, err := reachNearestPokemonCenter(m, romData, policy, linkTravelBattles)
		total = combineLinkTravel(total, travel)
		if err != nil {
			if name != "" {
				return total, fmt.Errorf("skill: VirtualTrade: reach %s: %w", name, err)
			}
			return total, fmt.Errorf("skill: VirtualTrade: find Pokemon Center: %w", err)
		}
	}

	mapID := m.Peek8(sym.CurMap)
	actors, err := rom.SpecialInteractionActors(romData, mapID)
	if err != nil {
		return total, fmt.Errorf("skill: VirtualTrade: inspect Cable Club receptionist: %w", err)
	}
	var actor rom.SpecialInteractionActor
	found := false
	for _, candidate := range actors {
		if candidate.Role == rom.InteractionCableClub {
			actor, found = candidate, true
			break
		}
	}
	if !found {
		return total, fmt.Errorf("skill: VirtualTrade: map %#04x has no Cable Club receptionist", mapID)
	}

	header, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return total, fmt.Errorf("skill: VirtualTrade: parse Pokemon Center: %w", err)
	}
	grid, err := world.Build(romData, header)
	if err != nil {
		return total, fmt.Errorf("skill: VirtualTrade: build Pokemon Center collision grid: %w", err)
	}

	// Receptionists are static service actors. Prefer the tile directly below
	// them, then the other three adjacent floor tiles. Each candidate is proved
	// walkable from ROM collision data before navigation tries it.
	steps := []world.Step{world.StepDown, world.StepLeft, world.StepRight, world.StepUp}
	var lastErr error
	for _, step := range steps {
		x, y := int(actor.X)+step.DX, int(actor.Y)+step.DY
		if !grid.InBounds(x, y) || !grid.Walkable(x, y) {
			continue
		}
		dest := Destination{Map: mapID, X: uint8(x), Y: uint8(y)}
		travel, err := TravelFlee(m, romData, dest, policy, 8)
		total = combineLinkTravel(total, travel)
		if err != nil {
			lastErr = err
			continue
		}
		if err := Face(m, actor.X, actor.Y); err != nil {
			lastErr = err
			continue
		}
		return total, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no adjacent walkable interaction tile")
	}
	return total, fmt.Errorf("skill: VirtualTrade: reach Cable Club receptionist at (%d,%d): %w", actor.X, actor.Y, lastErr)
}

func combineLinkTravel(a, b TravelResult) TravelResult {
	a.Battles += b.Battles
	a.Flees += b.Flees
	a.Dialogues += b.Dialogues
	a.BlackedOut = a.BlackedOut || b.BlackedOut
	a.Replans = append(a.Replans, b.Replans...)
	a.EmergencyEgresses = append(a.EmergencyEgresses, b.EmergencyEgresses...)
	return a
}

func enterTradeCenter(m *emu.Emu) error {
	m.Tap(emu.A, 3, 7)

	// Page only ordinary receptionist dialogue. Stop as soon as the save
	// YES/NO appears; never press A into an unknown live menu.
	if err := linkAdvanceUntil(m, linkReceptionBudget, func(mem *state.Mem) bool {
		prompt := state.DecodeTwoOptionMenu(mem)
		return prompt != nil && state.DecodeInteraction(mem).Options[0] == "YES"
	}, true, "Cable Club save prompt"); err != nil {
		return err
	}
	if err := selectTwoOption(m, 0); err != nil {
		return fmt.Errorf("skill: VirtualTrade: accept Cable Club save: %w", err)
	}

	// Saving and serial synchronization proceed without player input. The next
	// safe controller surface is the explicit three-item LinkMenu.
	if err := linkAdvanceUntil(m, linkMenuBudget, linkMenuScreen, false, "Cable Club link menu"); err != nil {
		return err
	}
	return selectTradeCenter(m)
}

// selectTradeCenter confirms TRADE CENTER on LinkMenu. LinkMenu alternates
// HandleMenuInput with Serial_ExchangeLinkMenuSelection, which spans several
// frames over a network link, so a short tap can land entirely inside the
// exchange and be lost. Hold A until the ROM records the agreed destination.
func selectTradeCenter(m *emu.Emu) error {
	m.StepFrames(talkSettle)
	var mem state.Mem
	state.Snapshot(m, &mem)
	if cur := mem.U8(sym.CurrentMenuItem); cur != 0 {
		return fmt.Errorf("skill: VirtualTrade: select TRADE CENTER: link menu cursor at %d, want 0", cur)
	}
	if _, err := m.HoldUntil(emu.A, linkMenuSelectBudget, func(em *emu.Emu) bool {
		return em.Peek8(sym.CableClubDestinationMap) != 0
	}); err != nil {
		return fmt.Errorf("skill: VirtualTrade: select TRADE CENTER: link menu did not accept A: %w", err)
	}
	if dest := m.Peek8(sym.CableClubDestinationMap); dest != tradeCenterMapID {
		return fmt.Errorf("skill: VirtualTrade: select TRADE CENTER: link menu chose map %#04x", dest)
	}
	return nil
}

func linkMenuScreen(mem *state.Mem) bool {
	text := strings.ToUpper(state.ScreenText(mem))
	return strings.Contains(text, "TRADE CENTER") && strings.Contains(text, "COLOSSEUM") && strings.Contains(text, "CANCEL") &&
		mem.U8(sym.TopMenuItemY) == 7 && mem.U8(sym.TopMenuItemX) == 6
}

// activateTradeCenterConsole performs the interaction that actually starts
// CableClub_DoBattleOrTrade. Selecting TRADE CENTER at the receptionist only
// warps both players into map 0xef; the ROM does not begin party exchange until
// the local player presses A on the Game Boy at the table.
//
// Red places the internal-clock player at (3,4), beside the left console
// hidden event at (4,4), and the external-clock player at (6,4), beside the
// right console at (5,4). CableClubLeftGameboy/CableClubRightGameboy then set
// wLinkState=LINK_STATE_START_TRADE and the map loop begins serial exchange.
func activateTradeCenterConsole(m *emu.Emu) error {
	var mem state.Mem
	if _, err := m.StepUntil(linkRoomBudget, func(em *emu.Emu) bool {
		state.Snapshot(em, &mem)
		if mem.U8(sym.CurMap) != tradeCenterMapID || !state.Controllable(&mem) {
			return false
		}
		_, _, ok := tradeCenterConsoleTarget(mem.U8(sym.XCoord), mem.U8(sym.YCoord))
		return ok
	}); err != nil {
		state.Snapshot(m, &mem)
		return fmt.Errorf("skill: VirtualTrade: Trade Center console position did not become ready: map=%#04x at (%d,%d): %w",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), err)
	}

	tx, ty, ok := tradeCenterConsoleTarget(mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	if !ok {
		return fmt.Errorf("skill: VirtualTrade: no Cable Club console adjacent at (%d,%d)",
			mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
	if err := Face(m, tx, ty); err != nil {
		return fmt.Errorf("skill: VirtualTrade: face Cable Club console at (%d,%d): %w", tx, ty, err)
	}
	m.Tap(emu.A, 3, 7)
	return nil
}

func tradeCenterConsoleTarget(x, y uint8) (uint8, uint8, bool) {
	if y != 4 {
		return 0, 0, false
	}
	switch x {
	case 3:
		return 4, 4, true
	case 6:
		return 5, 4, true
	default:
		return 0, 0, false
	}
}

func tradeSelectionMenu(mem *state.Mem) bool {
	if mem.U8(sym.CurMap) != tradeCenterMapID {
		return false
	}
	partyCount := mem.U8(sym.PartyCount)
	if partyCount == 0 {
		return false
	}
	// TradeCenter_SelectMon itself establishes this exact controller shape:
	// player menu at (1,1), wMaxMenuItem == wPartyCount. Do not require
	// ScreenText here. The Cable Club clears/redraws the tilemap around the
	// serial exchange, and a perfectly live selection menu can therefore have
	// no decodable text in our WRAM shadow even though HandleMenuInput is active.
	// The map + coordinates + party-derived bound are the ROM-owned liveness
	// invariants and avoid treating rendering as control state.
	return mem.U8(sym.TopMenuItemY) == 1 && mem.U8(sym.TopMenuItemX) == 1 &&
		mem.U8(sym.MaxMenuItem) == partyCount &&
		mem.U8(sym.CurrentMenuItem) <= partyCount
}

func waitForTradeSelectionMenu(m *emu.Emu) error {
	if err := linkAdvanceUntil(m, linkExchangeBudget, tradeSelectionMenu, false, "Trade Center party exchange/menu"); err != nil {
		return err
	}
	return nil
}

func executeCableTrade(m *emu.Emu, slot int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	count := int(mem.U8(sym.PartyCount))
	if slot < 0 || slot >= count {
		return fmt.Errorf("party slot %d outside current party of %d", slot, count)
	}
	if !tradeSelectionMenu(&mem) {
		return fmt.Errorf("player trade menu is not active")
	}
	// In TradeCenter_SelectMon, wMaxMenuItem is PartyCount and the Pokemon
	// entries are 0..PartyCount-1, so SelectMenuItem's exclusive bound is
	// exactly correct for real party slots (the extra cancel row is the only
	// inclusive index and is handled separately below).
	if err := SelectMenuItem(m, slot); err != nil {
		return fmt.Errorf("select player party slot %d: %w", slot, err)
	}
	if err := linkAdvanceUntil(m, linkTradeMenuBudget, tradeActionMenu, false, "STATS/TRADE action menu"); err != nil {
		return err
	}

	// The action popup is a horizontal hand-written menu. Right changes from
	// STATS to TRADE by moving wTopMenuItemX from 1 to 11; prove that before A.
	m.Tap(emu.Right, 3, 7)
	if _, err := m.StepUntil(120, func(em *emu.Emu) bool { return em.Peek8(sym.TopMenuItemX) == 11 }); err != nil {
		return fmt.Errorf("select TRADE action: cursor did not move right: %w", err)
	}
	m.Tap(emu.A, 3, 7)

	// TradeCenter_Trade prints _WillBeTradedText, whose `cont` scrolls a full
	// two-line box and waits for a button before the TRADE/CANCEL menu.
	if err := linkAdvanceUntil(m, linkTradeConfirmationBudget, tradeConfirmationMenu, true, "TRADE/CANCEL confirmation"); err != nil {
		return err
	}
	if err := selectTwoOption(m, 0); err != nil {
		return fmt.Errorf("confirm trade: %w", err)
	}

	// The ROM performs removal/addition, the trade animation, TryEvolvingMon,
	// SavePartyAndDexData, then immediately exchanges party data again before
	// returning to the selection screen. Waiting for that screen proves the
	// whole in-game transaction completed rather than just the confirmation.
	if err := waitForTradeSelectionMenu(m); err != nil {
		return fmt.Errorf("wait for completed trade: %w", err)
	}
	return nil
}

func tradeActionMenu(mem *state.Mem) bool {
	text := strings.ToUpper(state.ScreenText(mem))
	return mem.U8(sym.CurMap) == tradeCenterMapID && mem.U8(sym.TopMenuItemY) == 16 &&
		strings.Contains(text, "STATS") && strings.Contains(text, "TRADE")
}

func tradeConfirmationMenu(mem *state.Mem) bool {
	interaction := state.DecodeInteraction(mem)
	return mem.U8(sym.CurMap) == tradeCenterMapID && interaction.Kind == state.InteractionTwoOption &&
		interaction.Options[0] == "TRADE" && interaction.Options[1] == "CANCEL"
}

func leaveTradeCenter(m *emu.Emu) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !tradeSelectionMenu(&mem) {
		return fmt.Errorf("skill: VirtualTrade: cannot leave: trade selection menu is not active")
	}
	// The cancel row is index PartyCount and wMaxMenuItem stores that same
	// last-valid index. SelectMenuItem intentionally uses exclusive semantics,
	// so this one special menu needs an inclusive step-and-verify cursor move.
	target := int(mem.U8(sym.MaxMenuItem))
	const stuckLimit = 5
	stuck := 0
	for {
		cur := int(m.Peek8(sym.CurrentMenuItem))
		if cur == target {
			break
		}
		btn := emu.Down
		if cur > target {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if _, err := m.StepUntil(120, func(em *emu.Emu) bool { return int(em.Peek8(sym.CurrentMenuItem)) != cur }); err != nil {
			stuck++
			if stuck >= stuckLimit {
				return fmt.Errorf("skill: VirtualTrade: cancel cursor stuck at %d, want %d: %w", cur, target, err)
			}
		} else {
			stuck = 0
		}
	}
	m.Tap(emu.A, 3, 7)

	// Both sides cancelling runs ReturnToCableClubRoom: the player is back in
	// the Trade Center room, which has no warps. Gen I has no in-game exit from
	// a link room; the player resets and CONTINUEs from the receptionist's
	// save, which already holds the traded party (SavePartyAndDexData).
	if _, err := m.StepUntil(linkTradeExitBudget, func(em *emu.Emu) bool {
		var snap state.Mem
		state.Snapshot(em, &snap)
		return snap.U8(sym.CurMap) == tradeCenterMapID && state.Controllable(&snap)
	}); err != nil {
		return fmt.Errorf("skill: VirtualTrade: return to Trade Center room: %w", err)
	}
	if err := softResetToSave(m); err != nil {
		return fmt.Errorf("skill: VirtualTrade: leave Trade Center: %w", err)
	}
	return nil
}

var softResetButtons = []emu.Button{emu.A, emu.B, emu.Start, emu.Select}

// softResetToSave performs Red's A+B+START+SELECT reset and continues from
// the last save. _Joypad counts hSoftReset down once per poll while all four
// are held, and Init then clears WRAM, so wCurMap leaving the link room is
// the proof the reset happened.
func softResetToSave(m *emu.Emu) error {
	for _, b := range softResetButtons {
		m.Press(b)
	}
	_, err := m.StepUntil(linkTradeExitBudget, func(em *emu.Emu) bool { return em.Peek8(sym.CurMap) != tradeCenterMapID })
	for _, b := range softResetButtons {
		m.Release(b)
	}
	if err != nil {
		return fmt.Errorf("soft reset: %w", err)
	}

	// Title and intro accept START; the main menu opens with CONTINUE on
	// item 0 whenever a save exists.
	if err := tapUntil(m, linkTradeExitBudget, mainMenuScreen, emu.Start, "main menu"); err != nil {
		return err
	}
	m.Tap(emu.A, 3, 7)
	// DisplayContinueGameInfo waits for A, then SpecialEnterMap idles 20
	// frames with the saved map in wCurMap but not loaded, so Controllable
	// alone reads true while input is still dropped. Init left
	// wUpdateSpritesEnabled at $ff; LoadMapData setting it to 1 proves EnterMap
	// has run.
	return tapUntil(m, linkTradeExitBudget, func(mem *state.Mem) bool {
		return mem.U8(sym.CurMap) != tradeCenterMapID && mem.U8(sym.UpdateSpritesEnabled) == 1 &&
			state.Controllable(mem) && state.DecodeBattle(mem) == nil
	}, emu.A, "continued overworld")
}

// tapUntil taps btn between checks until pred holds. Only for screens where
// btn is the sole way forward (title, CONTINUE info); pred is checked first
// so the tap never reaches the surface it was waiting for.
func tapUntil(m *emu.Emu, budget int, pred func(*state.Mem) bool, btn emu.Button, what string) error {
	var mem state.Mem
	for spent := 0; spent < budget; spent += talkSettle {
		state.Snapshot(m, &mem)
		if pred(&mem) {
			return nil
		}
		m.Tap(btn, 3, 7)
		m.StepFrames(talkSettle)
	}
	return fmt.Errorf("skill: VirtualTrade: %s did not appear: screen=%q", what, state.ScreenText(&mem))
}

func mainMenuScreen(mem *state.Mem) bool {
	text := strings.ToUpper(state.ScreenText(mem))
	return strings.Contains(text, "CONTINUE") && strings.Contains(text, "NEW GAME") &&
		mem.U8(sym.TopMenuItemX) == 1 && mem.U8(sym.TopMenuItemY) == 2 && mem.U8(sym.CurrentMenuItem) == 0
}

func linkAdvanceUntil(m *emu.Emu, budget int, pred func(*state.Mem) bool, pageDialogue bool, what string) error {
	var mem state.Mem
	for spent := 0; spent < budget; spent += talkSettle {
		state.Snapshot(m, &mem)
		if pred(&mem) {
			return nil
		}
		interaction := state.DecodeInteraction(&mem)
		if pageDialogue && interaction.Kind == state.InteractionDialogue {
			m.Tap(emu.A, 3, 7)
		}
		m.StepFrames(talkSettle)
	}
	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: VirtualTrade: %s did not appear: map=%#04x at (%d,%d) interaction=%s screen=%q",
		what, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), state.DecodeInteraction(&mem).Kind, state.ScreenText(&mem))
}
