package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// NativeDestination is the wide-map-id navigation goal used by games such as
// Gold/Silver whose native map identity does not fit the historical uint8
// Destination contract.
type NativeDestination struct {
	Map     uint16
	X, Y    uint8
	MapOnly bool
}

func ExactNativeDestination(mapID uint16, x, y uint8) NativeDestination {
	return NativeDestination{Map: mapID, X: x, Y: y}
}

func NativeMapDestination(mapID uint16) NativeDestination {
	return NativeDestination{Map: mapID, MapOnly: true}
}

type nativeRoutingProfile interface {
	game.GameProfile
	game.RoutingDecoder
	game.OverworldDecoder
	NativeMapProvider([]byte) worldmodel.NativeGridProvider
}

func nativeRoutingProfileForROM(romData []byte) (nativeRoutingProfile, error) {
	profile, _, err := profiles.Detect(romData)
	if err != nil {
		return nil, fmt.Errorf("skill: native routing: detect profile: %w", err)
	}
	routing, ok := profile.(nativeRoutingProfile)
	if !ok {
		return nil, fmt.Errorf("skill: native routing: profile %s@%s does not expose native routing semantics", profile.ID(), profile.Revision())
	}
	return routing, nil
}

func nativeRoutingProfileFor(m *emu.Emu) (nativeRoutingProfile, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: native routing: nil emulator")
	}
	return nativeRoutingProfileForROM(m.ROM())
}

func nativeLiveGrid(
	reader game.MemoryReader,
	routing game.RoutingDecoder,
	provider worldmodel.NativeGridProvider,
	mapID uint16,
) (*world.NativeGrid, game.LiveTopologyState, worldmodel.NativeMapHeader, error) {
	if reader == nil || routing == nil || provider == nil {
		return nil, game.LiveTopologyState{}, worldmodel.NativeMapHeader{}, fmt.Errorf("skill: native routing: incomplete live-grid runtime")
	}
	header, err := provider.ParseMap(mapID)
	if err != nil {
		return nil, game.LiveTopologyState{}, worldmodel.NativeMapHeader{}, err
	}
	live, err := routing.DecodeLiveTopology(reader)
	if err != nil {
		return nil, game.LiveTopologyState{}, worldmodel.NativeMapHeader{}, err
	}
	if live.NativeMapID != mapID {
		return nil, live, header, fmt.Errorf("skill: native routing: live map is %#04x, want %#04x", live.NativeMapID, mapID)
	}
	if live.WidthBlocks != int(header.WidthBlocks) || live.HeightBlocks != int(header.HeightBlocks) {
		return nil, live, header, fmt.Errorf(
			"skill: native routing: live map %#04x is %dx%d blocks, provider says %dx%d",
			mapID, live.WidthBlocks, live.HeightBlocks, header.WidthBlocks, header.HeightBlocks,
		)
	}
	want := live.WidthBlocks * live.HeightBlocks
	if len(live.Blocks) != want {
		return nil, live, header, fmt.Errorf("skill: native routing: live map %#04x has %d blocks, want %d", mapID, len(live.Blocks), want)
	}
	spec, err := provider.Grid(mapID, live.Blocks, worldmodel.TraversalMode(live.Traversal))
	if err != nil {
		return nil, live, header, err
	}
	grid, err := world.NativeGridFromSpec(spec)
	if err != nil {
		return nil, live, header, err
	}
	return grid, live, header, nil
}

func nativeRuntimeBlockers(live game.LiveTopologyState, header worldmodel.NativeMapHeader, allowWarp *[2]int) map[[2]int]bool {
	blocked := make(map[[2]int]bool, len(live.LiveObjects)+len(header.Warps))
	for _, object := range live.LiveObjects {
		blocked[[2]int{object.X, object.Y}] = true
	}
	for _, warp := range header.Warps {
		if warp.Inert {
			continue
		}
		at := [2]int{int(warp.X), int(warp.Y)}
		if allowWarp != nil && at == *allowWarp {
			continue
		}
		blocked[at] = true
	}
	return blocked
}

