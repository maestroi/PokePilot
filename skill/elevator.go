package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

const elevatorMenuBudget = 300

func elevatorTransitionRequest(h worldmodel.HeaderView, floor worldmodel.ElevatorFloor) game.ElevatorTransition {
	header := h.WorldMapHeader()
	doors := make([]game.MapPoint, 0, len(header.Warps))
	for _, w := range header.Warps {
		doors = append(doors, game.MapPoint{X: int(w.X), Y: int(w.Y)})
	}
	return game.ElevatorTransition{
		SourceMapID:      uint16(header.ID),
		DestinationMapID: uint16(floor.MapID),
		DestinationWarp:  floor.DestWarpID,
		Doors:            doors,
	}
}

func waitForElevatorMenu(m *emu.Emu, decoder game.ListMenuDecoder, budget int) error {
	if decoder == nil {
		return fmt.Errorf("skill: elevator: nil list-menu decoder")
	}
	for i := 0; i <= budget; i++ {
		state := decoder.DecodeListMenu(m)
		if state.Visible && state.Kind == game.ListMenuElevator {
			return nil
		}
		if i != budget {
			m.StepFrame()
		}
	}
	state := decoder.DecodeListMenu(m)
	return fmt.Errorf("skill: elevator: floor menu did not appear after %d frames (visible=%v kind=%q)",
		budget, state.Visible, state.Kind)
}

// prepareElevatorEdge owns the explicit floor choice that turns an elevator's
// dynamic door into the graph edge requested by traversal. Static floor/panel
// metadata comes from the map provider; profiles own how live door mutation is
// represented and verified.
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
	if e.Kind != world.EdgeWarp {
		return fmt.Errorf("skill: elevator transition %02x->%02x is not a warp edge", e.From, e.To)
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

	transitionDecoder, err := elevatorTransitionDecoderFor(m)
	if err != nil {
		return err
	}
	transition := elevatorTransitionRequest(h, floor)

	// Idempotent: a resumed traversal may already have selected this floor.
	if transitionDecoder.ElevatorTransitionReady(m, transition) {
		return nil
	}

	live, err := currentRoutingRuntime(m)
	if err != nil {
		return err
	}
	sx, sy := live.X, live.Y
	header := h.WorldMapHeader()
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

	listDecoder, err := listMenuDecoderFor(m)
	if err != nil {
		return err
	}
	m.Tap(emu.A, 3, 7)
	if err := waitForElevatorMenu(m, listDecoder, elevatorMenuBudget); err != nil {
		return fmt.Errorf("skill: elevator %02x open floor menu for %02x: %w", e.From, e.To, err)
	}
	if err := selectScrollingListEntryWithDecoder(m, listDecoder, floorIndex); err != nil {
		return fmt.Errorf("skill: elevator %02x select floor %d for map %02x: %w", e.From, floorIndex, e.To, err)
	}

	overworld, err := overworldDecoderFor(m)
	if err != nil {
		return err
	}
	if _, err := m.StepUntil(arriveBudget, func(em *emu.Emu) bool {
		return overworld.DecodeOverworld(em).Controllable &&
			transitionDecoder.ElevatorTransitionReady(em, transition)
	}); err != nil {
		return fmt.Errorf("skill: elevator %02x floor %d did not arm doors for map %02x warp %d: %w",
			e.From, floorIndex, floor.MapID, floor.DestWarpID, err)
	}
	return nil
}
