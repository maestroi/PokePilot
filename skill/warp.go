package skill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// Budgets for the two phases of an edge crossing:
//
//   - crossBudget: hold the push direction until the map flips. Measured
//     ground truth is ~120 frames for the 2F stairs; 180 leaves margin.
//   - arriveBudget: after the flip, wait until the destination map is fully
//     loaded and the player is controllable on it.
const (
	crossBudget  = 180
	arriveBudget = 600

	// positionStableBudget / positionStableFrames: after a map flip the tile
	// position passes through transient states (the source warp tile, then the
	// destination door tile, then the standing position) before settling. On
	// the 25->00 warp the transients last ~32 and ~21 frames, so wait for the
	// position to be unchanged for positionStableFrames consecutive frames
	// (longer than any transient) within positionStableBudget total frames.
	positionStableBudget = 500
	positionStableFrames = 50
)

// Traverse executes one graph edge and returns once the destination map is
// loaded and the player is controllable on it.
//
// The player is walked to a walkable tile orthogonally adjacent to the
// crossing tile, then the push direction is held until wCurMap changes.
// The button is released the frame the map flips, so the player never
// re-walks on the destination map.
//
// For a warp edge the crossing tile is not the one the graph edge carries:
// a map can expose several warp tiles to the same destination (the forest
// gates both have (4,0) and (5,0)), and the graph does not know which of
// them the tile pathfinder can reach from where the player stands. Warp
// tiles the collision grid marks walkable are preferred, then considered in
// ROM warp-table order; solid-only stair/warp destinations remain supported
// (warpTarget).
// ErrLegUnwalkable reports that a leg the map graph offered cannot be
// walked from where the player is standing: the warp tile or the map edge
// it leads to is unreachable on this map. The graph knows which maps
// touch, not which are walkable between, so this is a normal discovery
// rather than a defect — the caller bans the edge and re-plans.
var ErrLegUnwalkable = errors.New("skill: leg is not walkable from here")

// ErrLegBouncesBack reports that a crossing was measured to settle back on
// its OWN origin map instead of holding on the destination — a forced-scroll
// script (Cycling Road's mandatory downhill descent) rather than a blocked
// approach tile. Unlike the ordinary ErrLegUnwalkable case (Route 2's ledge:
// genuinely walkable from one tile of the edge and not another), a bounce is
// evidence about the connection itself, not about the tile it was crossed
// from: every tile of Route 18's north edge feeds the same forced descent,
// so re-trying it from a different tile of the same map is not a fresh
// discovery. It wraps ErrLegUnwalkable so existing errors.Is(err,
// ErrLegUnwalkable) callers keep working unchanged; the caller that cares
// about the stronger claim checks ErrLegBouncesBack specifically and bans
// the whole map's edge (legFromMap) rather than just the one tile (legAt).
var ErrLegBouncesBack = fmt.Errorf("skill: leg settles back on its own origin map: %w", ErrLegUnwalkable)

// maxWarpApproachAttempts bounds the retry-from-a-different-side loop below
// to one try per orthogonal neighbour of the target warp tile.
const maxWarpApproachAttempts = 4

// maxWarpCandidates bounds how many distinct candidate warp tiles (Route 7
// Gate's Saffron door is a genuine ROM pair reachable from the same side)
// Traverse will cycle through before giving up on the edge.
const maxWarpCandidates = 4