func nativeStepToWorld(step world.NativeStep) world.Step {
	return world.Step{DX: step.DX, DY: step.DY}
}

func walkNativePath(m *emu.Emu, decoder game.OverworldDecoder, path []world.NativeStep) error {
	for _, step := range path {
		worldStep := nativeStepToWorld(step)
		if err := stepOnceWithOverworldDecoder(m, worldStep, decoder); err != nil {
			return err
		}
		if err := movementInterruptionWithDecoder(m, decoder); err != nil {
			if errors.Is(err, ErrBattleInterrupted) {
				return ErrBattle
			}
			return err
		}
	}
	return nil
}

func nativeWalkTo(m *emu.Emu, profile nativeRoutingProfile, provider worldmodel.NativeGridProvider, dest NativeDestination) error {
	const attempts = 12
	for attempt := 0; attempt < attempts; attempt++ {
		liveWorld := profile.DecodeOverworld(m)
		if liveWorld.InBattle {
			return ErrBattle
		}
		if liveWorld.InDialogue {
			return ErrDialogueInterrupted
		}
		if liveWorld.NativeMapID != dest.Map {
			return fmt.Errorf("skill: native routing: local walk on map %#04x while destination is %#04x", liveWorld.NativeMapID, dest.Map)
		}
		if liveWorld.X == dest.X && liveWorld.Y == dest.Y {
			return nil
		}

		grid, live, header, err := nativeLiveGrid(m, profile, provider, dest.Map)
		if err != nil {
			return err
		}
		blocked := nativeRuntimeBlockers(live, header, nil)
		delete(blocked, [2]int{int(liveWorld.X), int(liveWorld.Y)})
		delete(blocked, [2]int{int(dest.X), int(dest.Y)})
		path, err := world.FindNativePath(grid, int(liveWorld.X), int(liveWorld.Y), int(dest.X), int(dest.Y), blocked)
		if err != nil {
			return err
		}
		if err := walkNativePath(m, profile, path); err != nil {
			var blockedStep *ErrBlocked
			if errors.As(err, &blockedStep) {
				m.StepFrames(npcWaitFrames)
				continue
			}
			return err
		}
	}
	return fmt.Errorf("skill: native routing: could not settle on map %#04x tile (%d,%d): %w", dest.Map, dest.X, dest.Y, ErrNavigationStalled)
}

func nativeAdjacentApproach(
	grid *world.NativeGrid,
	sx, sy, wx, wy int,
	blocked map[[2]int]bool,
) ([]world.NativeStep, world.NativeStep, error) {
	type candidate struct {
		x, y int
		push world.NativeStep
		path []world.NativeStep
	}
	var best *candidate
	for _, c := range []candidate{
		{x: wx, y: wy - 1, push: world.NativeStep{DY: 1}},
		{x: wx - 1, y: wy, push: world.NativeStep{DX: 1}},
		{x: wx + 1, y: wy, push: world.NativeStep{DX: -1}},
		{x: wx, y: wy + 1, push: world.NativeStep{DY: -1}},
	} {
		if blocked[[2]int{c.x, c.y}] || !grid.Walkable(c.x, c.y) {
			continue
		}
		path, err := world.FindNativePath(grid, sx, sy, c.x, c.y, blocked)
		if err != nil {
			continue
		}
		c.path = path
		if best == nil || len(c.path) < len(best.path) {
			copy := c
			best = &copy
		}
	}
	if best == nil {
		return nil, world.NativeStep{}, world.ErrNoPath
	}
	return best.path, best.push, nil
}

