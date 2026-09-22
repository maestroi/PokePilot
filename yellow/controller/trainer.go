package controller

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

var (
	ErrNotYellowTrainer         = errors.New("yellow trainer: object is not a trainer")
	ErrUnsupportedYellowTrainer = errors.New("yellow trainer: custom/story trainer is not a standard trainer-header interaction")
)

type TrainerStatus struct {
	Defeated      bool
	Challengeable bool
}

type yellowTrainerFlagRef struct {
	addr uint16
	mask uint8
}

func (r yellowTrainerFlagRef) set(m *emu.Emu) bool {
	return m != nil && m.Peek8(r.addr)&r.mask != 0
}

type yellowTrainerTarget struct {
	objectID int
	object   yellowrom.Object
	flag     yellowTrainerFlagRef
}

func yellowTrainerROMOffset(romData []byte, bank uint8, ptr uint16) (int, error) {
	var off int
	switch {
	case ptr < 0x4000:
		if bank != 0 {
			return 0, fmt.Errorf("trainer pointer %#04x is outside fixed bank", ptr)
		}
		off = int(ptr)
	default:
		off = int(bank)*0x4000 + int(ptr-0x4000)
	}
	if off < 0 || off >= len(romData) {
		return 0, fmt.Errorf("trainer pointer %#04x bank %d exceeds ROM", ptr, bank)
	}
	return off, nil
}

func decodeYellowTrainerFlagRef(romData []byte, bank uint8, ptr uint16) (yellowTrainerFlagRef, error) {
	off, err := yellowTrainerROMOffset(romData, bank, ptr)
	if err != nil {
		return yellowTrainerFlagRef{}, err
	}
	if off+4 > len(romData) {
		return yellowTrainerFlagRef{}, fmt.Errorf("trainer header %#04x bank %d exceeds ROM", ptr, bank)
	}
	bit := romData[off]
	base := uint16(romData[off+2]) | uint16(romData[off+3])<<8
	addr := base + uint16(bit)/8
	if base < sym.EventFlags || addr >= sym.EventFlags+0x200 {
		return yellowTrainerFlagRef{}, fmt.Errorf("trainer event pointer %#04x + bit %d outside Yellow event flags", base, bit)
	}
	return yellowTrainerFlagRef{addr: addr, mask: 1 << (bit & 7)}, nil
}

func yellowTrainerTargetAt(romData []byte, h yellowrom.MapHeader, homeX, homeY uint8) (yellowTrainerTarget, error) {
	var target yellowTrainerTarget
	for i, object := range h.Objects {
		if object.X != homeX || object.Y != homeY {
			continue
		}
		if object.TextID&0x40 == 0 {
			return yellowTrainerTarget{}, fmt.Errorf("object at (%d,%d) on map %#02x: %w", homeX, homeY, h.ID, ErrNotYellowTrainer)
		}
		target.objectID = i + 1
		target.object = object
		break
	}
	if target.objectID == 0 {
		return yellowTrainerTarget{}, fmt.Errorf("no trainer object at (%d,%d) on map %#02x", homeX, homeY, h.ID)
	}

	textID := int(target.object.TextID & 0x3f)
	if textID == 0 {
		return yellowTrainerTarget{}, fmt.Errorf("%w: zero text id", ErrUnsupportedYellowTrainer)
	}
	tableOff, err := yellowTrainerROMOffset(romData, h.Bank, h.TextsAddr)
	if err != nil {
		return yellowTrainerTarget{}, err
	}
	entry := tableOff + (textID-1)*2
	if entry < 0 || entry+2 > len(romData) {
		return yellowTrainerTarget{}, fmt.Errorf("trainer text pointer %d exceeds ROM", textID)
	}
	textPtr := uint16(romData[entry]) | uint16(romData[entry+1])<<8
	textOff, err := yellowTrainerROMOffset(romData, h.Bank, textPtr)
	if err != nil {
		return yellowTrainerTarget{}, err
	}
	// Standard TalkToTrainer stub: text_asm ($08), ld hl, TrainerHeader ($21 lo hi).
	if textOff+4 > len(romData) || romData[textOff] != 0x08 || romData[textOff+1] != 0x21 {
		return yellowTrainerTarget{}, fmt.Errorf("trainer at (%d,%d) on map %#02x: %w", homeX, homeY, h.ID, ErrUnsupportedYellowTrainer)
	}
	headerPtr := uint16(romData[textOff+2]) | uint16(romData[textOff+3])<<8
	flag, err := decodeYellowTrainerFlagRef(romData, h.Bank, headerPtr)
	if err != nil {
		return yellowTrainerTarget{}, err
	}
	target.flag = flag
	return target, nil
}