func Traverse(m *emu.Emu, romData []byte, e world.Edge) error {
	cur := m.Peek8(sym.CurMap)
	if cur != e.From {
		return fmt.Errorf("skill: Traverse: on map %02x, but edge starts on %02x", cur, e.From)
	}
	if e.Kind == world.EdgeWarp && e.From == e.To {
		return traverseIntraMapWarp(m, romData, e)
	}

	h, err := rom.ParseMap(romData, e.From)
	if err != nil {
		return fmt.Errorf("skill: Traverse: parse map %02x: %w", e.From, err)
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return fmt.Errorf("skill: Traverse: build live map %02x: %w", e.From, err)
	}
	if err := prepareElevatorEdge(m, h, e, grid); err != nil {
		return fmt.Errorf("skill: Traverse: prepare elevator edge %02x->%02x: %w", e.From, e.To, err)
	}

	if e.Kind == world.EdgeConnection {
		push, err := walkToConnectionEdge(m, h, grid, e)
		if err != nil && errors.Is(err, ErrLegUnwalkable) {
			// Land-only collision may split the selected source band behind Cut,
			// Surf, Strength, or a forced-movement tile. Ask the same local
			// capability planner used by GoTo to reach this exact connection band,
			// then retry ordinary edge traversal from the resulting live state.
			if fieldErr := approachConnectionWithFieldPath(m, romData, e); fieldErr == nil {
				if refreshed, refreshErr := liveMapGrid(m, romData, h); refreshErr == nil {
					grid = refreshed
				}
				push, err = walkToConnectionEdge(m, h, grid, e)
			}
		}
		if err != nil {
			return err
		}
		btn, ok := buttonFor(push)
		if !ok {
			return fmt.Errorf("skill: Traverse: invalid push step %s on %02x->%02x", push, e.From, e.To)
		}
		if err := pushAcrossEdge(m, e, btn); err != nil {
			return err
		}
		return finishArrival(m, e)
	}
	if e.Kind != world.EdgeWarp {
		return fmt.Errorf("skill: Traverse: unknown edge kind %d on %02x->%02x", e.Kind, e.From, e.To)
	}

	// A destination can offer more than one candidate warp tile (Route 7
	// Gate's Saffron-side door is a genuine ROM pair, (18,9) and (18,10),
	// both leading to adjacent landing tiles on the far side). warpTarget
	// picks its first reachable candidate in ROM order, but that candidate
	// is only ever a guess: MEASURED on run-13kws9zfzq7ka1p4bmd16c9yg3, none
	// of the four orthogonal approaches to (18,9) ever fired the warp (the
	// tile's graphic is not a recognized door/warp tile, and the collision
	// check's "tile two ahead" never matched for that coordinate either),
	// while pushing west from (18,10) — the OTHER candidate — does fire it.
	// excludeWarp lets a caller ban an exhausted candidate and have
	// warpTarget hand back the next one instead of failing the whole edge.
	excludeWarp := map[[2]int]bool{}
	var lastErr error
	fieldApproachTried := false
	for candidate := 0; candidate < maxWarpCandidates; candidate++ {
		var unwalkable error
		var wx, wy int
		var push world.Step
		err = walkAroundAvoidingObjects(func() error { return movementInterruption(m) }, m, h,
			func(blocked map[[2]int]bool) ([]world.Step, error) {
				// The tile is re-chosen on every re-plan, from wherever the
				// interrupted walk stopped: which warp tile is reachable is a
				// fact about where the player stands right now, never a property
				// of the map or the warp, so it is never cached.
				x, y := playerXY(m)
				rx, ry, steps, p, err := warpTarget(h, e, grid, int(x), int(y), blocked, excludeWarp, romData)
				if err != nil {
					unwalkable = fmt.Errorf("skill: Traverse: no reachable warp to %02x from (%d,%d) on map %02x (edge tile %d,%d): %v: %w",
						e.To, x, y, e.From, e.WarpX, e.WarpY, err, ErrLegUnwalkable)
					return nil, unwalkable
				}
				wx, wy = rx, ry
				push = p // the last plan's push direction is the one walked
				return steps, nil
			}, func(steps []world.Step) error { return WalkPath(m, steps) },
			func() { m.StepFrames(npcWaitFrames) })
		if err != nil {
			if err == unwalkable {
				// Land-only FindPath still treats Cut trees as solid. Local
				// field pathing already owns those trees for same-map walks;
				// warp approaches must use the same destination-aware planner
				// instead of failing a sealed pocket (Celadon Gym's leader
				// chamber after #1327 removed post-failure nearest-tree cuts).
				if !fieldApproachTried {
					fieldApproachTried = true
					if aperr := approachWarpWithFieldPath(m, romData, e); aperr == nil {
						if g, gerr := liveMapGrid(m, romData, h); gerr == nil {
							grid = g
						}
						candidate--
						continue
					}
				}
				if lastErr != nil {
					return lastErr
				}
				return err
			}
			// Normalize to ErrBattle exactly as the connection branch does, so a
			// caller can test one sentinel no matter which layer was walking: a
			// wild encounter on the walk to a warp is the same recoverable event
			// as one on the walk to an edge, and Travel (and the llm loop) rely
			// on ErrBattle to fight it and re-plan from where the walk stopped.
			if errors.Is(err, ErrBattleInterrupted) {
				x, y := playerXY(m)
				return fmt.Errorf("skill: Traverse: battle on map %02x at (%d,%d): %w", e.From, x, y, ErrBattle)
			}
			return fmt.Errorf("skill: Traverse: walk to warp on map %02x: %w", e.From, err)
		}

		// A warp tile is only ever route's first guess, not a guarantee: Route
		// 18 Gate's (33,8)/(33,9) are genuine ROM warp tiles the door/warp-tile
		// graphic checks recognize when arrived at from the west (push right),
		// yet the exact same tiles never cross when arrived at from the north
		// (push down) — measured by holding the push for the full crossBudget,
		// then for 5,000,000 emulated CPU steps, with no difference. Pokemon
		// Red's own warp check is graphic- and (for the ExtraWarpCheck fallback)
		// facing-dependent, not purely coordinate-based, so more than one
		// orthogonal approach to the same warp tile can exist with only one of
		// them actually firing it.
		//
		// A failed approach is retried from a different orthogonal neighbour of
		// the SAME target tile, reached by an unrestricted walk rather than
		// another warpTarget search: the player has, by definition, ended the
		// failed push standing on or beside a warp tile that just proved it does
		// not fire from this direction, and warpTarget's own route-crosses-a-warp
		// guard (needed so a shared approach corridor never fires the wrong one
		// of two DIFFERENT destinations) would otherwise block every retry path
		// that leads back out through it.
		tried := map[[2]int]bool{}
		for attempt := 0; attempt < maxWarpApproachAttempts; attempt++ {
			ax, ay := playerXY(m)
			tried[[2]int{int(ax), int(ay)}] = true

			btn, ok := buttonFor(push)
			if !ok {
				return fmt.Errorf("skill: Traverse: invalid push step %s on %02x->%02x", push, e.From, e.To)
			}
			crossErr := pushAcrossEdge(m, e, btn)
			if crossErr == nil {
				return finishArrival(m, e)
			}
			if !errors.Is(crossErr, errDidNotCross) {
				return crossErr // battle, or another terminal failure
			}
			lastErr = crossErr

			nx, ny, npush, ok := untriedOrthogonalApproach(grid, wx, wy, tried)
			if !ok {
				break
			}
			x, y := playerXY(m)
			steps, ferr := world.FindPath(grid, int(x), int(y), nx, ny, nil)
			if ferr != nil {
				break
			}
			if werr := WalkPath(m, steps); werr != nil {
				if errors.Is(werr, ErrBattleInterrupted) {
					px, py := playerXY(m)
					return fmt.Errorf("skill: Traverse: battle on map %02x at (%d,%d): %w", e.From, px, py, ErrBattle)
				}
				break
			}
			push = npush
		}
		// Every orthogonal approach to this candidate tile within budget
		// either was unwalkable or failed to fire the warp: ban it and let
		// the next walkAroundAvoidingObjects/warpTarget pass hand back a
		// different candidate, if the edge has one.
		excludeWarp[[2]int{wx, wy}] = true
	}
	return lastErr
}

