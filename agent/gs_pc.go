package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/gs/sym"
	"github.com/maestroi/pokepilot/skill"
)

// errGSPCRefused is the ordinary blocked outcome of a Bill's PC transfer the
// game refused (box full, party full, last Pokemon).
var errGSPCRefused = errors.New("Gold/Silver Bill's PC refused the transfer")

const (
	// Every Gold/Silver Pokemon Center 1F shares one layout: the PC is the
	// COLL_PC tile at (9,1), used from (9,2) facing up.
	gsPokecenterPCX      uint8 = 9
	gsPokecenterPCY      uint8 = 1
	gsPokecenterPCStandX uint8 = 9
	gsPokecenterPCStandY uint8 = 2

	gsPCMenuHold     = 8
	gsPCStepFrames   = 20
	gsPCPhaseBudget  = 120 // gsPCStepFrames polls
	gsPCMaxListMoves = 40

	// BillsPC transfer jumptable states (engine/pokemon/bills_pc.asm).
	gsPCStateJoypad  byte = 1
	gsPCStateSubmenu byte = 3

	gsPCRowWithdraw = 0
	gsPCRowDeposit  = 1
)

func gsPCText(m *emu.Emu, profile *gsprofile.Profile) string { return profile.ScreenText(m) }

func gsPCMenuShape(m *emu.Emu) (rows, cols byte) {
	return m.Peek8(sym.TwoDMenuNumRows), m.Peek8(sym.TwoDMenuNumCols)
}

// gsBillsPCMenuUp is the WITHDRAW/DEPOSIT/CHANGE BOX/MOVE/SEE YA! menu.
func gsBillsPCMenuUp(m *emu.Emu, profile *gsprofile.Profile) bool {
	rows, cols := gsPCMenuShape(m)
	text := gsPCText(m, profile)
	return rows == 5 && cols == 1 && strings.Contains(text, "WITHDRAW") && strings.Contains(text, "SEE YA!")
}

// gsPCListUp is a transfer list polling input with the expected source loaded.
func gsPCListUp(m *emu.Emu, profile *gsprofile.Profile, loaded byte) bool {
	return m.Peek8(sym.JumptableIndex) == gsPCStateJoypad && m.Peek8(sym.BillsPCLoadedBox) == loaded &&
		strings.Contains(gsPCText(m, profile), "Choose a")
}

func gsPCList(m *emu.Emu) []uint8 {
	var out []uint8
	for i := 0; i < 30; i++ {
		species := m.Peek8(sym.BillsPCList + uint16(i*sym.BillsPCListEntrySize))
		if species == 0 || species == 0xff {
			break
		}
		out = append(out, species)
	}
	return out
}

func gsPCWait(m *emu.Emu, pred func() bool) bool {
	for i := 0; i < gsPCPhaseBudget; i++ {
		if pred() {
			return true
		}
		m.StepFrames(gsPCStepFrames)
	}
	return pred()
}

// gsOpenBillsPC walks to the area's Pokemon Center PC and opens Bill's PC
// (the first row, labelled BILL's or SOMEONE's PC). Plain text boxes are the
// only things advanced with A.
func gsOpenBillsPC(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	center, err := gsOwnedPokecenterID(profile.DecodeOverworld(m).NativeMapID)
	if err != nil {
		return err
	}
	if err := gsSecondBadgeFaceFrom(m, romData, profile,
		skill.ExactNativeDestination(center, gsPokecenterPCStandX, gsPokecenterPCStandY),
		gsPokecenterPCX, gsPokecenterPCY, "gen2:bills-pc"); err != nil {
		return fmt.Errorf("reach PC: %w", err)
	}
	m.Tap(emu.A, gsPCMenuHold, 7)
	for i := 0; i < gsPCPhaseBudget; i++ {
		m.StepFrames(gsPCStepFrames)
		if gsBillsPCMenuUp(m, profile) {
			return nil
		}
		text := gsPCText(m, profile)
		switch {
		case strings.Contains(text, "Access whose PC?"):
			if err := skill.SelectMenuItem(m, 0); err != nil {
				return fmt.Errorf("choose Bill's PC: %w", err)
			}
		case text != "":
			m.Tap(emu.A, gsPCMenuHold, 7)
		}
	}
	return fmt.Errorf("%w: Bill's PC menu did not open (screen %q)", errGSSecondBadgeStalled, gsPCText(m, profile))
}