func ordinaryYellowTrainerClass(class uint8) bool {
	switch class {
	case 0x00, 0x19, 0x1a, 0x1b, 0x1d, 0x21,
		0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28,
		0x2a, 0x2b, 0x2c, 0x2e, 0x2f:
		return false
	default:
		return true
	}
}

func yellowTrainerReachable(romData []byte, m *emu.Emu, h yellowrom.MapHeader, x, y uint8) bool {
	if m == nil || m.Peek8(sym.CurMap) != h.ID {
		return true
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return true
	}
	target := [2]int{int(x), int(y)}
	blocked := staticObjectBlockers(h, &target)
	_, _, err = world.FindPathAdjacent(grid,
		int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord)),
		int(x), int(y), blocked)
	return err == nil
}

func TrainerStatusAt(m *emu.Emu, romData []byte, mapID, homeX, homeY uint8) (TrainerStatus, error) {
	h, err := yellowrom.ParseMap(romData, mapID)
	if err != nil {
		return TrainerStatus{}, err
	}
	target, err := yellowTrainerTargetAt(romData, h, homeX, homeY)
	if err != nil {
		return TrainerStatus{}, err
	}
	return TrainerStatus{
		Defeated:      target.flag.set(m),
		Challengeable: ordinaryYellowTrainerClass(target.object.TrainerClass) && yellowTrainerReachable(romData, m, h, homeX, homeY),
	}, nil
}

// ChallengeTrainer defeats one standard Yellow trainer and verifies that the
// exact trainer-header flag is committed. Story bosses are deliberately
// rejected by the ordinary-object classifier and belong to story controllers.
func ChallengeTrainer(m *emu.Emu, romData []byte, homeX, homeY uint8) error {
	if m == nil {
		return fmt.Errorf("yellow trainer: nil emulator")
	}
	mapID := m.Peek8(sym.CurMap)
	h, err := yellowrom.ParseMap(romData, mapID)
	if err != nil {
		return fmt.Errorf("yellow trainer: parse map: %w", err)
	}
	target, err := yellowTrainerTargetAt(romData, h, homeX, homeY)
	if err != nil {
		return fmt.Errorf("yellow trainer: %w", err)
	}
	if target.flag.set(m) {
		return nil
	}

	// Approach may itself trigger a sight-line encounter. Treat a battle or
	// dialogue interruption as engagement with the target rather than a
	// pathfinding failure, but completion still requires this exact flag.
	if err := interactAt(m, romData, int(homeX), int(homeY)); err != nil {
		if m.Peek8(sym.IsInBattle) == 0 && m.Peek8(sym.FontLoaded) == 0 {
			return fmt.Errorf("yellow trainer: approach (%d,%d): %w", homeX, homeY, err)
		}
	}

	for frame := 0; frame < 1200; frame++ {
		if target.flag.set(m) {
			return waitYellowControllable(m, romData, 3000)
		}
		if m.Peek8(sym.IsInBattle) != 0 {
			outcome, err := Battle(m, romData)
			if err != nil {
				return fmt.Errorf("yellow trainer: battle: %w", err)
			}
			if outcome.Outcome == BattleOutcomeLost {
				return fmt.Errorf("yellow trainer: lost battle at (%d,%d)", homeX, homeY)
			}
			continue
		}
		if m.Peek8(sym.FontLoaded) != 0 || m.Peek8(sym.JoyIgnore) != 0 {
			if _, err := RecoverDialogue(m, romData); err != nil {
				return fmt.Errorf("yellow trainer: pre/post battle dialogue: %w", err)
			}
			continue
		}
		if target.flag.set(m) {
			break
		}
		m.StepFrame()
	}

	if !target.flag.set(m) {
		return fmt.Errorf("yellow trainer: requested trainer (%d,%d) completed no fought flag %#04x/%#02x",
			homeX, homeY, target.flag.addr, target.flag.mask)
	}
	return waitYellowControllable(m, romData, 3000)
}
