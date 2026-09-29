package skill

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/gs/sym"
)

func init() {
	registerFieldMoveTeachingAdapter(fieldMoveTeachingAdapter{
		match: func(profile game.FieldMoveProfile) bool {
			_, ok := profile.(*gsprofile.Profile)
			return ok
		},
		teach: func(m *emu.Emu, profile game.FieldMoveProfile, native game.NativeFieldMove) error {
			p, ok := profile.(*gsprofile.Profile)
			if !ok {
				return fmt.Errorf("skill: gs TM/HM adapter received %T", profile)
			}
			return teachGSTMHM(m, p, native)
		},
	})
}

func gsMachineIndex(item uint16) (int, bool) {
	for i, raw := range gsdata.MachineItems {
		if uint16(raw) == item {
			return i, true
		}
	}
	return -1, false
}

func gsPartyMoves(m game.MemoryReader, slot int) [4]byte {
	var out [4]byte
	if slot < 0 || slot >= 6 {
		return out
	}
	base := sym.PartyMon1 + uint16(slot)*sym.PartyMonSize + 2
	for i := range out {
		out[i] = m.Peek8(base + uint16(i))
	}
	return out
}

func gsKnowsMove(m game.MemoryReader, slot int, move byte) bool {
	for _, known := range gsPartyMoves(m, slot) {
		if known == move {
			return true
		}
	}
	return false
}

func gsReplacementSlot(m game.MemoryReader, slot int) (int, error) {
	moves := gsPartyMoves(m, slot)
	for i, move := range moves {
		if move == 0 {
			return -1, nil
		}
	}
	protected := map[byte]bool{
		0x0f: true, 0x13: true, 0x39: true, 0x46: true,
		0x94: true, 0xfa: true, 0x7f: true, 0x1d: true,
	}
	for i, move := range moves {
		if !protected[move] {
			return i, nil
		}
	}
	return -1, fmt.Errorf("all four moves are retained field capabilities")
}

func gsCloseTeachingUI(m *emu.Emu, p *gsprofile.Profile) error {
	for i := 0; i < 80; i++ {
		state := p.DecodeFieldAction(m)
		if state.Controllable && !state.ResultTextActive {
			return nil
		}
		m.Tap(emu.B, 3, 7)
		m.StepFrames(10)
	}
	return fmt.Errorf("Gen-II teaching UI did not close to the overworld")
}