func nativeConnectionApproach(
	provider worldmodel.NativeMapTopologyProvider,
	grid *world.NativeGrid,
	edge world.NativeEdge,
	sx, sy int,
	blocked map[[2]int]bool,
) ([]world.NativeStep, world.NativeStep, error) {
	dest, err := provider.ParseMap(edge.To)
	if err != nil {
		return nil, world.NativeStep{}, err
	}
	sourceWidth, sourceHeight := grid.Width, grid.Height
	destWidth, destHeight := int(dest.WidthBlocks)*2, int(dest.HeightBlocks)*2

	limit := sourceWidth
	destLimit := destWidth
	push := world.NativeStep{DY: -1}
	switch edge.Dir {
	case 1:
		push = world.NativeStep{DY: 1}
	case 2:
		limit, destLimit = sourceHeight, destHeight
		push = world.NativeStep{DX: -1}
	case 3:
		limit, destLimit = sourceHeight, destHeight
		push = world.NativeStep{DX: 1}
	}

	var best []world.NativeStep
	found := false
	for i := 0; i < limit; i++ {
		if j := i + int(edge.Offset); j < 0 || j >= destLimit {
			continue
		}
		tx, ty := i, 0
		switch edge.Dir {
		case 1:
			ty = sourceHeight - 1
		case 2:
			tx, ty = 0, i
		case 3:
			tx, ty = sourceWidth-1, i
		}
		if blocked[[2]int{tx, ty}] || !grid.Walkable(tx, ty) {
			continue
		}
		path, pathErr := world.FindNativePath(grid, sx, sy, tx, ty, blocked)
		if pathErr != nil {
			continue
		}
		if !found || len(path) < len(best) {
			best = path
			found = true
		}
	}
	if !found {
		return nil, world.NativeStep{}, world.ErrNoPath
	}
	return best, push, nil
}

func pushAcrossNativeEdge(m *emu.Emu, decoder game.OverworldDecoder, edge world.NativeEdge, push world.NativeStep) error {
	btn, ok := buttonFor(nativeStepToWorld(push))
	if !ok {
		return fmt.Errorf("skill: native routing: invalid edge push %+v", push)
	}
	m.Press(btn)
	crossed := false
	for i := 0; i < crossBudget; i++ {
		state := decoder.DecodeOverworld(m)
		if state.NativeMapID != edge.From {
			crossed = true
			break
		}
		if state.InBattle {
			m.Release(btn)
			return ErrBattle
		}
		if state.InDialogue {
			m.Release(btn)
			return ErrDialogueInterrupted
		}
		m.StepFrame()
	}
	m.Release(btn)
	if !crossed {
		state := decoder.DecodeOverworld(m)
		return fmt.Errorf(
			"skill: native routing: edge %#04x -> %#04x did not cross within %d frames at (%d,%d): %w",
			edge.From, edge.To, crossBudget, state.X, state.Y, ErrLegUnwalkable,
		)
	}

	for i := 0; i < arriveBudget; i++ {
		state := decoder.DecodeOverworld(m)
		if state.NativeMapID == edge.To && state.Controllable {
			if err := waitForPositionStableWithDecoder(m, decoder, positionStableBudget, positionStableFrames); err != nil {
				return err
			}
			final := decoder.DecodeOverworld(m)
			if final.NativeMapID != edge.To {
				return fmt.Errorf("skill: native routing: settled on map %#04x after crossing to %#04x: %w", final.NativeMapID, edge.To, ErrLegUnwalkable)
			}
			return nil
		}
		if state.InBattle {
			return ErrBattle
		}
		if state.NativeMapID == edge.To && state.InDialogue {
			return ErrDialogueInterrupted
		}
		m.StepFrame()
	}
	state := decoder.DecodeOverworld(m)
	return fmt.Errorf(
		"skill: native routing: player not controllable after edge %#04x -> %#04x; on %#04x at (%d,%d)",
		edge.From, edge.To, state.NativeMapID, state.X, state.Y,
	)
}

