package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
)

// objectiveFrameBudget is the emergency guard for one synchronous objective.
// Most low-level waits are hundreds or thousands of frames and a journey is
// already bounded to 20 engagements; half a million frames leaves generous
// room for legitimate long travel/training while ensuring a broken inner loop
// returns control to Run instead of leaving a farm worker on one round forever.
const objectiveFrameBudget uint64 = 500_000

// redObjectiveAdapter is Pokémon Red's implementation of the portable
// objective transaction seam. It is intentionally still located in agent while
// the repository is migrated incrementally: all Red/emulator dependencies are
// concentrated here and in the existing Red skill/state implementation instead
// of being required by the lifecycle runtime itself.
type redObjectiveAdapter struct {
	m       *emu.Emu
	romData []byte
}

func newRedObjectiveAdapter(m *emu.Emu, romData []byte) *redObjectiveAdapter {
	return &redObjectiveAdapter{m: m, romData: romData}
}

func (a *redObjectiveAdapter) Observe() Observation {
	return Observe(a.m, a.romData)
}

func (a *redObjectiveAdapter) Validate(o Objective, _ Observation) error {
	return o.Validate()
}

func (a *redObjectiveAdapter) NormalizeBoundary() error {
	return normalizeObjectiveBoundary(a.m)
}

func (a *redObjectiveAdapter) ExecuteOwned(o Objective) (ObjectiveResult, error) {
	return executeRedOwned(a.m, a.romData, o)
}

func (a *redObjectiveAdapter) WithinObjectiveBudget(o Objective, fn func() error) error {
	deadline := a.m.FrameCount() + objectiveFrameBudget
	err := a.m.WithFrameDeadline(deadline, fn)
	if errors.Is(err, emu.ErrFrameDeadline) {
		return fmt.Errorf("agent: %s: objective frame watchdog: %w", o, err)
	}
	return err
}

func (a *redObjectiveAdapter) SettlePostcondition(o Objective) {
	settleObjectivePostcondition(a.m, o)
}

func (a *redObjectiveAdapter) VerifyPostcondition(o Objective, final Observation, _ ObjectiveResult) error {
	_, err := objectivePostcondition(o, final)
	return err
}

func (a *redObjectiveAdapter) CaptureFailure(o Objective, err error) error {
	return captureObjectiveFailure(a.m, o, err)
}

// executeObjective keeps the existing Run-internal name while routing through
// exactly the same adapter-backed public transaction boundary as Execute. There
// is no longer a second Red-only lifecycle implementation hidden behind Run.
func executeObjective(m *emu.Emu, romData []byte, o Objective) (ObjectiveResult, error) {
	return Execute(m, romData, o)
}
