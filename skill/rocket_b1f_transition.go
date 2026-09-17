package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// rocketB1FExitReachableOnGrid reports whether the selected B1F exit warp is
// already reachable through ordinary live geometry. The Rocket5 semantic
// transition exists to open the runtime-replaced door between the elevator
// landing and the north exits; it must not force that fight when a resumed
// checkpoint is already on the exit side of the still-closed door.
func rocketB1FExitReachableOnGrid(h rom.MapHeader, edge world.Edge, grid *world.Grid, sx, sy int, romData []byte) (bool, error) {
	_, _, _, _, err := warpTarget(h, edge, grid, sx, sy, nil, romData)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, world.ErrNoPath) {
		return false, nil
	}
	return false, err
}

// executeRocketB1FTrainerDoorIfNeeded preserves #587's elevator-side recovery
// while making the action conditional on live topology. #895 was captured on
// B1F while trying to return to Celadon: if the player is already north of the
// closed door, ChallengeTrainer cannot reach Rocket5 on the south side and the
// semantic action itself becomes the no_path it was meant to solve.
func (x *redRouteTransitionExecutor) executeRocketB1FTrainerDoorIfNeeded(edge world.Edge) (world.TransitionExecutionResult, error) {
	if got := x.m.Peek8(sym.CurMap); got != rocketHideoutB1FMap {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Rocket B1F door transition on map %#04x, want B1F %#04x", got, rocketHideoutB1FMap)
	}

	h, err := rom.ParseMap(x.romData, rocketHideoutB1FMap)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Rocket B1F door: parse B1F: %w", err)
	}
	grid, err := liveMapGrid(x.m, x.romData, h)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Rocket B1F door: build live B1F grid: %w", err)
	}
	px, py := playerXY(x.m)
	reachable, err := rocketB1FExitReachableOnGrid(h, edge, grid, int(px), int(py), x.romData)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Rocket B1F door: inspect selected exit: %w", err)
	}
	if reachable {
		// Ordinary Traverse can already reach this exact exit. Do not turn a
		// valid north-side checkpoint into a request to cross the locked door
		// backwards just to fight the grunt that opens it.
		return world.TransitionExecutionResult{}, nil
	}

	return x.executeRocketB1FTrainerDoor()
}