// untriedOrthogonalApproach returns a walkable orthogonal neighbour of
// (wx,wy) not yet in tried, plus the push step from it into (wx,wy). Order
// matches FindPathAdjacent's own (north, west, east, south) so a fresh
// warpTarget call and this retry agree on which neighbour is "first".
func untriedOrthogonalApproach(g *world.Grid, wx, wy int, tried map[[2]int]bool) (nx, ny int, push world.Step, ok bool) {
	for _, n := range [][2]int{{wx, wy - 1}, {wx - 1, wy}, {wx + 1, wy}, {wx, wy + 1}} {
		if !g.InBounds(n[0], n[1]) || !g.Walkable(n[0], n[1]) || tried[n] {
			continue
		}
		return n[0], n[1], world.Step{DX: wx - n[0], DY: wy - n[1]}, true
	}
	return 0, 0, world.Step{}, false
}

// walkToConnectionEdge walks to a plain map-edge crossing and returns the
// push step that enters the destination map. The edge tile is re-chosen on
// every re-plan, not just the path to it: an NPC standing in a one-tile gap
// can make the nearest edge tile unreachable while another one on the same
// edge is fine.
func walkToConnectionEdge(m *emu.Emu, h rom.MapHeader, grid *world.Grid, e world.Edge) (world.Step, error) {
	var unwalkable error
	err := walkAroundAvoidingObjects(func() error { return movementInterruption(m) }, m, h,
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			x, y := playerXY(m)
			// A connection edge is ordinary ground, not a door, but the path
			// to it can still cross another warp tile of this same map (the
			// Cerulean Badge House's front door sits right in the plaza).
			// Stepping on ANY warp tile fires it, same as walking onto the
			// intended one, and silently diverts the walk into that building
			// instead of toward the border — MEASURED on
			// run-3cefsxn84apv3126k7vkfk517y round 5: the walk toward
			// Cerulean's east border crossed the Badge House door at (9,11),
			// and the far door (9,9) is a genuine dead pocket with no route
			// back out except through the same house, so every re-plan after
			// that kept failing the same unreachable border search. Ban every
			// other warp tile from the path exactly like warpTarget already
			// does for a warp approach.
			blocked = warpAvoidance(h, int(x), int(y), blocked)
			tx, ty, err := edgeTargetForConnection(grid, e, int(x), int(y), blocked)
			if err != nil {
				// Type it as ErrLegUnwalkable like the FindPath failure below:
				// Route 2's ledge makes the north edge unreachable from the
				// southern landing tile, and GoTo's per-tile ban is what
				// re-routes around it through the forest. Unwrapped, the
				// error is terminal and the only real route to Pewter dies.
				unwalkable = fmt.Errorf("skill: Traverse: map %02x: %v: %w", e.From, err, ErrLegUnwalkable)
				return nil, unwalkable
			}
			steps, err := world.FindPath(grid, int(x), int(y), tx, ty, blocked)
			if err != nil {
				unwalkable = fmt.Errorf("skill: Traverse: no route to edge tile (%d,%d) on map %02x: %v: %w",
					tx, ty, e.From, err, ErrLegUnwalkable)
				return nil, unwalkable
			}
			return steps, nil
		}, func(steps []world.Step) error { return WalkPath(m, steps) },
		func() { m.StepFrames(npcWaitFrames) })
	if err != nil {
		if err == unwalkable {
			return world.Step{}, err
		}
		// Normalize to ErrBattle like walkWithinMap does, so a caller
		// can test one sentinel no matter which layer was walking.
		if errors.Is(err, ErrBattleInterrupted) {
			x, y := playerXY(m)
			return world.Step{}, fmt.Errorf("skill: Traverse: battle on map %02x at (%d,%d): %w", e.From, x, y, ErrBattle)
		}
		return world.Step{}, fmt.Errorf("skill: Traverse: walk to edge on map %02x: %w", e.From, err)
	}
	return edgeDirStep(e.Dir), nil
}

