package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// topologyInteraction is a verified topology mutation selected by
// story-specific code. Navigation owns the common transaction, but never
// invents interactions: callers provide the exact approach, target, legal
// action, durable RAM fact, and (for route actions) the reachability goal.
//
// GoalReachable is what makes an interaction route-scoped rather than merely a
// scripted story action. When present, the executor refuses to press A if that
// exact goal is already reachable, and after the durable state change it
// verifies the goal became reachable before reporting Changed=true.
type topologyInteraction struct {
	Name          string
	Approach      Destination
	TargetX       uint8
	TargetY       uint8
	MaxBattles    int
	Budget        int
	Complete      func(*state.Mem) bool
	Interact      func() error
	RouteGoal     string
	GoalReachable func() bool
}

func (spec topologyInteraction) routeGoalName() string {
	if spec.RouteGoal != "" {
		return spec.RouteGoal
	}
	return "requested route goal"
}

func (spec topologyInteraction) goalReachable() bool {
	return spec.GoalReachable != nil && spec.GoalReachable()
}

func validateTopologyInteraction(spec topologyInteraction) error {
	if spec.Complete == nil || spec.Interact == nil {
		return fmt.Errorf("skill: topology interaction %q: incomplete specification", spec.Name)
	}
	if (spec.RouteGoal == "") != (spec.GoalReachable == nil) {
		return fmt.Errorf("skill: topology interaction %q: RouteGoal and GoalReachable must be supplied together", spec.Name)
	}
	return nil
}

// executeTopologyInteraction performs one verified topology mutation.
//
// The transaction is:
//   1. prove the requested route goal is still blocked (when supplied),
//   2. walk to the story-owned interaction,
//   3. re-check both goal and durable completion from live state,
//   4. face + interact,
//   5. positively verify the RAM/event postcondition,
//   6. prove the requested goal is now reachable,
//   7. return Changed=true so callers discard stale route/grid state.
//
// A route-scoped interaction therefore cannot become "navigation failed, press
// A on something nearby": story code chooses the legal action, while this
// executor proves that action was needed and changed the requested topology.
func executeTopologyInteraction(
	m *emu.Emu,
	romData []byte,
	policy MovePolicy,
	spec topologyInteraction,
) (bool, error) {
	if policy == nil {
		return false, fmt.Errorf("skill: topology interaction %q: nil move policy", spec.Name)
	}
	if err := validateTopologyInteraction(spec); err != nil {
		return false, err
	}
	if spec.MaxBattles <= 0 {
		spec.MaxBattles = 20
	}
	if spec.Budget <= 0 {
		spec.Budget = 1200
	}

	// Destination-aware route actions are demand-driven. If the exact goal is
	// already reachable, do not walk toward or interact with the switch/door.
	if spec.goalReachable() {
		return false, nil
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if spec.Complete(&mem) {
		if spec.GoalReachable != nil {
			return false, fmt.Errorf(
				"skill: topology interaction %q: durable postcondition is already complete but %s is still unreachable",
				spec.Name, spec.routeGoalName(),
			)
		}
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

	// Movement can itself change topology (for example a trainer encounter that
	// opens the same gate). Re-read the route before replaying any interaction.
	if spec.goalReachable() {
		state.Snapshot(m, &mem)
		return spec.Complete(&mem), nil
	}

	state.Snapshot(m, &mem)
	if spec.Complete(&mem) {
		if spec.GoalReachable != nil {
			return false, fmt.Errorf(
				"skill: topology interaction %q: movement satisfied the durable state but did not make %s reachable",
				spec.Name, spec.routeGoalName(),
			)
		}
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

	if spec.GoalReachable != nil && !spec.GoalReachable() {
		return false, fmt.Errorf(
			"skill: topology interaction %q: verified durable state change did not make %s reachable",
			spec.Name, spec.routeGoalName(),
		)
	}
	return true, nil
}
