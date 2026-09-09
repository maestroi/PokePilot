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

	// crossAttemptBudget bounds how many different edge tiles a connection
	// crossing will try before giving up: a tile the collision grid marks
	// walkable can still sit outside the ROM-aligned connecting seam and
	// never flip the map no matter how long the push is held (see Traverse).
	crossAttemptBudget = 8

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

// mergeBlocked returns a new set containing every tile from both inputs,
// leaving neither argument mutated.
func mergeBlocked(a, b map[[2]int]bool) map[[2]int]bool {
	if len(b) == 0 {
		return a
	}
	out := make(map[[2]int]bool, len(a)+len(b))
	for t := range a {
		out[t] = true
	}
	for t := range b {
		out[t] = true
	}
	return out
}

func Traverse(m *emu.Emu, romData []byte, e world.Edge) error {
	cur := m.Peek8(sym.CurMap)
	if cur != e.From {
		return fmt.Errorf("skill: Traverse: on map %02x, but edge starts on %02x", cur, e.From)
	}

	h, err := rom.ParseMap(romData, e.From)
	if err != nil {
		return fmt.Errorf("skill: Traverse: parse map %02x: %w", e.From, err)
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return fmt.Errorf("skill: Traverse: build live map %02x: %w", e.From, err)
	}

	// Position is re-read inside each plan below, not here: a re-plan
	// around an NPC starts from wherever the interrupted walk stopped.
	var push world.Step
	switch e.Kind {
	case world.EdgeWarp:
		var unwalkable error
		err := walkAround(func() error { return movementInterruption(m) }, func() map[[2]int]bool { return spriteBlockers(m) },
			func(blocked map[[2]int]bool) ([]world.Step, error) {
				// The tile is re-chosen on every re-plan, from wherever the
				// interrupted walk stopped: which warp tile is reachable is a
				// fact about where the player stands right now, never a property
				// of the map or the warp, so it is never cached.
				x, y := playerXY(m)
				_, _, steps, p, err := warpTarget(h, e, grid, int(x), int(y), blocked, romData)
				if err != nil {
					unwalkable = fmt.Errorf("skill: Traverse: no reachable warp to %02x from (%d,%d) on map %02x (edge tile %d,%d): %v: %w",
						e.To, x, y, e.From, e.WarpX, e.WarpY, err, ErrLegUnwalkable)
					return nil, unwalkable
				}
				push = p // the last plan's push direction is the one walked
				return steps, nil
			}, func(steps []world.Step) error { return WalkPath(m, steps) },
			func() { m.StepFrames(npcWaitFrames) })
		if err != nil {
			if err == unwalkable {
				return err
			}
			// Normalize to ErrBattle exactly as the connection branch does
			// below, so a caller can test one sentinel no matter which layer
			// was walking: a wild encounter on the walk to a warp is the same
			// recoverable event as one on the walk to an edge, and Travel (and
			// the llm loop) rely on ErrBattle to fight it and re-plan from
			// where the walk stopped.
			if errors.Is(err, ErrBattleInterrupted) {
				x, y := playerXY(m)
				return fmt.Errorf("skill: Traverse: battle on map %02x at (%d,%d): %w", e.From, x, y, ErrBattle)
			}
			return fmt.Errorf("skill: Traverse: walk to warp on map %02x: %w", e.From, err)
		}
	case world.EdgeConnection:
		// The edge tile is re-chosen on every re-plan, not just the path to
		// it: an NPC standing in a one-tile gap can make the nearest edge
		// tile unreachable while another one on the same edge is fine.
		//
		// A tile that edgeTarget picked as walkable can still fail to
		// actually flip the map: the collision grid marks it walkable but it
		// sits outside the ROM-aligned connecting seam (the seam is narrower
		// than the walkable run of the edge). Measured 2026-09-09 on Route
		// 3's north edge to Route 4: (62,0) is walkable and holds north for
		// the full budget with no battle and no crossing, while (61,0) one
		// tile over crosses in a handful of frames. deadTiles bans a tile
		// that proved dead so the next attempt picks a different one on the
		// same edge, mirroring warpTarget's multi-candidate search.
		deadTiles := map[[2]int]bool{}
		push = edgeDirStep(e.Dir)
		btn, ok := buttonFor(push)
		if !ok {
			return fmt.Errorf("skill: Traverse: invalid push step %s on %02x->%02x", push, e.From, e.To)
		}
		for attempt := 0; ; attempt++ {
			var unwalkable error
			err := walkAround(func() error { return movementInterruption(m) },
				func() map[[2]int]bool { return mergeBlocked(spriteBlockers(m), deadTiles) },
				func(blocked map[[2]int]bool) ([]world.Step, error) {
					x, y := playerXY(m)
					tx, ty, err := edgeTarget(grid, e.Dir, int(x), int(y), blocked)
					if err != nil {
						// Type it as ErrLegUnwalkable like the FindPath failure below:
						// Route 2's ledge makes the north edge unreachable from the
						// southern landing tile, and GoTo's per-tile ban is what
						// re-routes around it through the forest. Unwrapped, the
						// error is terminal and the only real route to Pewter dies.
						// Once every edge tile has proven dead this also fires,
						// correctly reporting the leg as unwalkable rather than
						// looping forever.
						unwalkable = fmt.Errorf("skill: Traverse: map %02x: %v: %w", e.From, err, ErrLegUnwalkable)
						return nil, unwalkable
					}
					// The start tile itself must never be in the blocked set
					// passed to FindPath: a tile banned as an edgeTarget
					// candidate (dead from a previous crossing attempt) can
					// still be exactly where the player is now standing,
					// re-planning after that attempt stopped, and a blocked
					// start tile makes FindPath report no route to anywhere.
					pathBlocked := blocked
					if blocked[[2]int{int(x), int(y)}] {
						pathBlocked = make(map[[2]int]bool, len(blocked))
						for t := range blocked {
							pathBlocked[t] = true
						}
						delete(pathBlocked, [2]int{int(x), int(y)})
					}
					steps, err := world.FindPath(grid, int(x), int(y), tx, ty, pathBlocked)
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
					return err
				}
				// Normalize to ErrBattle like walkWithinMap does, so a caller
				// can test one sentinel no matter which layer was walking.
				if errors.Is(err, ErrBattleInterrupted) {
					x, y := playerXY(m)
					return fmt.Errorf("skill: Traverse: battle on map %02x at (%d,%d): %w", e.From, x, y, ErrBattle)
				}
				return fmt.Errorf("skill: Traverse: walk to edge on map %02x: %w", e.From, err)
			}

			// The push watches for a battle as well as the map flip. A wild
			// encounter fires on the step that lands the walk on a tall-grass
			// edge tile — Route 1's south edge is grass at x=10 and x=11,
			// measured — and that step is WalkPath's LAST one, whose
			// DecodeBattle check can land a few frames before the encounter
			// fires. The push then holds its button inside a frozen battle
			// and reads as "did not cross within 180 frames" (the 0c->00
			// swarm failure, measured 2026-08-29: the tile is walkable on
			// both sides and crosses in 17 frames when no battle fires).
			// Returning ErrBattle normalizes it exactly like a battle on the
			// walk: Travel fights it and re-plans from the same tile, where
			// no second encounter can fire because the player is already
			// standing on the grass.
			m.Press(btn)
			crossed := false
			battled := false
			for i := 0; i < crossBudget; i++ {
				if m.Peek8(sym.CurMap) != e.From {
					crossed = true
					break
				}
				if m.Peek8(sym.IsInBattle) != 0 {
					battled = true
					break
				}
				m.StepFrame()
			}
			m.Release(btn)
			if battled {
				x, y := playerXY(m)
				return fmt.Errorf("skill: Traverse: %s: battle on map %02x at (%d,%d): %w",
					edgeName(e), e.From, x, y, ErrBattle)
			}
			if crossed {
				break
			}
			x, y := playerXY(m)
			if attempt+1 >= crossAttemptBudget {
				return fmt.Errorf("skill: Traverse: %s did not cross within %d frames; still on map %02x at (%d,%d)",
					edgeName(e), crossBudget, m.Peek8(sym.CurMap), x, y)
			}
			deadTiles[[2]int{int(x), int(y)}] = true
		}
	default:
		return fmt.Errorf("skill: Traverse: unknown edge kind %d on %02x->%02x", e.Kind, e.From, e.To)
	}

	if e.Kind == world.EdgeWarp {
		btn, ok := buttonFor(push)
		if !ok {
			return fmt.Errorf("skill: Traverse: invalid push step %s on %02x->%02x", push, e.From, e.To)
		}
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
			return fmt.Errorf("skill: Traverse: %s did not cross within %d frames; still on map %02x at (%d,%d)",
				edgeName(e), crossBudget, m.Peek8(sym.CurMap), x, y)
		}
	}

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
func warpTarget(h rom.MapHeader, e world.Edge, g *world.Grid, sx, sy int, blocked map[[2]int]bool, romData []byte) (wx, wy int, steps []world.Step, push world.Step, err error) {
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
	warpTile := make(map[[2]int]bool, len(h.Warps))
	approachBlocked := make(map[[2]int]bool, len(blocked)+len(h.Warps))
	for p, b := range blocked {
		approachBlocked[p] = b
	}
	for _, w := range h.Warps {
		warpTile[[2]int{int(w.X), int(w.Y)}] = true
		if int(w.X) != sx || int(w.Y) != sy {
			approachBlocked[[2]int{int(w.X), int(w.Y)}] = true
		}
	}

	var candidates []rom.Warp
	destHeader, destErr := rom.ParseMap(romData, e.To)
	for _, w := range h.Warps {
		// Equal destination maps do not make ladders interchangeable: their
		// landing warps can be in disconnected rooms on the same floor.
		equivalent := w.DestWarpID == targetWarp
		// Paired door tiles can name adjacent landing tiles. Preserve this
		// measured door equivalence, but never substitute a remote ladder.
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

	var reasons []string
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
		steps, push, err = world.FindPathAdjacent(g, sx, sy, wx, wy, approachBlocked)
		if err != nil {
			reasons = append(reasons, fmt.Sprintf("warp (%d,%d): %v", wx, wy, err))
			continue
		}
		if routeCrossesWarp(steps, sx, sy, warpTile) {
			reasons = append(reasons, fmt.Sprintf("warp (%d,%d): every route steps on another warp tile", wx, wy))
			continue
		}
		return wx, wy, steps, push, nil
	}
	if len(reasons) > 0 {
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

	// blocked bans a candidate edge tile from being chosen (a tile a caller
	// already proved dead, or a sprite standing on it), but must never ban
	// the start tile itself: the player is validly standing there right
	// now, and FindPath's start-tile check would otherwise report every
	// candidate unreachable, not just the banned one.
	pathBlocked := blocked
	if blocked[[2]int{sx, sy}] {
		pathBlocked = make(map[[2]int]bool, len(blocked))
		for t := range blocked {
			pathBlocked[t] = true
		}
		delete(pathBlocked, [2]int{sx, sy})
	}

	var best [2]int
	bestLen := -1
	for _, t := range edge {
		if !g.Walkable(t[0], t[1]) || blocked[[2]int{t[0], t[1]}] {
			continue
		}
		steps, err := world.FindPath(g, sx, sy, t[0], t[1], pathBlocked)
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