// errDidNotCross reports that a held push exhausted crossBudget without the
// map flipping. It is a sentinel distinct from ErrBattle: the caller can
// retry with a different approach on this, but must propagate a battle.
var errDidNotCross = errors.New("skill: Traverse: did not cross within budget")

// pushAcrossEdge holds btn until wCurMap flips off e.From, or reports
// errDidNotCross if crossBudget runs out first. A wild encounter fires on
// the step that lands the walk on a tall-grass edge tile — Route 1's south
// edge is grass at x=10 and x=11, measured — and that step is WalkPath's
// LAST one, whose DecodeBattle check can land a few frames before the
// encounter fires. The push then holds its button inside a frozen battle and
// reads as "did not cross within 180 frames" (the 0c->00 swarm failure,
// measured 2026-08-29: the tile is walkable on both sides and crosses in 17
// frames when no battle fires). Returning ErrBattle normalizes it exactly
// like a battle on the walk: Travel fights it and re-plans from the same
// tile, where no second encounter can fire because the player is already
// standing on the grass.
func pushAcrossEdge(m *emu.Emu, e world.Edge, btn emu.Button) error {
	m.Press(btn)
	crossed := false
	for i := 0; i < crossBudget; i++ {
		if m.Peek8(sym.CurMap) != e.From {
			crossed = true
			break
		}
		if m.Peek8(sym.IsInBattle) != 0 {
			m.Release(btn)
			x, y := playerXY(m)
			return fmt.Errorf("skill: Traverse: %s: battle on map %02x at (%d,%d): %w",
				edgeName(e), e.From, x, y, ErrBattle)
		}
		m.StepFrame()
	}
	m.Release(btn)
	if !crossed {
		x, y := playerXY(m)
		return fmt.Errorf("skill: Traverse: %s did not cross within %d frames; still on map %02x at (%d,%d): %w",
			edgeName(e), crossBudget, m.Peek8(sym.CurMap), x, y, errDidNotCross)
	}
	return nil
}

