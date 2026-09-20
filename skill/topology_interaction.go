package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// topologyInteraction is a verified route mutation selected by story-specific
// code. Navigation owns the common transaction, but never invents interactions:
// callers provide the exact approach, target, action, and durable RAM fact.
type topologyInteraction struct {
	Name       string
	Approach   Destination
	TargetX    uint8
	TargetY    uint8
	MaxBattles int
	Budget     int
	Complete   func(*state.Mem) bool
	Interact   func() error
}

// executeTopologyInteraction performs one destination-aware topology mutation.
// It is idempotent on the supplied Complete fact and positively verifies the
// fact after the interaction before returning Changed=true. Callers should
// discard stale route/grid state when Changed is true.
func executeTopologyInteraction(
	m *emu.Emu,
	romData []byte,
	policy MovePolicy,
	spec topologyInteraction,
) (bool, error) {
	if policy == nil {
		return false, fmt.Errorf("skill: topology interaction %q: nil move policy", spec.Name)
	}
	if spec.Complete == nil || spec.Interact == nil {
		return false, fmt.Errorf("skill: topology interaction %q: incomplete specification", spec.Name)
	}
	if spec.MaxBattles <= 0 {
		spec.MaxBattles = 20
	}
	if spec.Budget <= 0 {
		spec.Budget = 1200
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if spec.Complete(&mem) {
		return false, nil
	}

	res, err := Travel(m, romData, spec.Approach, policy, spec.MaxBattles)
	if err != nil {
		return false, fmt.Errorf("skill: topology interaction %q: reach (%d,%d): %w",
			spec.Name, spec.Approach.X, spec.Approach.Y, err)
	}
	if res.BlackedOut {
		return false, fmt.Errorf("skill: topology interaction %q: %w", spec.Name, ErrBlackedOut)
	}

	// Movement to the action can itself satisfy the postcondition (for example
	// an associated trainer battle opening the same gate). Never replay an
	// already-completed interaction.
	state.Snapshot(m, &mem)
	if spec.Complete(&mem) {
		return true, nil
	}
	if got := mem.U8(sym.CurMap); got != spec.Approach.Map {
		return false, fmt.Errorf("skill: topology interaction %q: approach landed on map %#02x, want %#02x", spec.Name, got, spec.Approach.Map)
	}
	if err := Face(m, spec.TargetX, spec.TargetY); err != nil {
		return false, fmt.Errorf("skill: topology interaction %q: face (%d,%d): %w", spec.Name, spec.TargetX, spec.TargetY, err)
	}
	if err := spec.Interact(); err != nil {
		return false, fmt.Errorf("skill: topology interaction %q: %w", spec.Name, err)
	}

	if _, err := m.StepUntil(spec.Budget, func(e *emu.Emu) bool {
		state.Snapshot(e, &mem)
		return spec.Complete(&mem) && state.Controllable(&mem)
	}); err != nil {
		state.Snapshot(m, &mem)
		return false, fmt.Errorf("skill: topology interaction %q: postcondition not observed within %d frames at map %#02x (%d,%d)",
			spec.Name, spec.Budget, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
	return true, nil
}
