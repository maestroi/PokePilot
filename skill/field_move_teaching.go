package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

type fieldMoveTeachingAdapter struct {
	match func(game.FieldMoveProfile) bool
	teach func(*emu.Emu, game.FieldMoveProfile, game.NativeFieldMove) error
}

var fieldMoveTeachingAdapters []fieldMoveTeachingAdapter

func registerFieldMoveTeachingAdapter(adapter fieldMoveTeachingAdapter) {
	fieldMoveTeachingAdapters = append(fieldMoveTeachingAdapters, adapter)
}

func teachNativeFieldMove(m *emu.Emu, profile game.FieldMoveProfile, native game.NativeFieldMove) error {
	for _, adapter := range fieldMoveTeachingAdapters {
		if adapter.match != nil && adapter.match(profile) {
			return adapter.teach(m, profile, native)
		}
	}
	// Preserve the existing Gen-I execution path as the compatibility fallback.
	if native.MachineItemID == 0 || native.MachineItemID > 0xff || native.MoveID == 0 || native.MoveID > 0xff {
		return fmt.Errorf("native machine/item ids %#04x/%#04x exceed legacy move-learning executor range",
			native.MachineItemID, native.MoveID)
	}
	result, err := TeachTMHM(m, uint8(native.MachineItemID), true)
	if err != nil {
		return err
	}
	if uint16(result.Decision.Machine.Move) != native.MoveID {
		return fmt.Errorf("machine %#04x mapped to move %#04x, want %#04x",
			native.MachineItemID, result.Decision.Machine.Move, native.MoveID)
	}
	return nil
}