// finishArrival waits for the destination map to load and the position to
// settle after a successful pushAcrossEdge.
func finishArrival(m *emu.Emu, e world.Edge) error {
	// Positive arrival facts: a map is actually loaded (non-zero dimensions,
	// part of Controllable) and the player is controllable on it.
	if _, err := m.StepUntil(arriveBudget, func(m *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(m, &mem)
		return state.Controllable(&mem)
	}); err != nil {
		x, y := playerXY(m)
		return fmt.Errorf("skill: Traverse: %s: player not controllable on map %02x after %d frames at (%d,%d)",
			edgeName(e), m.Peek8(sym.CurMap), arriveBudget, x, y)
	}

	if got := m.Peek8(sym.CurMap); got != e.To {
		return fmt.Errorf("skill: Traverse: %s: arrived on map %02x, want %02x", edgeName(e), got, e.To)
	}

	// After a map flip the tile position is transient: it carries the source
	// map's warp tile, then the destination's door tile, then the standing
	// position. Controllable passes before that settles, so wait until the
	// position has been unchanged for a few consecutive frames.
	if err := waitForPositionStable(m, positionStableBudget, positionStableFrames); err != nil {
		return fmt.Errorf("skill: Traverse: %s: %w", edgeName(e), err)
	}

	// waitForPositionStable only tracks (x,y): it never re-checks CurMap, so a
	// forced-scroll script that keeps running after Controllable first flips
	// true can carry the player back off e.To and let its settling position
	// on a DIFFERENT map read as "stable". MEASURED on Route 18 -> Route 17
	// (0x1d -> 0x1c, run-18ou0y2719oq33ly7rpykncxk4): the earlier CurMap check
	// above passed while Cycling Road's forced downhill descent was still
	// mid-flight, then that descent pushed the player back across the
	// boundary onto e.From's exact starting tile, where the position finally
	// stopped changing. The crossing never actually held, but every check
	// before this one only ever sampled while it looked like it had. Without
	// this, GoTo's navigationGuard sees "returned to a state already seen"
	// and reports an unrecoverable stall instead of the ordinary "this leg
	// does not hold from here" the router already knows how to route around.
	if got := m.Peek8(sym.CurMap); got != e.To {
		x, y := playerXY(m)
		if got == e.From {
			return fmt.Errorf("skill: Traverse: %s: settled back on map %02x at (%d,%d), never held %02x: %w",
				edgeName(e), got, x, y, e.To, ErrLegBouncesBack)
		}
		return fmt.Errorf("skill: Traverse: %s: settled back on map %02x at (%d,%d), never held %02x: %w",
			edgeName(e), got, x, y, e.To, ErrLegUnwalkable)
	}
	return nil
}

// waitForPositionStable steps frames until the player's tile position has been
// unchanged for stableFrames consecutive frames, or the budget is exhausted.
func waitForPositionStable(m *emu.Emu, budget, stableFrames int) error {
	lastX, lastY := playerXY(m)
	stable := 0
	for i := 0; i < budget; i++ {
		m.StepFrame()
		x, y := playerXY(m)
		if x == lastX && y == lastY {
			stable++
			if stable >= stableFrames {
				return nil
			}
		} else {
			stable = 0
		}
		lastX, lastY = x, y
	}
	x, y := playerXY(m)
	return fmt.Errorf("position not stable within %d frames on map %02x at (%d,%d)",
		budget, m.Peek8(sym.CurMap), x, y)
}

// warpAvoidance extends blocked with every one of this map's warp tiles
// except the tile the player is standing on. A path search that does not
// know about warps can freely route across one on its way to some other
// tile, firing it and silently diverting the walk into whatever it leads to
// — the same hazard walkAroundAvoidingObjects's object blockers exist for,
// just for doors instead of sprites. Standing on a warp tile does not refire
// it (pokered only fires a warp on the step that arrives on it), so the
// current tile is exempt: a caller already there must be free to walk off
// it in any direction.
//
// Decomp-annotated "; inaccessible" warp-table entries are also exempt.
// Those occupy ordinary walkable floor but do not fire as exits — MEASURED
// on SILPH_CO_11F (5,5), where treating the inert teleporter as an eject
// tile forced every approach to the president through the Beauty at (10,5)
// and left GoTo with no capability-aware path.
func warpAvoidance(h rom.MapHeader, sx, sy int, blocked map[[2]int]bool) map[[2]int]bool {
	out := make(map[[2]int]bool, len(blocked)+len(h.Warps))
	for p, b := range blocked {
		out[p] = b
	}
	for _, w := range h.Warps {
		if int(w.X) == sx && int(w.Y) == sy {
			continue
		}
		if rom.IsInertWarp(h.ID, w.X, w.Y) {
			continue
		}
		out[[2]int{int(w.X), int(w.Y)}] = true
	}
	return out
}