// gsCloseBillsPC backs out with B until the player is controllable.
func gsCloseBillsPC(m *emu.Emu, profile *gsprofile.Profile) error {
	for i := 0; i < gsPCPhaseBudget; i++ {
		if profile.DecodeOverworld(m).Controllable {
			m.StepFrames(gsPCStepFrames)
			if profile.DecodeOverworld(m).Controllable {
				return nil
			}
		}
		m.Tap(emu.B, gsPCMenuHold, 7)
		m.StepFrames(gsPCStepFrames)
	}
	return fmt.Errorf("%w: Bill's PC did not close (screen %q)", errGSSecondBadgeStalled, gsPCText(m, profile))
}

// gsBackToBillsPCMenu leaves a transfer list for the Bill's PC menu.
func gsBackToBillsPCMenu(m *emu.Emu, profile *gsprofile.Profile) error {
	for i := 0; i < 10 && !gsBillsPCMenuUp(m, profile); i++ {
		m.Tap(emu.B, gsPCMenuHold, 7)
		gsPCWait(m, func() bool { return gsBillsPCMenuUp(m, profile) })
	}
	if !gsBillsPCMenuUp(m, profile) {
		return fmt.Errorf("%w: did not return to Bill's PC menu (screen %q)", errGSSecondBadgeStalled, gsPCText(m, profile))
	}
	return nil
}

// gsPCOpenList selects WITHDRAW or DEPOSIT and waits for its list.
func gsPCOpenList(m *emu.Emu, profile *gsprofile.Profile, row int, loaded byte) error {
	if err := skill.SelectMenuItem(m, row); err != nil {
		return fmt.Errorf("select PC row %d: %w", row, err)
	}
	if !gsPCWait(m, func() bool { return gsPCListUp(m, profile, loaded) }) {
		return fmt.Errorf("%w: PC transfer list did not open (screen %q)", errGSSecondBadgeStalled, gsPCText(m, profile))
	}
	return nil
}

// gsPCTransfer moves the open list's cursor to index and confirms the
// default submenu action (DEPOSIT or WITHDRAW). It returns once the party
// count moved by delta; a refusal box is a typed refusal.
func gsPCTransfer(m *emu.Emu, profile *gsprofile.Profile, loaded byte, index int, species uint8, delta int) error {
	if list := gsPCList(m); index < 0 || index >= len(list) || list[index] != species {
		return fmt.Errorf("%w: PC list %x has no species %#02x at %d", errGSSecondBadgeUnexpectedState, gsPCList(m), species, index)
	}
	for moves := 0; ; moves++ {
		pos := int(m.Peek8(sym.BillsPCScroll)) + int(m.Peek8(sym.BillsPCCursor))
		if pos == index {
			break
		}
		if moves >= gsPCMaxListMoves {
			return fmt.Errorf("%w: PC cursor stuck at %d, want %d", errGSSecondBadgeStalled, pos, index)
		}
		btn := emu.Down
		if pos > index {
			btn = emu.Up
		}
		m.Tap(btn, gsPCMenuHold, 7)
		m.StepFrames(gsPCStepFrames)
	}
	before := len(profile.DecodeCapture(m).PartySpecies)
	m.Tap(emu.A, gsPCMenuHold, 7)
	if !gsPCWait(m, func() bool {
		return m.Peek8(sym.JumptableIndex) == gsPCStateSubmenu && strings.Contains(gsPCText(m, profile), "What")
	}) {
		return fmt.Errorf("%w: PC submenu did not open (screen %q)", errGSSecondBadgeStalled, gsPCText(m, profile))
	}
	m.StepFrames(gsPCStepFrames) // the submenu draws before it polls input
	m.Tap(emu.A, gsPCMenuHold, 7)
	var refusal string
	if !gsPCWait(m, func() bool {
		text := gsPCText(m, profile)
		for _, refused := range []string{"BOX is full", "party's full", "last ", "No more usable", "Remove MAIL"} {
			if strings.Contains(text, refused) {
				refusal = refused
				return true
			}
		}
		return len(profile.DecodeCapture(m).PartySpecies) == before+delta
	}) {
		return fmt.Errorf("%w: PC transfer of %#02x did not change the party (screen %q)", errGSSecondBadgeStalled, species, gsPCText(m, profile))
	}
	if refusal != "" {
		return fmt.Errorf("%w: %q", errGSPCRefused, refusal)
	}
	return nil
}

