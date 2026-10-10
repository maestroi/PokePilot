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

// errLiveMapNotSettled means the cartridge has not handed control back for
// the current map yet, so the live block buffer is not the map's geometry: a
// connection or warp makes the map identity current while a field script still
// owns the overworld, and during that window the block buffer still holds the
// previous map's bytes. Decoding collision there answers a question about a map
// that is not loaded, so this is a retryable interruption rather than a
// geometry failure.
var errLiveMapNotSettled = errors.New("skill: native routing: live map is not ready to route")

const (
	// liveMapSettleFrameBudget bounds waiting for the cartridge to hand back
	// control on the current map. It is generous next to a field script's own
	// text pages while still making a wedged map observable.
	liveMapSettleFrameBudget = 900
	// liveMapSettlePollFrames is how many frames one poll advances. The
	// cartridge's own map entry borrows the same frame loop, so checking more
	// often than this cannot observe anything new.
	liveMapSettlePollFrames = 2
)

// waitLiveMapSettled drives the current map toward a decoded live topology that
// the profile is willing to vouch for. It returns the settled state, and
// errLiveMapNotSettled when the budget runs out with a script still owning the
// overworld. A battle or a dialogue waiting on the player is not a map shell
// that is still loading: neither hands the overworld back without input, so
// waiting on them only burns the budget and hides the interruption the caller
// can actually settle. They return ErrBattle / ErrDialogueInterrupted at once.
func waitLiveMapSettled(m *emu.Emu, routing nativeRoutingProfile) (game.LiveTopologyState, error) {
	var live game.LiveTopologyState
	if m == nil || routing == nil {
		return live, fmt.Errorf("skill: native routing: incomplete live-map runtime")
	}
	for waited := 0; waited <= liveMapSettleFrameBudget; waited += liveMapSettlePollFrames {
		switch world := routing.DecodeOverworld(m); {
		case world.InBattle:
			return game.LiveTopologyState{}, ErrBattle
		case world.InDialogue:
			return game.LiveTopologyState{}, ErrDialogueInterrupted
		}
		decoded, err := routing.DecodeLiveTopology(m)
		if err != nil {
			return game.LiveTopologyState{}, err
		}
		if decoded.BlocksSettled {
			return decoded, nil
		}
		live = decoded
		m.StepFrames(liveMapSettlePollFrames)
	}
	return live, fmt.Errorf("%w: map %#04x phase %d after %d frames", errLiveMapNotSettled, live.NativeMapID, live.MapShellPhase, liveMapSettleFrameBudget)
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
	if !live.BlocksSettled {
		return nil, live, header, fmt.Errorf("%w: map %#04x phase %d", errLiveMapNotSettled, live.NativeMapID, live.MapShellPhase)
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
	// A warp-table entry on a tile that cannot fire it is plain floor here:
	// it must not be treated as a wall to avoid, nor as an exit to take.
	warps := append([]worldmodel.NativeWarp(nil), header.Warps...)
	for i := range warps {
		if !grid.WarpTriggers(int(warps[i].X), int(warps[i].Y)) {
			warps[i].Inert = true
		}
	}
	header.Warps = warps
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

// nativeStaticBlockers is the subset of nativeRuntimeBlockers that outlives one
// plan: warps. Live sprites are omitted so a caller can ask whether an
// ErrNoPath is geometric (still sealed with an empty floor) or only explained
// by a sprite that may move.
func nativeStaticBlockers(header worldmodel.NativeMapHeader, allowWarp *[2]int) map[[2]int]bool {
	return nativeRuntimeBlockers(game.LiveTopologyState{}, header, allowWarp)
}

// nativeErrNoPathCausedBySprites reports whether a no-path result would clear
// if live map objects were not treated as occupied. That is the shared signal
// that the router should wait for a sprite to move and replan, instead of
// proving an edge permanently unreachable for the current entry.
func nativeErrNoPathCausedBySprites(withSprites, withoutSprites error) bool {
	return errors.Is(withSprites, world.ErrNoPath) && withoutSprites == nil
}

// nativeAvoidUnderfootLeave prefers a sibling leave edge when the graph's
// shortest route reverses through the warp the player is already standing on.
// Split-floor maps (Sprout Tower 2F) often tie between that reverse and a side
// stair whose destination entry can reach the goal component; taking the
// reverse drops the player into a pocket that cannot climb the required stairs.
func nativeAvoidUnderfootLeave(
	route []world.NativeEdge,
	x, y uint8,
	alternate func(avoid world.NativeEdge) ([]world.NativeEdge, error),
) []world.NativeEdge {
	if len(route) == 0 || alternate == nil {
		return route
	}
	first := route[0]
	if first.Kind != world.EdgeWarp || int(first.WarpX) != int(x) || int(first.WarpY) != int(y) {
		return route
	}
	alt, err := alternate(first)
	if err != nil || len(alt) == 0 {
		return route
	}
	return alt
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

// nativeArrivalState classifies a live overworld observation against a native
// destination. Standing on the destination tile is not by itself arrival: the
// step that got there can start a player event (a wild encounter, a trainer
// sightline, a warp, a forced map script) that keeps the machine after the step
// finishes.
type nativeArrivalState int

const (
	// nativeArrivalPending means the player is not on the destination tile yet.
	nativeArrivalPending nativeArrivalState = iota
	// nativeArrivalComplete means the player is on the destination tile and the
	// cartridge has handed control back.
	nativeArrivalComplete
	// nativeArrivalInterrupted means the player is on the destination tile but a
	// player event still owns the overworld.
	nativeArrivalInterrupted
)

// nativeArrival decides nativeWalkTo's arrival branch. Reporting success from a
// frozen overworld hands the caller a game that ignores input, so the next
// interaction fails as an untyped "facing did not change" instead of the
// interruption it is (farm run run-11dd5ya1qev0ry: the step onto Ilex Forest
// (20,23) rolled a wild encounter, and Face then polled a direction the
// encounter script would never apply). The controllability requirement matches
// the one GoToNativeRemembering already applies between transitions, and the
// typed outcome lets the owning caller settle the event and re-route.
func nativeArrival(state game.OverworldState, dest NativeDestination) nativeArrivalState {
	if state.NativeMapID != dest.Map || int(state.X) != int(dest.X) || int(state.Y) != int(dest.Y) {
		return nativeArrivalPending
	}
	if state.Controllable {
		return nativeArrivalComplete
	}
	return nativeArrivalInterrupted
}

// nativeObstacleFieldMove maps an obstacle kind to the field move that clears it.
func nativeObstacleFieldMove(kind worldmodel.NativeObstacle) (FieldMove, bool) {
	switch kind {
	case worldmodel.ObstacleCutTree:
		return FieldCut, true
	case worldmodel.ObstacleSmashRock:
		return FieldRockSmash, true
	case worldmodel.ObstacleWhirlpool:
		return FieldWhirlpool, true
	default:
		return 0, false
	}
}

// nativeObstacleUsable reports which obstacle kinds the party can clear right
// now: the badge is owned and the move is learned or teachable. A missing
// prerequisite is a stable "not usable", never an error.
func nativeObstacleUsable(profile nativeRoutingProfile, reader game.MemoryReader, romData []byte) (func(worldmodel.NativeObstacle) bool, error) {
	field, ok := any(profile).(game.FieldMoveDecoder)
	usable := map[worldmodel.NativeObstacle]bool{}
	if ok {
		for _, kind := range []worldmodel.NativeObstacle{worldmodel.ObstacleCutTree, worldmodel.ObstacleSmashRock, worldmodel.ObstacleWhirlpool} {
			move, _ := nativeObstacleFieldMove(kind)
			capability, err := fieldMoveCapabilityWithProfile(field, reader, romData, move)
			if err != nil {
				if errors.Is(err, ErrFieldMovePrerequisite) {
					continue
				}
				return nil, err
			}
			usable[kind] = capability.BadgeOwned && (capability.Usable || (capability.MachineOwned && capability.Preparable))
		}
	}
	return func(kind worldmodel.NativeObstacle) bool { return usable[kind] }, nil
}

// markNativeObjectObstacles overlays live clearable objects (smashable rocks)
// onto the tile grid. They stay in the occupied set; see SetObjectObstacle.
func markNativeObjectObstacles(grid *world.NativeGrid, live game.LiveTopologyState) {
	for _, object := range live.LiveObjects {
		switch object.Clearable {
		case game.FieldMoveRockSmash:
			grid.SetObjectObstacle(object.X, object.Y, worldmodel.ObstacleSmashRock)
		}
	}
}

// ErrObstacleNotCleared reports that a field move ran but the live map still
// shows the obstacle: the move's success byte is not proof the route opened.
var ErrObstacleNotCleared = errors.New("skill: native routing: obstacle still present after field move")

const obstacleClearSettleFrames = 600

func executeNativeObstacleApproach(
	m *emu.Emu,
	profile nativeRoutingProfile,
	provider worldmodel.NativeGridProvider,
	mapID uint16,
	plan world.NativeObstacleApproach,
) error {
	move, ok := nativeObstacleFieldMove(plan.Kind)
	if !ok {
		return fmt.Errorf("skill: native routing: unknown obstacle kind %q", plan.Kind)
	}
	if err := walkNativePath(m, profile, plan.Approach); err != nil {
		return err
	}
	if plan.X < 0 || plan.Y < 0 || plan.X > 255 || plan.Y > 255 {
		return fmt.Errorf("skill: native routing: %s target (%d,%d) outside byte coordinate range", move, plan.X, plan.Y)
	}
	if err := Face(m, uint8(plan.X), uint8(plan.Y)); err != nil {
		return fmt.Errorf("skill: native routing: face %s target (%d,%d): %w", move, plan.X, plan.Y, err)
	}
	if _, err := UseFieldMove(m, move); err != nil {
		if errors.Is(err, ErrBattle) {
			return ErrBattle
		}
		return fmt.Errorf("skill: native routing: %s target (%d,%d): %w", move, plan.X, plan.Y, err)
	}
	// The success byte is set when the script is queued, not when the map
	// changed. Wait for the live map to show the obstacle gone (a Rock Smash
	// can also start a wild battle first).
	for spent := 0; spent < obstacleClearSettleFrames; spent += 10 {
		if profile.DecodeOverworld(m).InBattle {
			return ErrBattle
		}
		if grid, live, _, err := nativeLiveGrid(m, profile, provider, mapID); err == nil {
			if grid.Obstacle(plan.X, plan.Y) == "" && nativeObjectAt(live, plan.X, plan.Y) == "" {
				return nil
			}
		}
		m.StepFrames(10)
	}
	return fmt.Errorf("%w: %s at (%d,%d)", ErrObstacleNotCleared, move, plan.X, plan.Y)
}

func nativeObjectAt(live game.LiveTopologyState, x, y int) game.FieldMoveID {
	for _, object := range live.LiveObjects {
		if object.X == x && object.Y == y {
			return object.Clearable
		}
	}
	return ""
}

func nativeWalkTo(m *emu.Emu, profile nativeRoutingProfile, provider worldmodel.NativeGridProvider, romData []byte, dest NativeDestination) error {
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
		switch nativeArrival(liveWorld, dest) {
		case nativeArrivalComplete:
			return nil
		case nativeArrivalInterrupted:
			return ErrDialogueInterrupted
		}

		if _, err := waitLiveMapSettled(m, profile); err != nil {
			return err
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
			if !errors.Is(err, world.ErrNoPath) {
				return err
			}
			cleared, bridgeErr := nativeObstacleBridge(m, profile, provider, romData, dest.Map, grid, live,
				int(liveWorld.X), int(liveWorld.Y), blocked, [][2]int{{int(dest.X), int(dest.Y)}})
			if bridgeErr != nil {
				return bridgeErr
			}
			if cleared {
				continue
			}
			surfed, surfErr := nativeSurfBridge(m, profile, provider, romData, dest.Map, grid, live,
				int(liveWorld.X), int(liveWorld.Y), blocked,
				func(*world.NativeGrid) ([][2]int, error) {
					return [][2]int{{int(dest.X), int(dest.Y)}}, nil
				})
			if surfErr != nil {
				return surfErr
			}
			if surfed {
				continue // Surf changed the live traversal mode; rebuild the grid.
			}
			staticBlocked := nativeStaticBlockers(header, nil)
			delete(staticBlocked, [2]int{int(liveWorld.X), int(liveWorld.Y)})
			delete(staticBlocked, [2]int{int(dest.X), int(dest.Y)})
			_, staticErr := world.FindNativePath(grid, int(liveWorld.X), int(liveWorld.Y), int(dest.X), int(dest.Y), staticBlocked)
			if nativeErrNoPathCausedBySprites(err, staticErr) {
				m.StepFrames(npcWaitFrames)
				continue
			}
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

func nativeConnectionEdgeBand(
	provider worldmodel.NativeMapTopologyProvider,
	live game.LiveTopologyState,
	edge world.NativeEdge,
) (limit, destLimit, sourceWidth, sourceHeight int, push world.NativeStep, err error) {
	dest, err := provider.ParseMap(edge.To)
	if err != nil {
		return 0, 0, 0, 0, world.NativeStep{}, err
	}
	sourceWidth, sourceHeight = live.WidthBlocks*2, live.HeightBlocks*2
	destWidth, destHeight := int(dest.WidthBlocks)*2, int(dest.HeightBlocks)*2

	limit = sourceWidth
	destLimit = destWidth
	push = world.NativeStep{DY: -1}
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
	return limit, destLimit, sourceWidth, sourceHeight, push, nil
}

func nativeConnectionEdgeTile(edge world.NativeEdge, i, sourceWidth, sourceHeight int) (tx, ty int) {
	tx, ty = i, 0
	switch edge.Dir {
	case 1:
		ty = sourceHeight - 1
	case 2:
		tx, ty = 0, i
	case 3:
		tx, ty = sourceWidth-1, i
	}
	return tx, ty
}

func nativeConnectionApproach(
	provider worldmodel.NativeMapTopologyProvider,
	grid *world.NativeGrid,
	live game.LiveTopologyState,
	edge world.NativeEdge,
	sx, sy int,
	blocked map[[2]int]bool,
) ([]world.NativeStep, world.NativeStep, error) {
	limit, destLimit, sourceWidth, sourceHeight, push, err := nativeConnectionEdgeBand(provider, live, edge)
	if err != nil {
		return nil, world.NativeStep{}, err
	}

	var best []world.NativeStep
	found := false
	for i := 0; i < limit; i++ {
		// Native (Gen-II) offsets are the decomp's: blocks, positive when the
		// neighbour's origin lies further along the shared edge than ours. A
		// source tile i therefore lands on neighbour tile i - 2*Offset. (Gen-I
		// graphs carry the ROM's pre-negated tile alignment, hence i + Offset.)
		if j := i - 2*int(edge.Offset); j < 0 || j >= destLimit {
			continue
		}
		tx, ty := nativeConnectionEdgeTile(edge, i, sourceWidth, sourceHeight)
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

// nativeConnectionTargets lists the walkable, unblocked tiles of a connection
// band: any one of them leaves the map.
func nativeConnectionTargets(
	provider worldmodel.NativeMapTopologyProvider,
	grid *world.NativeGrid,
	live game.LiveTopologyState,
	edge world.NativeEdge,
	blocked map[[2]int]bool,
) ([][2]int, error) {
	limit, destLimit, sourceWidth, sourceHeight, _, err := nativeConnectionEdgeBand(provider, live, edge)
	if err != nil {
		return nil, err
	}
	var targets [][2]int
	for i := 0; i < limit; i++ {
		if j := i - 2*int(edge.Offset); j < 0 || j >= destLimit {
			continue
		}
		tx, ty := nativeConnectionEdgeTile(edge, i, sourceWidth, sourceHeight)
		if blocked[[2]int{tx, ty}] || !grid.Walkable(tx, ty) {
			continue
		}
		targets = append(targets, [2]int{tx, ty})
	}
	return targets, nil
}

// nativeWarpTargets lists the walkable, unblocked tiles beside a warp from
// which one push steps onto it.
func nativeWarpTargets(grid *world.NativeGrid, wx, wy int, blocked map[[2]int]bool) [][2]int {
	var targets [][2]int
	for _, at := range [][2]int{{wx, wy - 1}, {wx - 1, wy}, {wx + 1, wy}, {wx, wy + 1}} {
		if !blocked[at] && grid.Walkable(at[0], at[1]) {
			targets = append(targets, at)
		}
	}
	return targets
}

// nativeObstacleBridge finds the cheapest single field-move clear that opens a
// sealed path to any of targets, runs it, and reports cleared=true so the
// caller re-reads the live map. Same-map nativeWalkTo, connection edges (a Cut
// tree on Route 35 sealing the north exit) and warp edges (Rock Smash rocks on
// the Burned Tower 1F approach to its pit) all share it. cleared=false with a
// nil error means no usable obstacle bridges the gap.
func nativeObstacleBridge(
	m *emu.Emu,
	profile nativeRoutingProfile,
	provider worldmodel.NativeGridProvider,
	romData []byte,
	mapID uint16,
	grid *world.NativeGrid,
	live game.LiveTopologyState,
	sx, sy int,
	blocked map[[2]int]bool,
	targets [][2]int,
) (cleared bool, err error) {
	usable, err := nativeObstacleUsable(profile, m, romData)
	if err != nil {
		return false, fmt.Errorf("skill: native routing: decode field-move capability: %w", err)
	}
	markNativeObjectObstacles(grid, live)
	var best world.NativeObstacleApproach
	found := false
	for _, at := range targets {
		plan, planErr := world.FindNativeObstacleApproach(grid, sx, sy, at[0], at[1], blocked, usable)
		if planErr != nil {
			continue
		}
		if !found || len(plan.Approach) < len(best.Approach) {
			best, found = plan, true
		}
	}
	if !found {
		return false, nil
	}
	if execErr := executeNativeObstacleApproach(m, profile, provider, mapID, best); execErr != nil {
		// The approach walk is a walk: a transient sprite can stand on its
		// route. A raw step block is retryable, so re-read the map and replan.
		var blockedStep *ErrBlocked
		if !errors.As(execErr, &blockedStep) {
			return false, execErr
		}
		m.StepFrames(npcWaitFrames)
	}
	return true, nil
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
	romData []byte,
	edge world.NativeEdge,
) error {
	const attempts = 12
	spriteApproachBlocked := false
	for attempt := 0; attempt < attempts; attempt++ {
		state := profile.DecodeOverworld(m)
		if state.InBattle {
			return ErrBattle
		}
		if state.NativeMapID != edge.From {
			return fmt.Errorf("skill: native routing: on map %#04x, edge starts on %#04x", state.NativeMapID, edge.From)
		}
		if _, err := waitLiveMapSettled(m, profile); err != nil {
			return err
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
			if !grid.WarpTriggers(int(edge.WarpX), int(edge.WarpY)) {
				// Landing-only warp: stepping on it does nothing. Report it as
				// unreachable so the router tries a sibling warp, instead of
				// pushing onto the tile until retries run out.
				return fmt.Errorf("skill: native routing: warp (%d,%d) on %#04x does not trigger from its own tile: %w",
					edge.WarpX, edge.WarpY, edge.From, world.ErrNoPath)
			}
			path, push, err = nativeAdjacentApproach(grid, int(state.X), int(state.Y), int(edge.WarpX), int(edge.WarpY), blocked)
			if errors.Is(err, world.ErrNoPath) {
				cleared, bridgeErr := nativeObstacleBridge(m, profile, provider, romData, edge.From, grid, live,
					int(state.X), int(state.Y), blocked, nativeWarpTargets(grid, int(edge.WarpX), int(edge.WarpY), blocked))
				if bridgeErr != nil {
					return bridgeErr
				}
				if cleared {
					continue
				}
				surfed, surfErr := nativeSurfBridge(m, profile, provider, romData, edge.From, grid, live,
					int(state.X), int(state.Y), blocked,
					func(water *world.NativeGrid) ([][2]int, error) {
						return nativeWarpTargets(water, int(edge.WarpX), int(edge.WarpY), blocked), nil
					})
				if surfErr != nil {
					return surfErr
				}
				if surfed {
					continue
				}
			}
		case world.EdgeConnection:
			// The connection approach works in the destination's block
			// dimensions. They are read from the same settled topology as the
			// grid so the two can never describe different maps.
			path, push, err = nativeConnectionApproach(provider, grid, live, edge, int(state.X), int(state.Y), blocked)
			if errors.Is(err, world.ErrNoPath) {
				targets, targetErr := nativeConnectionTargets(provider, grid, live, edge, blocked)
				if targetErr != nil {
					return targetErr
				}
				cleared, bridgeErr := nativeObstacleBridge(m, profile, provider, romData, edge.From, grid, live,
					int(state.X), int(state.Y), blocked, targets)
				if bridgeErr != nil {
					return bridgeErr
				}
				if cleared {
					continue
				}
				surfed, surfErr := nativeSurfBridge(m, profile, provider, romData, edge.From, grid, live,
					int(state.X), int(state.Y), blocked,
					func(water *world.NativeGrid) ([][2]int, error) {
						return nativeConnectionTargets(provider, water, live, edge, blocked)
					})
				if surfErr != nil {
					return surfErr
				}
				if surfed {
					continue
				}
			}
		default:
			return fmt.Errorf("skill: native routing: unsupported edge kind %d", edge.Kind)
		}
		if err != nil {
			// A wandering sprite on the approach corridor is not proof that this
			// entry can never take the edge. Wait and replan; only a path that
			// stays sealed with sprites removed is a permanent unreachable for
			// the caller's route memory.
			if errors.Is(err, world.ErrNoPath) {
				staticBlocked := nativeStaticBlockers(header, nil)
				delete(staticBlocked, [2]int{int(state.X), int(state.Y)})
				var staticErr error
				switch edge.Kind {
				case world.EdgeWarp:
					_, _, staticErr = nativeAdjacentApproach(grid, int(state.X), int(state.Y), int(edge.WarpX), int(edge.WarpY), staticBlocked)
				case world.EdgeConnection:
					_, _, staticErr = nativeConnectionApproach(provider, grid, live, edge, int(state.X), int(state.Y), staticBlocked)
				}
				if nativeErrNoPathCausedBySprites(err, staticErr) {
					spriteApproachBlocked = true
					m.StepFrames(npcWaitFrames)
					continue
				}
			}
			return fmt.Errorf("skill: native routing: approach edge %#04x -> %#04x: %w", edge.From, edge.To, err)
		}
		spriteApproachBlocked = false
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
	if spriteApproachBlocked {
		return fmt.Errorf("skill: native routing: approach edge %#04x -> %#04x: %w", edge.From, edge.To, world.ErrNoPath)
	}
	return fmt.Errorf("skill: native routing: edge %#04x -> %#04x exhausted retries: %w", edge.From, edge.To, ErrLegUnwalkable)
}

// nativeEntriesReaching returns the warp entries into dest.Map whose landing
// tile can walk to dest on the current live geometry. It must be called while
// standing on dest.Map so the live blocks describe the map being reasoned
// about. Connection entries are not considered: their landing is a band, not a
// tile.
func nativeEntriesReaching(m *emu.Emu, profile nativeRoutingProfile, provider worldmodel.NativeGridProvider, dest NativeDestination) (map[int]bool, error) {
	grid, live, header, err := nativeLiveGrid(m, profile, provider, dest.Map)
	if err != nil {
		return nil, err
	}
	blocked := nativeRuntimeBlockers(live, header, nil)
	delete(blocked, [2]int{int(dest.X), int(dest.Y)})
	entries := map[int]bool{}
	for i, warp := range header.Warps {
		landing := [2]int{int(warp.X), int(warp.Y)}
		blockedHere := make(map[[2]int]bool, len(blocked))
		for k, v := range blocked {
			blockedHere[k] = v
		}
		delete(blockedHere, landing)
		if _, err := world.FindNativePath(grid, landing[0], landing[1], int(dest.X), int(dest.Y), blockedHere); err == nil {
			entries[i] = true
		}
	}
	return entries, nil
}

// GoToNative executes map and tile routing through the wide-id native graph.
// It is deliberately independent of the Gen-I Destination/Graph contracts so
// a recognized Gen-II cartridge is never narrowed or routed through Red.
func GoToNative(m *emu.Emu, romData []byte, dest NativeDestination) error {
	return GoToNativeRemembering(m, romData, dest, NewNativeRouteMemory())
}

// NativeRouteMemory is what a journey has learned about the map graph: which
// edges its live geometry proved unreachable from which entry into a map, and
// how the current map was entered. A caller that retries GoToNativeRemembering
// after a battle or dialogue interruption passes the same memory so the retry
// does not walk back into an edge already proven a dead end.
type NativeRouteMemory struct {
	unreachable map[world.NativeUnreachable]bool
	entry       int
	last        *world.NativeEdge
	// goal/goodEntries remember, across interrupted retries, that goal is in
	// a different walkable component of its own map and may only be reached
	// by entering through these warp entries.
	goal        NativeDestination
	goodEntries map[int]bool
}

func NewNativeRouteMemory() *NativeRouteMemory {
	return &NativeRouteMemory{unreachable: map[world.NativeUnreachable]bool{}, entry: world.NativeEntryUnknown}
}

// GoToNativeRemembering is GoToNative with caller-owned route memory.
func GoToNativeRemembering(m *emu.Emu, romData []byte, dest NativeDestination, mem *NativeRouteMemory) error {
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
	// goodEntries, once set, restricts how dest.Map may be entered: the warp
	// landings whose walkable component contains the goal tile. It survives
	// battle/dialogue retries through mem.
	var goodEntries map[int]bool
	if mem.goal == dest {
		goodEntries = mem.goodEntries
	}
	for transitions := 0; transitions <= maxNavigationTransitions; transitions++ {
		if err := waitOutScriptedMovement(m); err != nil {
			return err
		}
		// A connection or warp makes the destination map current while the
		// cartridge is still rebuilding its blocks. Routing on those bytes
		// would answer a geometry question about a map that does not exist
		// yet, so wait for the shell to settle before reading it as collision.
		if _, err := waitLiveMapSettled(m, profile); err != nil {
			return err
		}
		state := profile.DecodeOverworld(m)
		if state.InBattle {
			return ErrBattle
		}
		if state.InDialogue {
			return ErrDialogueInterrupted
		}
		// Map entry itself can run a script (SetUpScriptedMovement), which is a
		// different interruption from the tile-level dialogue check above: the
		// map identity is already the destination's while a text box still owns
		// the overworld. Report it as the interruption it is so the caller can
		// settle the script instead of routing through stale map bytes.
		if !state.Controllable {
			return ErrDialogueInterrupted
		}
		// An interruption can land after the edge was crossed, so the entry is
		// settled from where the player actually is, not from what finished.
		if e := mem.last; e != nil && e.To != e.From && state.NativeMapID == e.To {
			mem.entry = e.Entry()
		}
		mem.last = nil

		if state.NativeMapID == dest.Map && (goodEntries == nil || goodEntries[mem.entry]) {
			if dest.MapOnly {
				return nil
			}
			walkErr := nativeWalkTo(m, profile, provider, romData, dest)
			if !errors.Is(walkErr, world.ErrNoPath) || goodEntries != nil {
				return walkErr
			}
			// The goal tile is in another walkable component of this map
			// (Burned Tower B1F is separate pockets joined only through 1F
			// pits). Leave and re-enter through a warp whose landing reaches it.
			entries, entriesErr := nativeEntriesReaching(m, profile, provider, dest)
			if entriesErr != nil || len(entries) == 0 {
				return walkErr
			}
			goodEntries = entries
			mem.goal, mem.goodEntries = dest, entries
			continue
		}

		key := [3]uint16{state.NativeMapID, uint16(state.X), uint16(state.Y)}
		if seen[key] {
			return fmt.Errorf("skill: native routing: repeated map %#04x at (%d,%d): %w", state.NativeMapID, state.X, state.Y, ErrNavigationStalled)
		}
		seen[key] = true

		var accept func(world.NativeEdge) bool
		if goodEntries != nil {
			accept = func(e world.NativeEdge) bool { return e.Kind == world.EdgeWarp && goodEntries[e.Entry()] }
		}
		route, routeErr := world.FindNativeRouteToEntry(graph, state.NativeMapID, mem.entry, dest.Map, accept, mem.unreachable)
		if routeErr != nil {
			return fmt.Errorf("skill: native routing: route %#04x -> %#04x: %w", state.NativeMapID, dest.Map, routeErr)
		}
		if len(route) == 0 {
			return fmt.Errorf("skill: native routing: empty cross-map route %#04x -> %#04x: %w", state.NativeMapID, dest.Map, ErrNavigationStalled)
		}
		route = nativeAvoidUnderfootLeave(route, state.X, state.Y, func(avoid world.NativeEdge) ([]world.NativeEdge, error) {
			trialBad := make(map[world.NativeUnreachable]bool, len(mem.unreachable)+1)
			for k, v := range mem.unreachable {
				trialBad[k] = v
			}
			trialBad[world.NativeUnreachable{Map: state.NativeMapID, Entry: mem.entry, Edge: avoid}] = true
			return world.FindNativeRouteToEntry(graph, state.NativeMapID, mem.entry, dest.Map, accept, trialBad)
		})
		mem.last = &route[0]
		if err := traverseNativeEdge(m, profile, provider, romData, route[0]); err != nil {
			// The edge exists in the map graph but this arrival's walkable
			// component cannot reach it (a sealed seam, or stairs that only
			// the other entry into the map connects to): remember that and
			// re-route, which leaves the map and re-enters elsewhere.
			if errors.Is(err, world.ErrNoPath) {
				mem.unreachable[world.NativeUnreachable{Map: state.NativeMapID, Entry: mem.entry, Edge: route[0]}] = true
				mem.last = nil
				delete(seen, key)
				continue
			}
			return err
		}
	}
	return fmt.Errorf("skill: native routing: exceeded %d map transitions toward %#04x: %w", maxNavigationTransitions, dest.Map, ErrNavigationStalled)
}