// warpTarget picks the warp tile to cross. Among tiles that lead to e.To it
// uses walkable tiles when any exist, then takes the first one in ROM table
// order that the pathfinder can reach from (sx,sy). A destination made only
// of solid stair/warp tiles retains the push-to-enter behavior. Reachability
// is re-derived on every call from the current position — it is a fact about
// where the player stands right now, exactly like GoTo's per-tile leg bans,
// and is never cached as a property of the map or the warp.
//
// A warp tile is reachable when FindPathAdjacent finds a route to it whose
// walked tiles step on no other warp tile of this map: entering a warp tile
// fires that warp the frame the player steps on it, so a route through
// (5,0) cannot reach (4,0) — the player is already on the destination map
// before the push lands. That is the forest gate: both (4,0) and (5,0) warp
// to the forest, (4,0) is non-walkable, and the only path the pathfinder
// offers to it from the corridor runs through the (5,0) warp.
//
// The returned walk ends on a walkable tile orthogonally adjacent to the
// chosen warp; the push step enters it. The entry is the push the caller
// holds, never a WalkPath step: a mid-walk entry lands the re-plan on the
// destination map, against the source grid.
//
// A 0xFF (LAST_MAP) destination means "the map you came from." The graph
// resolved it when it built e: if e's own tile is a 0xFF warp, every 0xFF
// warp on this map resolves to e.To, so all of them lead to the target.

// edgeWarpCandidates lists the door/ladder tiles on h that Traverse may use
// for edge e. The filters match warpTarget: elevator scripts rewrite live
// destinations, paired door tiles may share adjacent landings, and LAST_MAP
// (0xFF) warps resolve to e.To when the edge tile itself is a LAST_MAP warp.
func edgeWarpCandidates(h rom.MapHeader, e world.Edge, romData []byte) []rom.Warp {
	lastMapDest, haveLastMap := uint8(0), false
	var targetWarp uint8
	haveTarget := false
	for _, w := range h.Warps {
		if w.X == e.WarpX && w.Y == e.WarpY {
			targetWarp, haveTarget = w.DestWarpID, true
		}
		if int(w.X) == int(e.WarpX) && int(w.Y) == int(e.WarpY) && w.DestMap == 0xFF {
			lastMapDest, haveLastMap = e.To, true
		}
	}

	_, _, _, elevatorEdge := rom.ElevatorFloorForDestination(e.From, e.To)
	var candidates []rom.Warp
	destHeader, destErr := rom.ParseMap(romData, e.To)
	for _, w := range h.Warps {
		if elevatorEdge {
			candidates = append(candidates, w)
			continue
		}
		equivalent := w.DestWarpID == targetWarp
		if !equivalent && destErr == nil && int(targetWarp) < len(destHeader.Warps) && int(w.DestWarpID) < len(destHeader.Warps) {
			a, b := destHeader.Warps[targetWarp], destHeader.Warps[w.DestWarpID]
			equivalent = absInt(int(w.X)-int(e.WarpX))+absInt(int(w.Y)-int(e.WarpY)) == 1 && absInt(int(a.X)-int(b.X))+absInt(int(a.Y)-int(b.Y)) == 1
		}
		if !haveTarget || !equivalent {
			continue
		}
		dest := w.DestMap
		if dest == 0xFF {
			if !haveLastMap || lastMapDest != e.To {
				continue
			}
			dest = e.To
		}
		if dest == e.To {
			candidates = append(candidates, w)
		}
	}
	return candidates
}

