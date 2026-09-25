package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

const elevatorMenuBudget = 300

// prepareElevatorEdge owns the explicit floor choice that turns a Red
// elevator's dynamic door into the graph edge requested by traversal. The ROM
// initially points the doors back to the floor the player entered from; merely
// walking out therefore returns to that floor. Selecting the panel menu rewrites
// both live wWarpEntries records to the requested destination.
//
// This runs only for adapter-declared elevator edges. Ordinary warps retain the
// generic Traverse behavior.
func prepareElevatorEdge(m *emu.Emu, h worldmodel.HeaderView, e world.Edge, grid *world.Grid) error {
	routing, err := routingProfileFor(m)
	if err != nil {
		return err
	}
	provider := routing.MapProvider(m.ROM())
	if provider == nil {
		return fmt.Errorf("skill: elevator: nil map provider")
	}
	spec, ok := provider.LookupElevator(e.From)
	if !ok {
		return nil
	}
	floor, ok := provider.ElevatorFloorForDestination(e.From, e.To)
	if !ok {
		return nil
	}
	floorIndex := -1
	for i, candidate := range spec.Floors {
		if candidate == floor {
			floorIndex = i
			break
		}
	}
	if floorIndex < 0 {
		return fmt.Errorf("skill: elevator %02x destination %02x missing from floor menu", e.From, e.To)
	}
	header := h.WorldMapHeader()
	if e.Kind != world.EdgeWarp {
		return fmt.Errorf("skill: elevator transition %02x->%02x is not a warp edge", e.From, e.To)
	}

	// Idempotent: a resumed round can re-enter Traverse for an edge whose
	// floor choice a PRIOR attempt already made. MEASURED (run
	// run-1e5adrg1q06ekjpj56w7sz0xc): re-opening DisplayElevatorFloorMenu and
	// re-selecting the same floor when the live door table already points
	// there corrupts something in the menu-close/shake sequencing that then
	// blocks the walk out — the identical crossing succeeds immediately when
	// this redundant re-selection is skipped. If the doors are already armed
	// for this destination, there is nothing left to do.
	if elevatorWarpEntriesMatch(m, h, floor) {
		return nil
	}

	// Reach the control panel without ever stepping on a door warp. A path
	// through one of those tiles would leave the elevator before the choice is
	// made, reproducing the exact failure this controller is preventing.
	sx, sy := playerXY(m)
	blocked := spriteBlockers(m)
	if blocked == nil {
		blocked = map[[2]int]bool{}
	}
	for _, w := range header.Warps {
		if int(w.X) == int(sx) && int(w.Y) == int(sy) {
			continue
		}
		blocked[[2]int{int(w.X), int(w.Y)}] = true
	}
	steps, _, err := world.FindPathAdjacent(grid, int(sx), int(sy), int(spec.PanelX), int(spec.PanelY), blocked)
	if err != nil {
		return fmt.Errorf("skill: elevator %02x cannot reach panel (%d,%d) from (%d,%d): %w",
			e.From, spec.PanelX, spec.PanelY, sx, sy, err)
	}
	if len(steps) > 0 {
		if err := WalkPath(m, steps); err != nil {
			return fmt.Errorf("skill: elevator %02x walk to panel (%d,%d): %w", e.From, spec.PanelX, spec.PanelY, err)
		}
	}
	if err := Face(m, spec.PanelX, spec.PanelY); err != nil {
		return fmt.Errorf("skill: elevator %02x face panel (%d,%d): %w", e.From, spec.PanelX, spec.PanelY, err)
	}

	m.Tap(emu.A, 3, 7)
	if _, err := WaitForInteraction(m, state.InteractionElevatorMenu, elevatorMenuBudget); err != nil {
		return fmt.Errorf("skill: elevator %02x open floor menu for %02x: %w", e.From, e.To, err)
	}
	if err := SelectInteractionIndex(m, floorIndex); err != nil {
		return fmt.Errorf("skill: elevator %02x select floor %d for map %02x: %w", e.From, floorIndex, e.To, err)
	}

	// Positive postcondition: the menu must actually have rewritten every live
	// elevator door to the requested map/warp and returned control before
	// Traverse is allowed to step through one of them.
	if _, err := m.StepUntil(arriveBudget, func(em *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(em, &mem)
		return state.Controllable(&mem) && elevatorWarpEntriesMatch(em, h, floor)
	}); err != nil {
		return fmt.Errorf("skill: elevator %02x floor %d did not arm doors for map %02x warp %d: %w",
			e.From, floorIndex, floor.MapID, floor.DestWarpID, err)
	}
	return nil
}

func elevatorWarpEntriesMatch(m *emu.Emu, h worldmodel.HeaderView, floor worldmodel.ElevatorFloor) bool {
	header := h.WorldMapHeader()
	if int(m.Peek8(sym.NumberOfWarps)) < len(header.Warps) {
		return false
	}
	for i, w := range header.Warps {
		addr := sym.WarpEntries + uint16(i*4)
		if m.Peek8(addr) != w.Y || m.Peek8(addr+1) != w.X {
			return false
		}
		if m.Peek8(addr+2) != floor.DestWarpID || m.Peek8(addr+3) != floor.MapID {
			return false
		}
	}
	return len(header.Warps) > 0
}
