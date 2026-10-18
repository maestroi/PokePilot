package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// nativeElevatorArmBudget covers the floor-change script that runs after the
// menu closes (pause, elevator sound, screen shake) before control returns.
const nativeElevatorArmBudget = 900

// prepareNativeElevatorEdge owns the explicit floor choice that turns a native
// elevator's dynamic doors into the edge traversal requested. Without it the
// doors lead back to wherever the player boarded, so walking out cannot prove
// the edge. The profile owns how armed doors are represented and verified.
func prepareNativeElevatorEdge(
	m *emu.Emu,
	profile nativeRoutingProfile,
	provider worldmodel.NativeGridProvider,
	edge world.NativeEdge,
) error {
	if edge.Kind != world.EdgeWarp {
		return nil
	}
	spec, index, ok := worldmodel.NativeElevatorFloorFor(provider, edge.From, edge.To)
	if !ok {
		return nil
	}
	doors, ok := profile.(game.ElevatorTransitionDecoder)
	if !ok {
		return fmt.Errorf("skill: native elevator %#04x: profile %s cannot verify elevator doors", edge.From, profile.ID())
	}
	lists, ok := profile.(game.ListMenuDecoder)
	if !ok {
		return fmt.Errorf("skill: native elevator %#04x: profile %s cannot decode the floor menu", edge.From, profile.ID())
	}
	floor := spec.Floors[index]
	transition := game.ElevatorTransition{
		SourceMapID:      edge.From,
		DestinationMapID: floor.MapID,
		DestinationWarp:  floor.DestWarpID,
		Doors:            []game.MapPoint{{X: int(edge.WarpX), Y: int(edge.WarpY)}},
	}
	// Idempotent: a resumed traversal may already have selected this floor.
	if doors.ElevatorTransitionReady(m, transition) {
		return nil
	}

	grid, live, header, err := nativeLiveGrid(m, profile, provider, edge.From)
	if err != nil {
		return err
	}
	state := profile.DecodeOverworld(m)
	blocked := nativeRuntimeBlockers(live, header, nil)
	delete(blocked, [2]int{int(state.X), int(state.Y)})
	path, _, err := nativeAdjacentApproach(grid, int(state.X), int(state.Y), int(spec.PanelX), int(spec.PanelY), blocked)
	if err != nil {
		return fmt.Errorf("skill: native elevator %#04x cannot reach panel (%d,%d) from (%d,%d): %w",
			edge.From, spec.PanelX, spec.PanelY, state.X, state.Y, err)
	}
	if err := walkNativePath(m, profile, path); err != nil {
		return fmt.Errorf("skill: native elevator %#04x walk to panel (%d,%d): %w", edge.From, spec.PanelX, spec.PanelY, err)
	}
	if err := Face(m, spec.PanelX, spec.PanelY); err != nil {
		return fmt.Errorf("skill: native elevator %#04x face panel (%d,%d): %w", edge.From, spec.PanelX, spec.PanelY, err)
	}
	m.Tap(emu.A, 3, 7)
	if err := waitForElevatorMenu(m, lists, elevatorMenuBudget); err != nil {
		return fmt.Errorf("skill: native elevator %#04x open floor menu for %#04x: %w", edge.From, edge.To, err)
	}
	if err := selectScrollingListEntryWithDecoder(m, lists, index); err != nil {
		return fmt.Errorf("skill: native elevator %#04x select floor %d for map %#04x: %w", edge.From, index, edge.To, err)
	}
	if _, err := m.StepUntil(nativeElevatorArmBudget, func(em *emu.Emu) bool {
		return profile.DecodeOverworld(em).Controllable && doors.ElevatorTransitionReady(em, transition)
	}); err != nil {
		return fmt.Errorf("skill: native elevator %#04x floor %d did not arm doors for map %#04x: %w",
			edge.From, index, floor.MapID, err)
	}
	return nil
}
