package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

var (
	// ErrNotTrainer reports that a requested map object is not a trainer object.
	ErrNotTrainer = errors.New("skill: object is not a trainer")
	// ErrUnsupportedTrainer reports a trainer object whose text script is not
	// the standard TalkToTrainer stub. Higher-level story skills own those
	// bespoke script triggers and postconditions.
	ErrUnsupportedTrainer = errors.New("skill: trainer does not use the standard trainer header interaction")
)

// TrainerStatus is the reusable observation-side state for a trainer object.
// Challengeable means it is an ordinary trainer that the generic objective may
// safely offer. Story bosses can still be passed explicitly to
// ChallengeTrainer by their owning story skill, which must verify its extra
// postconditions after the fought flag is committed.
type TrainerStatus struct {
	Defeated      bool
	Challengeable bool
}

// trainerFlagRef is the exact RAM bit a trainer header uses as its defeated
// postcondition. Gen 1 trainer headers carry a bit offset plus a pointer to the
// start of the relevant event-bit field. FlagAction adds bit/8 to that pointer,
// so a header with CURRENT_TRAINER_BIT >= 8 must advance the byte address too.
type trainerFlagRef struct {
	addr uint16
	mask uint8
}

func (r trainerFlagRef) set(m *emu.Emu) bool { return m.Peek8(r.addr)&r.mask != 0 }
func (r trainerFlagRef) setMem(m *state.Mem) bool {
	return m.U8(r.addr)&r.mask != 0
}

// decodeTrainerFlagRef decodes a trainer header from ROM. The header stores:
//
//	+0 current trainer bit offset
//	+1 sight range / facing metadata
//	+2 little-endian pointer to the base event-flag byte
//
// FlagAction treats the first byte as an arbitrary bit offset, not just 0..7:
// it advances HL by bit/8 before applying 1<<(bit%8). Route 3 reaches trainer
// bits 8 and 9, so dropping the byte carry would verify the wrong trainer.
func decodeTrainerFlagRef(romData []byte, bank uint8, ptr uint16) (trainerFlagRef, error) {
	off, err := trainerROMOffset(romData, bank, ptr)
	if err != nil {
		return trainerFlagRef{}, err
	}
	if off+4 > len(romData) {
		return trainerFlagRef{}, fmt.Errorf("skill: trainer header %#04x in bank %d exceeds ROM", ptr, bank)
	}
	bit := romData[off]
	base := uint16(romData[off+2]) | uint16(romData[off+3])<<8
	addr := base + uint16(bit)/8
	if base < sym.EventFlags || addr >= sym.EventFlags+0x200 {
		return trainerFlagRef{}, fmt.Errorf("skill: trainer header event pointer %#04x + bit %d is outside wEventFlags", base, bit)
	}
	return trainerFlagRef{addr: addr, mask: uint8(1 << (bit & 7))}, nil
}

func trainerROMOffset(romData []byte, bank uint8, ptr uint16) (int, error) {
	var off int
	switch {
	case ptr < 0x4000:
		if bank != 0 {
			return 0, fmt.Errorf("skill: trainer pointer %#04x is outside bank %d", ptr, bank)
		}
		off = int(ptr)
	default:
		off = int(bank)*0x4000 + int(ptr-0x4000)
	}
	if off < 0 || off >= len(romData) {
		return 0, fmt.Errorf("skill: trainer pointer %#04x in bank %d exceeds ROM", ptr, bank)
	}
	return off, nil
}

type trainerTarget struct {
	objectID int
	object   rom.Object
	flag     trainerFlagRef
}