// approachWarpWithFieldPath walks to an orthogonal neighbour of a warp that
// leads across e, using the same Cut/Surf local planner as walkWithinMap.
// Land-only FindPath still treats Cut trees as solid, so a sealed pocket
// (Celadon Gym's leader chamber) has no ordinary route to the door even when
// the party can legally Cut out. This is destination-aware: it ranks approach
// tiles by field-action cost and never cuts an arbitrary nearby tree.
func approachWarpWithFieldPath(m *emu.Emu, romData []byte, e world.Edge) error {
	if e.Kind != world.EdgeWarp {
		return world.ErrNoPath
	}
	if got := m.Peek8(sym.CurMap); got != e.From {
		return fmt.Errorf("skill: field-path warp approach on map %02x, edge starts on %02x", got, e.From)
	}
	h, err := rom.ParseMap(romData, e.From)
	if err != nil {
		return err
	}
	candidates := edgeWarpCandidates(h, e, romData)
	if len(candidates) == 0 {
		return world.ErrNoPath
	}

	sx, sy := playerXY(m)
	blocked := spriteBlockers(m)
	blocked = warpAvoidance(h, int(sx), int(sy), blocked)

	type rankedApproach struct {
		dest    Destination
		actions int
		moves   int
	}
	var best *rankedApproach
	for _, w := range candidates {
		for _, step := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
			ax, ay := int(w.X)+step.DX, int(w.Y)+step.DY
			if ax < 0 || ay < 0 || ax > 255 || ay > 255 {
				continue
			}
			dest := Destination{Map: e.From, X: uint8(ax), Y: uint8(ay)}
			plan, perr := currentFieldPathPlan(m, romData, h, dest, blocked)
			if perr != nil {
				continue
			}
			actions := 0
			for _, p := range plan {
				if p.Action != fieldPathWalk {
					actions++
				}
			}
			cand := rankedApproach{dest: dest, actions: actions, moves: len(plan)}
			if best == nil || cand.actions < best.actions || (cand.actions == best.actions && cand.moves < best.moves) {
				best = &cand
			}
		}
	}
	if best == nil {
		return world.ErrNoPath
	}
	return walkWithinMap(m, romData, best.dest)
}

func warpTarget(h rom.MapHeader, e world.Edge, g *world.Grid, sx, sy int, blocked map[[2]int]bool, excludeWarp map[[2]int]bool, romData []byte) (wx, wy int, steps []world.Step, push world.Step, err error) {
	warpTile := make(map[[2]int]bool, len(h.Warps))
	for _, w := range h.Warps {
		warpTile[[2]int{int(w.X), int(w.Y)}] = true
	}
	approachBlocked := warpAvoidance(h, sx, sy, blocked)
	candidates := edgeWarpCandidates(h, e, romData)

	// The player may already be standing on one of this destination's warp
	// tiles: a resumed checkpoint whose previous leg warped in and landed
	// exactly on the door back out (Rocket Hideout's B4F<->elevator alcove,
	// measured). Red only re-fires a warp by tile ID on the step that ARRIVES
	// on it (IsPlayerStandingOnDoorTileOrWarpTile, pokered
	// engine/overworld/doors.asm); shuffling to a neighboring tile of the same
	// door is not an arrival and never fires it, so this would otherwise walk
	// back and forth between the pad's tiles forever. wMovementFlags'
	// BIT_STANDING_ON_WARP is set once on entering a map already on a warp
	// tile (pokered engine/overworld/player_state.asm) and stays set until a
	// collision — bumping a wall — fires the warp through ExtraWarpCheck
	// instead (pokered home/overworld.asm). Reproduce that: push toward
	// whichever neighbor is not walkable rather than walking anywhere.
	for _, w := range candidates {
		if int(w.X) != sx || int(w.Y) != sy {
			continue
		}
		if excludeWarp[[2]int{int(w.X), int(w.Y)}] {
			continue
		}
		for _, s := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
			if !g.Walkable(sx+s.DX, sy+s.DY) {
				return sx, sy, nil, s, nil
			}
		}
	}

	var reasons []string
	allNoPath := true
	// Prefer warp tiles the collision grid says can actually be entered.
	// Some stairs are intentionally solid and still activate on a push, so
	// use solid candidates only when this destination has no walkable tile.
	useWalkable := false
	for _, w := range candidates {
		if g.Walkable(int(w.X), int(w.Y)) {
			useWalkable = true
			break
		}
	}
	for _, w := range candidates {
		wx, wy = int(w.X), int(w.Y)
		if g.Walkable(wx, wy) != useWalkable {
			continue
		}
		if blocked[[2]int{wx, wy}] {
			continue
		}
		if excludeWarp[[2]int{wx, wy}] {
			continue
		}
		steps, push, err = world.FindPathAdjacent(g, sx, sy, wx, wy, approachBlocked)
		if err != nil {
			if !errors.Is(err, world.ErrNoPath) {
				allNoPath = false
			}
			reasons = append(reasons, fmt.Sprintf("warp (%d,%d): %v", wx, wy, err))
			continue
		}
		if routeCrossesWarp(steps, sx, sy, warpTile) {
			allNoPath = false
			reasons = append(reasons, fmt.Sprintf("warp (%d,%d): every route steps on another warp tile", wx, wy))
			continue
		}
		return wx, wy, steps, push, nil
	}
	if len(reasons) > 0 {
		// Callers such as rocketB1FExitReachableOnGrid use errors.Is against
		// world.ErrNoPath to tell "not reachable yet" apart from a real
		// failure. Joining every candidate's reason into one plain string
		// (MEASURED on run-27dtzi7qnqt962i4ecootzj8tg) used to drop that
		// sentinel even when every candidate failed with exactly ErrNoPath,
		// turning a normal "still behind the door" result into an
		// unknown_failure. Keep the sentinel wrapped whenever it still
		// applies to every candidate.
		if allNoPath {
			return 0, 0, nil, world.Step{}, fmt.Errorf("%s: %w", strings.Join(reasons, "; "), world.ErrNoPath)
		}
		return 0, 0, nil, world.Step{}, fmt.Errorf("%s", strings.Join(reasons, "; "))
	}
	return 0, 0, nil, world.Step{}, fmt.Errorf("no warp on map %02x leads to %02x", e.From, e.To)
}