func teachGSTMHM(m *emu.Emu, p *gsprofile.Profile, native game.NativeFieldMove) (retErr error) {
	if m == nil || p == nil {
		return fmt.Errorf("skill: Gen-II TM/HM: nil runtime/profile")
	}
	if native.MoveID == 0 || native.MoveID > 0xff {
		return fmt.Errorf("skill: Gen-II TM/HM: unsupported move id %#04x", native.MoveID)
	}
	machineIndex, ok := gsMachineIndex(native.MachineItemID)
	if !ok || machineIndex >= sym.TMsHMsCount {
		return fmt.Errorf("skill: Gen-II TM/HM: unknown machine item %#04x", native.MachineItemID)
	}
	beforeQty := int(m.Peek8(sym.TMsHMs + uint16(machineIndex)))
	if beforeQty == 0 {
		return fmt.Errorf("skill: Gen-II TM/HM: machine %#04x is not owned", native.MachineItemID)
	}

	menuMayBeOpen := false
	defer func() {
		if retErr == nil || !menuMayBeOpen {
			return
		}
		if err := gsCloseTeachingUI(m, p); err != nil {
			retErr = fmt.Errorf("%w; cleanup: %v", retErr, err)
		}
	}()

	menuMayBeOpen = true
	if err := openStartMenuEntryWithDecoder(m, p, startMenuItems); err != nil {
		return fmt.Errorf("skill: Gen-II TM/HM: open PACK: %w", err)
	}
	m.StepFrames(60)

	lastPocket := int(m.Peek8(sym.LastPocket) & 3)
	for turns := (3 - lastPocket + 4) % 4; turns > 0; turns-- {
		m.Tap(emu.Right, 3, 7)
		m.StepFrames(30)
	}

	rank := 0
	for i := 0; i < machineIndex; i++ {
		if m.Peek8(sym.TMsHMs+uint16(i)) != 0 {
			rank++
		}
	}
	for attempts := 0; attempts < 80; attempts++ {
		current := int(m.Peek8(sym.TMHMPocketScroll)) + int(m.Peek8(sym.TMHMPocketCursor))
		if current == rank {
			break
		}
		if current < rank {
			m.Tap(emu.Down, 3, 7)
		} else {
			m.Tap(emu.Up, 3, 7)
		}
		m.StepFrames(15)
		if attempts == 79 {
			return fmt.Errorf("skill: Gen-II TM/HM: machine cursor did not reach owned rank %d", rank)
		}
	}
	m.Tap(emu.A, 3, 7)

	if !waitMenuUntil(m, 300, func() bool {
		_, open := p.DecodeTwoOption(m)
		return open
	}) {
		return fmt.Errorf("skill: Gen-II TM/HM: USE/QUIT menu did not appear")
	}
	if err := selectTwoOptionWithDecoder(m, p, 0); err != nil {
		return fmt.Errorf("skill: Gen-II TM/HM: choose USE: %w", err)
	}

	if !waitMenuUntil(m, 1200, func() bool {
		if _, open := p.DecodeTwoOption(m); open {
			return true
		}
		m.Tap(emu.A, 3, 7)
		return false
	}) {
		return fmt.Errorf("skill: Gen-II TM/HM: teach confirmation did not appear")
	}
	if err := selectTwoOptionWithDecoder(m, p, 0); err != nil {
		return fmt.Errorf("skill: Gen-II TM/HM: confirm teaching: %w", err)
	}

	if !waitMenuUntil(m, 1000, func() bool {
		s := p.DecodePartyMenu(m)
		return s.Visible && s.Kind == game.PartyMenuTMHMTeach
	}) {
		return fmt.Errorf("skill: Gen-II TM/HM: compatibility party menu did not appear")
	}
	compatible := p.TMHMCompatiblePartySlots(m)
	if len(compatible) == 0 {
		return fmt.Errorf("%w: no current party member is ABLE to learn move %#02x", ErrFieldMovePrerequisite, native.MoveID)
	}
	carrier := compatible[0]
	if native.MoveID == 0x0f || native.MoveID == 0x94 {
		for _, slot := range compatible {
			if slot > 0 {
				carrier = slot
				break
			}
		}
	}
	replaceSlot, err := gsReplacementSlot(m, carrier)
	if err != nil {
		return fmt.Errorf("skill: Gen-II TM/HM: party slot %d: %w", carrier, err)
	}
	if err := selectPartySlotWithDecoder(m, p, carrier); err != nil {
		return fmt.Errorf("skill: Gen-II TM/HM: select compatible party slot %d: %w", carrier, err)
	}

	for spent := 0; spent < 3000; spent += 20 {
		if gsKnowsMove(m, carrier, byte(native.MoveID)) {
			break
		}
		if _, open := p.DecodeTwoOption(m); open {
			if err := selectTwoOptionWithDecoder(m, p, 0); err != nil {
				return fmt.Errorf("skill: Gen-II TM/HM: answer replacement prompt: %w", err)
			}
			continue
		}
		text := p.ScreenText(m)
		if replaceSlot >= 0 && (strings.Contains(text, "Which move") || strings.Contains(strings.ToLower(text), "forget")) {
			if err := selectMenuItemWithDecoder(m, p, replaceSlot); err != nil {
				return fmt.Errorf("skill: Gen-II TM/HM: choose move slot %d to forget: %w", replaceSlot, err)
			}
			continue
		}
		m.Tap(emu.A, 3, 7)
		m.StepFrames(20)
	}
	if !gsKnowsMove(m, carrier, byte(native.MoveID)) {
		return fmt.Errorf("skill: Gen-II TM/HM: move %#02x was not verified in party slot %d", native.MoveID, carrier)
	}
	if err := gsCloseTeachingUI(m, p); err != nil {
		return fmt.Errorf("skill: Gen-II TM/HM: close menus: %w", err)
	}
	menuMayBeOpen = false

	afterQty := int(m.Peek8(sym.TMsHMs + uint16(machineIndex)))
	if machineIndex < 50 {
		if afterQty != beforeQty-1 {
			return fmt.Errorf("skill: Gen-II TM/HM: TM quantity %d->%d, want one consumed", beforeQty, afterQty)
		}
	} else if afterQty != beforeQty {
		return fmt.Errorf("skill: Gen-II TM/HM: HM quantity changed %d->%d", beforeQty, afterQty)
	}
	return nil
}