// trainerTargetAt resolves a trainer object to its standard trainer header
// without touching live UI state. Object text IDs are 1-based indexes into the
// map's text-pointer table. A standard trainer entry points at:
//
//	text_asm              ; $08
//	ld hl, TrainerHeader  ; $21 lo hi
//	call TalkToTrainer
//
// Requiring that stub makes generic offering fail closed on custom story
// scripts instead of guessing that every object with the trainer bit has the
// ordinary TalkToTrainer contract.
func trainerTargetAt(romData []byte, h rom.MapHeader, homeX, homeY uint8) (trainerTarget, error) {
	var target trainerTarget
	for i, object := range h.Objects {
		if object.X != homeX || object.Y != homeY {
			continue
		}
		if object.TextID&0x40 == 0 {
			return trainerTarget{}, fmt.Errorf("skill: object at (%d,%d) on map %#04x: %w", homeX, homeY, h.ID, ErrNotTrainer)
		}
		target.objectID = i + 1
		target.object = object
		break
	}
	if target.objectID == 0 {
		return trainerTarget{}, fmt.Errorf("skill: no trainer object at (%d,%d) on map %#04x", homeX, homeY, h.ID)
	}

	textID := int(target.object.TextID & 0x3f)
	if textID == 0 {
		return trainerTarget{}, fmt.Errorf("skill: trainer at (%d,%d) on map %#04x: %w: zero text id", homeX, homeY, h.ID, ErrUnsupportedTrainer)
	}
	tableOff, err := trainerROMOffset(romData, h.Bank, h.TextsAddr)
	if err != nil {
		return trainerTarget{}, fmt.Errorf("skill: trainer text table: %w", err)
	}
	entry := tableOff + (textID-1)*2
	if entry < 0 || entry+2 > len(romData) {
		return trainerTarget{}, fmt.Errorf("skill: trainer text pointer %d exceeds ROM", textID)
	}
	textPtr := uint16(romData[entry]) | uint16(romData[entry+1])<<8
	textOff, err := trainerROMOffset(romData, h.Bank, textPtr)
	if err != nil {
		return trainerTarget{}, fmt.Errorf("skill: trainer text %d: %w", textID, err)
	}
	if textOff+4 > len(romData) || romData[textOff] != 0x08 || romData[textOff+1] != 0x21 {
		return trainerTarget{}, fmt.Errorf("skill: trainer at (%d,%d) on map %#04x: %w", homeX, homeY, h.ID, ErrUnsupportedTrainer)
	}
	headerPtr := uint16(romData[textOff+2]) | uint16(romData[textOff+3])<<8
	flag, err := decodeTrainerFlagRef(romData, h.Bank, headerPtr)
	if err != nil {
		return trainerTarget{}, fmt.Errorf("skill: trainer header at (%d,%d): %w", homeX, homeY, err)
	}
	target.flag = flag
	return target, nil
}

// ordinaryTrainerClass excludes trainer classes whose win is only one part of
// a larger story contract: rivals, Giovanni, gym leaders, Elite Four/Champion,
// and unused boss classes. Their owning story skills may call ChallengeTrainer
// directly, but the portable Offer menu must not present the fought flag alone
// as completion of those encounters.
func ordinaryTrainerClass(opponent uint8) bool {
	const opponentOffset = 200
	if opponent < opponentOffset {
		return false // wild/special Pokemon objects such as Mewtwo
	}
	class := opponent - opponentOffset
	switch class {
	case 0x00, // NOBODY
		0x19,                                     // RIVAL1
		0x1a,                                     // PROF_OAK
		0x1b,                                     // CHIEF
		0x1d,                                     // GIOVANNI
		0x21,                                     // BRUNO
		0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, // gym leaders
		0x2a, 0x2b, // RIVAL2 / RIVAL3
		0x2c, // LORELEI
		0x2e, // AGATHA
		0x2f: // LANCE
		return false
	default:
		return true
	}
}

// TrainerStatusAt reports whether a standard trainer object is already
// defeated and whether it is safe to expose as a generic challenge. It is a
// pure ROM+RAM observation: no movement or button press occurs.
func TrainerStatusAt(romData []byte, mem *state.Mem, mapID, homeX, homeY uint8) (TrainerStatus, error) {
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return TrainerStatus{}, fmt.Errorf("skill: TrainerStatusAt: parse map %#04x: %w", mapID, err)
	}
	target, err := trainerTargetAt(romData, h, homeX, homeY)
	if err != nil {
		return TrainerStatus{}, err
	}
	return TrainerStatus{
		Defeated:      target.flag.setMem(mem),
		Challengeable: ordinaryTrainerClass(target.object.TrainerClass),
	}, nil
}