// routeCrossesWarp reports whether the walk given by steps from (sx,sy)
// lands on any tile in warps at any point. The start tile is exempt: the
// player may be standing on a warp tile (warp arrival does not re-fire it),
// and the path only leaves it.
func routeCrossesWarp(steps []world.Step, sx, sy int, warps map[[2]int]bool) bool {
	x, y := sx, sy
	for _, s := range steps {
		x += s.DX
		y += s.DY
		if warps[[2]int{x, y}] {
			return true
		}
	}
	return false
}

// edgeTarget picks the walkable tile on the map's connection edge with the
// shortest path from (sx,sy). Dir 0 (north) is y == 0, dir 1 (south) is
// y == height-1, dir 2 (west) is x == 0, dir 3 (east) is x == width-1, in
// game tile coordinates. Tiles are scanned in (y, x) order and only a
// strictly shorter path replaces the current best, so ties break toward the
// lowest y, then the lowest x.
func edgeTarget(g *world.Grid, dir uint8, sx, sy int, blocked map[[2]int]bool) (int, int, error) {
	var edge [][2]int
	switch dir {
	case 0:
		for x := 0; x < g.Width; x++ {
			edge = append(edge, [2]int{x, 0})
		}
	case 1:
		for x := 0; x < g.Width; x++ {
			edge = append(edge, [2]int{x, g.Height - 1})
		}
	case 2:
		for y := 0; y < g.Height; y++ {
			edge = append(edge, [2]int{0, y})
		}
	case 3:
		for y := 0; y < g.Height; y++ {
			edge = append(edge, [2]int{g.Width - 1, y})
		}
	default:
		return 0, 0, fmt.Errorf("skill: Traverse: unknown connection dir %d", dir)
	}

	var best [2]int
	bestLen := -1
	for _, t := range edge {
		if !g.Walkable(t[0], t[1]) || blocked[[2]int{t[0], t[1]}] {
			continue
		}
		steps, err := world.FindPath(g, sx, sy, t[0], t[1], blocked)
		if err != nil {
			continue
		}
		if bestLen >= 0 && len(steps) >= bestLen {
			continue
		}
		best, bestLen = t, len(steps)
	}
	if bestLen < 0 {
		return 0, 0, fmt.Errorf("skill: Traverse: no reachable walkable tile on the %s edge from (%d,%d)",
			dirName(dir), sx, sy)
	}
	return best[0], best[1], nil
}

func edgeDirStep(dir uint8) world.Step {
	switch dir {
	case 0:
		return world.StepUp
	case 1:
		return world.StepDown
	case 2:
		return world.StepLeft
	case 3:
		return world.StepRight
	}
	return world.Step{}
}

func dirName(dir uint8) string {
	switch dir {
	case 0:
		return "north"
	case 1:
		return "south"
	case 2:
		return "west"
	case 3:
		return "east"
	}
	return fmt.Sprintf("dir %d", dir)
}

func edgeName(e world.Edge) string {
	switch e.Kind {
	case world.EdgeWarp:
		return fmt.Sprintf("warp edge %02x->%02x via tile (%d,%d)", e.From, e.To, e.WarpX, e.WarpY)
	case world.EdgeConnection:
		return fmt.Sprintf("connection edge %02x->%02x via %s", e.From, e.To, dirName(e.Dir))
	}
	return fmt.Sprintf("edge %02x->%02x kind %d", e.From, e.To, e.Kind)
}