func traverseNativeEdge(
	m *emu.Emu,
	profile nativeRoutingProfile,
	provider worldmodel.NativeGridProvider,
	edge world.NativeEdge,
) error {
	const attempts = 12
	for attempt := 0; attempt < attempts; attempt++ {
		state := profile.DecodeOverworld(m)
		if state.NativeMapID != edge.From {
			return fmt.Errorf("skill: native routing: on map %#04x, edge starts on %#04x", state.NativeMapID, edge.From)
		}
		grid, live, header, err := nativeLiveGrid(m, profile, provider, edge.From)
		if err != nil {
			return err
		}
		blocked := nativeRuntimeBlockers(live, header, nil)
		delete(blocked, [2]int{int(state.X), int(state.Y)})

		var path []world.NativeStep
		var push world.NativeStep
		switch edge.Kind {
		case world.EdgeWarp:
			path, push, err = nativeAdjacentApproach(grid, int(state.X), int(state.Y), int(edge.WarpX), int(edge.WarpY), blocked)
		case world.EdgeConnection:
			path, push, err = nativeConnectionApproach(provider, grid, edge, int(state.X), int(state.Y), blocked)
		default:
			return fmt.Errorf("skill: native routing: unsupported edge kind %d", edge.Kind)
		}
		if err != nil {
			return fmt.Errorf("skill: native routing: approach edge %#04x -> %#04x: %w", edge.From, edge.To, err)
		}
		if err := walkNativePath(m, profile, path); err != nil {
			var blockedStep *ErrBlocked
			if errors.As(err, &blockedStep) {
				m.StepFrames(npcWaitFrames)
				continue
			}
			return err
		}
		if err := pushAcrossNativeEdge(m, profile, edge, push); err != nil {
			if errors.Is(err, ErrLegUnwalkable) {
				m.StepFrames(npcWaitFrames)
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("skill: native routing: edge %#04x -> %#04x exhausted retries: %w", edge.From, edge.To, ErrLegUnwalkable)
}

// GoToNative executes map and tile routing through the wide-id native graph.
// It is deliberately independent of the Gen-I Destination/Graph contracts so
// a recognized Gen-II cartridge is never narrowed or routed through Red.
func GoToNative(m *emu.Emu, romData []byte, dest NativeDestination) error {
	profile, err := nativeRoutingProfileFor(m)
	if err != nil {
		return err
	}
	provider := profile.NativeMapProvider(romData)
	if provider == nil {
		return fmt.Errorf("skill: native routing: profile returned nil native map provider")
	}
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		return err
	}

	seen := map[[3]uint16]bool{}
	for transitions := 0; transitions <= maxNavigationTransitions; transitions++ {
		state := profile.DecodeOverworld(m)
		if state.InBattle {
			return ErrBattle
		}
		if state.InDialogue {
			return ErrDialogueInterrupted
		}
		if state.NativeMapID == dest.Map {
			if dest.MapOnly {
				return nil
			}
			return nativeWalkTo(m, profile, provider, dest)
		}

		key := [3]uint16{state.NativeMapID, uint16(state.X), uint16(state.Y)}
		if seen[key] {
			return fmt.Errorf("skill: native routing: repeated map %#04x at (%d,%d): %w", state.NativeMapID, state.X, state.Y, ErrNavigationStalled)
		}
		seen[key] = true

		route, routeErr := world.FindNativeRoute(graph, state.NativeMapID, dest.Map)
		if routeErr != nil {
			return fmt.Errorf("skill: native routing: route %#04x -> %#04x: %w", state.NativeMapID, dest.Map, routeErr)
		}
		if len(route) == 0 {
			return fmt.Errorf("skill: native routing: empty cross-map route %#04x -> %#04x: %w", state.NativeMapID, dest.Map, ErrNavigationStalled)
		}
		if err := traverseNativeEdge(m, profile, provider, route[0]); err != nil {
			return err
		}
	}
	return fmt.Errorf("skill: native routing: exceeded %d map transitions toward %#04x: %w", maxNavigationTransitions, dest.Map, ErrNavigationStalled)
}