// gsDepositSlot picks the surplus party member to store: never the lead or a
// learned carrier the caller keeps, otherwise the lowest level.
func gsDepositSlot(levels []int, keep map[int]bool) (int, bool) {
	best := -1
	for slot := 1; slot < len(levels); slot++ {
		if keep[slot] {
			continue
		}
		if best < 0 || levels[slot] < levels[best] {
			best = slot
		}
	}
	return best, best >= 0
}

// gsTraversalFieldMoves are the field moves the route graph depends on.
// Utility moves (Flash, Headbutt, Rock Smash) do not protect a carrier.
var gsTraversalFieldMoves = []game.FieldMoveID{
	game.FieldMoveCut, game.FieldMoveFly, game.FieldMoveSurf,
	game.FieldMoveStrength, game.FieldMoveWhirlpool, game.FieldMoveWaterfall,
}

// gsFieldCarrierSlots are the party slots currently carrying a usable
// traversal move; depositing one would break a capability the run has.
func gsFieldCarrierSlots(m *emu.Emu, romData []byte, profile *gsprofile.Profile) map[int]bool {
	keep := map[int]bool{}
	for _, id := range gsTraversalFieldMoves {
		if capability, ok, err := profile.DecodeFieldMoveCapability(m, romData, id); err == nil && ok && capability.Usable {
			keep[capability.PartySlot] = true
		}
	}
	return keep
}

// gsPrepareCarrierRoster uses Bill's PC so a field-move carrier can join the
// party: a full party stores its surplus member first, then a boxed species
// that can learn id is withdrawn. It reports whether a carrier was withdrawn;
// otherwise the party has room for a catch.
func gsPrepareCarrierRoster(m *emu.Emu, romData []byte, profile *gsprofile.Profile, id game.FieldMoveID) (bool, error) {
	if err := gsOpenBillsPC(m, romData, profile); err != nil {
		return false, err
	}
	withdrew, err := gsPrepareCarrierRosterOpen(m, romData, profile, id)
	if closeErr := gsCloseBillsPC(m, profile); err == nil {
		err = closeErr
	}
	return withdrew, err
}

func gsPrepareCarrierRosterOpen(m *emu.Emu, romData []byte, profile *gsprofile.Profile, id game.FieldMoveID) (bool, error) {
	obs, err := profile.DecodeObservation(m, romData)
	if err != nil {
		return false, err
	}
	if len(obs.Party) >= 6 {
		levels := make([]int, len(obs.Party))
		for i, mon := range obs.Party {
			levels[i] = int(mon.Level)
		}
		slot, ok := gsDepositSlot(levels, gsFieldCarrierSlots(m, romData, profile))
		if !ok {
			return false, fmt.Errorf("%w: no surplus party member to store", errGSPCRefused)
		}
		if err := gsPCOpenList(m, profile, gsPCRowDeposit, sym.BillsPCLoadedParty); err != nil {
			return false, err
		}
		species := profile.DecodeCapture(m).PartySpecies[slot]
		if err := gsPCTransfer(m, profile, sym.BillsPCLoadedParty, slot, uint8(species), -1); err != nil {
			return false, fmt.Errorf("store party slot %d: %w", slot, err)
		}
		if err := gsBackToBillsPCMenu(m, profile); err != nil {
			return false, err
		}
	}

	if err := gsPCOpenList(m, profile, gsPCRowWithdraw, sym.BillsPCLoadedActive); err != nil {
		return false, err
	}
	for index, species := range gsPCList(m) {
		ok, err := profile.SpeciesCanLearnFieldMove(romData, species, id)
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}
		if err := gsPCTransfer(m, profile, sym.BillsPCLoadedActive, index, species, +1); err != nil {
			return false, fmt.Errorf("withdraw boxed %s carrier %#02x: %w", id, species, err)
		}
		party := profile.DecodeCapture(m).PartySpecies
		if party[len(party)-1] != uint16(species) {
			return false, fmt.Errorf("%w: withdrew %#02x but party ends with %#02x", errGSSecondBadgeUnexpectedState, species, party[len(party)-1])
		}
		return true, nil
	}
	return false, nil
}