// ChallengeTrainer deliberately defeats the trainer whose ROM home coordinate
// is (homeX,homeY) on the current map. It supports both encounter forms Red
// uses for ordinary map trainers:
//
//   - line-of-sight: the approach may itself trigger the target; talkBeside's
//     TravelFlee path delegates the unavoidable trainer battle to Battle.
//   - explicit interaction: once adjacent, press A, advance the before-battle
//     text, and delegate combat to Battle here.
//
// The exact target flag is resolved before moving. This matters when another
// trainer intercepts the approach: winning that battle is not success unless
// the requested trainer's own flag changed. Already-defeated trainers return
// success without moving. A loss preserves ErrTrainerBlackedOut, so the
// existing trainer-wall recovery asks for training instead of treating the
// failure as an unrelated blackout.
//
// Boss/story interactions whose completion is more than the trainer fought
// flag still belong in a higher-level story skill. Such a skill may reuse this
// fight primitive and then assert its badge/item/warp/story postcondition.
func ChallengeTrainer(m *emu.Emu, romData []byte, homeX, homeY uint8, policy MovePolicy) error {
	if policy == nil {
		return errors.New("skill: ChallengeTrainer: nil policy")
	}
	cur := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return fmt.Errorf("skill: ChallengeTrainer: parse map %#04x: %w", cur, err)
	}
	target, err := trainerTargetAt(romData, h, homeX, homeY)
	if err != nil {
		return fmt.Errorf("skill: ChallengeTrainer: %w", err)
	}
	if target.flag.set(m) {
		return nil
	}

	// Walking beside a sight trainer can itself be the challenge. TravelFlee
	// fights trainer interruptions and preserves ErrTrainerBlackedOut on loss.
	// After it returns, only the requested trainer's flag can prove that this
	// approach already completed the objective.
	if err := talkBeside(m, romData, homeX, homeY, policy); err != nil {
		return fmt.Errorf("skill: ChallengeTrainer: approach trainer at (%d,%d): %w", homeX, homeY, err)
	}
	if target.flag.set(m) {
		return nil
	}

	tx, ty, live := liveObjectPosition(m, target.objectID)
	if !live {
		return fmt.Errorf("skill: ChallengeTrainer: trainer sprite %d disappeared but fought flag %#04x/%#02x is clear", target.objectID, target.flag.addr, target.flag.mask)
	}
	if err := Face(m, tx, ty); err != nil {
		return fmt.Errorf("skill: ChallengeTrainer: face trainer at (%d,%d): %w", tx, ty, err)
	}

	m.Tap(emu.A, 3, 7)
	var mem state.Mem
	if _, err := m.StepUntil(talkOpenBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return mem.U8(sym.FontLoaded) != 0 || state.DecodeBattle(&mem) != nil
	}); err != nil {
		return fmt.Errorf("skill: ChallengeTrainer: interaction at (%d,%d) opened neither dialogue nor battle", tx, ty)
	}

	mem = advanceUntil(m, gymBattleWaitBudget, func(mm *state.Mem) bool {
		return state.DecodeBattle(mm) != nil || target.flag.setMem(mm) || (mm.U8(sym.FontLoaded) == 0 && state.Controllable(mm))
	})
	if target.flag.setMem(&mem) {
		return nil
	}
	if state.DecodeBattle(&mem) == nil {
		if !state.Controllable(&mem) {
			return fmt.Errorf("skill: ChallengeTrainer: trainer dialogue neither started a battle nor returned control")
		}
		return fmt.Errorf("skill: ChallengeTrainer: trainer returned control without battle but fought flag %#04x/%#02x is clear", target.flag.addr, target.flag.mask)
	}

	outcome, err := Battle(m, policy)
	if err != nil {
		return fmt.Errorf("skill: ChallengeTrainer: battle: %w", err)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("skill: ChallengeTrainer: %w at (%d,%d)", ErrTrainerBlackedOut, homeX, homeY)
	}

	mem = advanceUntil(m, storyBattleSettleBudget, func(mm *state.Mem) bool {
		return target.flag.setMem(mm) && state.Controllable(mm)
	})
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: ChallengeTrainer: not controllable after winning trainer battle")
	}
	if !target.flag.setMem(&mem) {
		return fmt.Errorf("skill: ChallengeTrainer: battle won but fought flag %#04x/%#02x was not committed", target.flag.addr, target.flag.mask)
	}
	return nil
}
