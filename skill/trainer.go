package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// ErrNotTrainer reports that a requested map object is not a trainer object.
var ErrNotTrainer = errors.New("skill: object is not a trainer")

// trainerFlagRef is the exact RAM bit the active trainer header uses as its
// defeated postcondition. Gen 1 trainer headers carry both the bit number and
// a pointer into wEventFlags; EndTrainerBattle sets this bit only after a
// non-loss. Capturing this reference before Battle lets ChallengeTrainer prove
// the ROM committed the win instead of treating ResultWon as sufficient.
type trainerFlagRef struct {
	addr uint16
	mask uint8
}

func (r trainerFlagRef) set(m *emu.Emu) bool { return m.Peek8(r.addr)&r.mask != 0 }

// decodeTrainerFlagRef decodes the active trainer header from ROM. The header
// pointer is a Game Boy banked address; trainer headers store:
//
//	+0 current trainer bit
//	+1 sight range
//	+2 little-endian pointer to the event-flag byte
//
// See pokered/macros/scripts/maps.asm: trainer.
func decodeTrainerFlagRef(romData []byte, bank uint8, ptr uint16) (trainerFlagRef, error) {
	var off int
	switch {
	case ptr < 0x4000:
		if bank != 0 {
			return trainerFlagRef{}, fmt.Errorf("skill: trainer header pointer %#04x is outside bank %d", ptr, bank)
		}
		off = int(ptr)
	default:
		off = int(bank)*0x4000 + int(ptr-0x4000)
	}
	if off < 0 || off+4 > len(romData) {
		return trainerFlagRef{}, fmt.Errorf("skill: trainer header %#04x in bank %d exceeds ROM", ptr, bank)
	}
	bit := romData[off]
	addr := uint16(romData[off+2]) | uint16(romData[off+3])<<8
	if addr < sym.EventFlags || addr >= sym.EventFlags+0x200 {
		return trainerFlagRef{}, fmt.Errorf("skill: trainer header event pointer %#04x is outside wEventFlags", addr)
	}
	return trainerFlagRef{addr: addr, mask: uint8(1 << (bit & 7))}, nil
}

func activeTrainerFlagRef(m *emu.Emu, romData []byte, bank uint8) (trainerFlagRef, error) {
	// StoreTrainerHeaderPointer writes H then L, unlike normal little-endian
	// RAM words, so read the two bytes explicitly.
	ptr := uint16(m.Peek8(sym.TrainerHeaderPtr))<<8 | uint16(m.Peek8(sym.TrainerHeaderPtr+1))
	if ptr == 0 {
		return trainerFlagRef{}, errors.New("skill: active trainer header pointer is zero")
	}
	return decodeTrainerFlagRef(romData, bank, ptr)
}

// ChallengeTrainer deliberately defeats the trainer whose ROM home coordinate
// is (homeX,homeY) on the current map. It supports both encounter forms Red
// uses for ordinary map trainers:
//
//   - line-of-sight: the approach may itself trigger the trainer; talkBeside's
//     TravelFlee path delegates the unavoidable trainer battle to Battle.
//   - explicit interaction: once adjacent, press A, advance the before-battle
//     text, and delegate combat to Battle here.
//
// The objective is idempotent. A defeated trainer either has been hidden by
// the map script, or answers with after-battle dialogue; in the latter case we
// decode that trainer's live header and require its fought flag to already be
// set. After a new win, the same exact flag must be set before success is
// returned. A loss is reported through ErrBlackedOut so the run's existing
// trainer-wall recovery semantics remain intact.
//
// This primitive is intentionally for ordinary trainer-header encounters.
// Boss/story interactions whose completion is more than the trainer fought
// flag (badge, key item, warp, story event, etc.) still belong in a higher-level
// story skill which may reuse ChallengeTrainer for the fight itself.
func ChallengeTrainer(m *emu.Emu, romData []byte, homeX, homeY uint8, policy MovePolicy) error {
	if policy == nil {
		return errors.New("skill: ChallengeTrainer: nil policy")
	}
	cur := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return fmt.Errorf("skill: ChallengeTrainer: parse map %#04x: %w", cur, err)
	}

	objectID := 0
	for i, object := range h.Objects {
		if object.X != homeX || object.Y != homeY {
			continue
		}
		if object.TextID&0x40 == 0 {
			return fmt.Errorf("skill: ChallengeTrainer: object at (%d,%d) on map %#04x: %w", homeX, homeY, cur, ErrNotTrainer)
		}
		objectID = i + 1
		break
	}
	if objectID == 0 {
		return fmt.Errorf("skill: ChallengeTrainer: no trainer object at (%d,%d) on map %#04x", homeX, homeY, cur)
	}

	// The walk is allowed to trigger a sight trainer. talkBeside already
	// resolves wild encounters by fleeing and trainer encounters by fighting;
	// any loss comes back wrapping ErrBlackedOut. If this target was defeated
	// on the walk and its sprite is hidden, that object-state transition is a
	// positive postcondition and there is nothing left to interact with.
	if err := talkBeside(m, romData, homeX, homeY, policy); err != nil {
		return fmt.Errorf("skill: ChallengeTrainer: approach trainer at (%d,%d): %w", homeX, homeY, err)
	}

	tx, ty, live := liveObjectPosition(m, objectID)
	if !live {
		return nil
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
		return state.DecodeBattle(mm) != nil || (mm.U8(sym.FontLoaded) == 0 && state.Controllable(mm))
	})

	flag, flagErr := activeTrainerFlagRef(m, romData, h.Bank)
	if state.DecodeBattle(&mem) == nil {
		if !state.Controllable(&mem) {
			return fmt.Errorf("skill: ChallengeTrainer: trainer dialogue neither started a battle nor returned control")
		}
		if flagErr != nil {
			return fmt.Errorf("skill: ChallengeTrainer: verify already-defeated trainer: %w", flagErr)
		}
		if !flag.set(m) {
			return fmt.Errorf("skill: ChallengeTrainer: trainer returned control without battle but fought flag %#04x/%#02x is clear", flag.addr, flag.mask)
		}
		return nil
	}
	if flagErr != nil {
		return fmt.Errorf("skill: ChallengeTrainer: capture trainer fought flag before battle: %w", flagErr)
	}

	outcome, err := Battle(m, policy)
	if err != nil {
		return fmt.Errorf("skill: ChallengeTrainer: battle: %w", err)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("skill: ChallengeTrainer: %w after losing to trainer at (%d,%d)", ErrBlackedOut, homeX, homeY)
	}

	mem = advanceUntil(m, storyBattleSettleBudget, func(mm *state.Mem) bool {
		return state.Controllable(mm)
	})
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: ChallengeTrainer: not controllable after winning trainer battle")
	}
	if !flag.set(m) {
		return fmt.Errorf("skill: ChallengeTrainer: battle won but fought flag %#04x/%#02x was not committed", flag.addr, flag.mask)
	}
	return nil
}
