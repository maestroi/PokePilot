package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

// catchWanted preserves the existing Gen-I helper seam for fishing/water
// capture callers that still carry a Red snapshot. The generic target driver
// no longer consumes that snapshot; all live execution state comes from the
// active profile.
func catchWanted(
	m *emu.Emu,
	_ *state.Mem,
	profile game.CaptureProfile,
	want []uint8,
	wantNative, wantDex []uint16,
	policy MovePolicy,
	before game.CaptureState,
	res CatchResult,
	maxBalls int,
) (CatchResult, error) {
	exec, err := captureExecutionFor(m)
	if err != nil {
		return res, fmt.Errorf("skill: catch target: %w", err)
	}
	return catchWantedWithSemantics(m, profile, exec, want, wantNative, wantDex, policy, before, res, maxBalls)
}
