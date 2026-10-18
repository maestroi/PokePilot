package world

import (
	"fmt"
	"sort"
)

// FindNativeStrengthSinkPush solves boulder-to-pit puzzles without walking
// through warp holes. Targets map *live* movable object slots to exact retail
// sink tiles. Each call returns only the FIRST push on a bounded solution;
// the caller must execute it in the ROM, settle any falling-boulder script,
// re-observe the cartridge and replan from the changed map.
//
// Search is breadth-first by push count. At each node, candidate pushes are
// tried in ascending approach-walk length so that among equal-depth solutions
// the returned first push prefers a short stand approach over a long detour
// (run-a018a6cmhnkr / triage f658a1d845ef36a5: a 27-step ice approach softlocked
// while a same-depth underfoot push remained legal).
func FindNativeStrengthSinkPush(grid *NativeGrid, player Point, movables []Movable,
	fixed map[[2]int]bool, sinks map[int]Point, maxStates int,
) (NativePush, error) {
	if grid == nil || len(movables) == 0 || len(sinks) == 0 ||
		!grid.InBounds(player.X, player.Y) {
		return NativePush{}, ErrNoPath
	}
	if maxStates <= 0 {
		maxStates = 10000
	}
	ordered, err := normalizeMovables(movables)
	if err != nil {
		return NativePush{}, err
	}
	// Source of truth for traversable pit locations is the live collision
	// table, not a hard-coded coordinate assumed safe by the solver.
	seenSink := false
	for _, movable := range ordered {
		goal, ok := sinks[movable.ID]
		if !ok {
			continue
		}
		if !grid.InBounds(goal.X, goal.Y) {
			return NativePush{}, fmt.Errorf("world: sink for boulder %d outside native grid", movable.ID)
		}
		tile, ok := grid.Tile(goal.X, goal.Y)
		if !ok || tile != 0x60 {
			return NativePush{}, fmt.Errorf("world: sink for boulder %d lacks COLL_PIT ($60)", movable.ID)
		}
		seenSink = true
	}
	if !seenSink {
		return NativePush{}, ErrNoPath
	}
	nodes := []nativePushState{{player: player, movables: ordered, parent: -1}}
	visited := map[string]bool{nativePushKey(player, ordered): true}
	for i := 0; i < len(nodes); i++ {
		n := nodes[i]
		blocked := nativePushBlockers(fixed, n.movables)
		delete(blocked, [2]int{n.player.X, n.player.Y})
		candidates := nativeSinkPushCandidates(grid, n, blocked, sinks)
		sort.SliceStable(candidates, func(a, b int) bool {
			if len(candidates[a].push.Walk) != len(candidates[b].push.Walk) {
				return len(candidates[a].push.Walk) < len(candidates[b].push.Walk)
			}
			if candidates[a].sink != candidates[b].sink {
				return candidates[a].sink
			}
			return false
		})
		for _, cand := range candidates {
			first := cand.push
			if n.parent >= 0 {
				first = n.first
			}
			if cand.sink {
				return first, nil
			}
			nextMovables := append([]Movable(nil), n.movables...)
			nextMovables[cand.movableIndex].Pos = cand.push.To
			key := nativePushKey(cand.push.From, nextMovables)
			if visited[key] {
				continue
			}
			visited[key] = true
			if len(nodes) >= maxStates {
				return NativePush{}, fmt.Errorf("world: sink puzzle search exceeded %d states: %w",
					maxStates, ErrPushPuzzleStateLimit)
			}
			nodes = append(nodes, nativePushState{
				player: cand.push.From, movables: nextMovables, parent: i, first: first,
			})
		}
	}
	return NativePush{}, ErrNoPath
}

type nativeSinkCandidate struct {
	push         NativePush
	movableIndex int
	sink         bool
}

func nativeSinkPushCandidates(
	grid *NativeGrid, n nativePushState, blocked map[[2]int]bool, sinks map[int]Point,
) []nativeSinkCandidate {
	out := make([]nativeSinkCandidate, 0, len(n.movables)*4)
	for j, movable := range n.movables {
		target, hasTarget := sinks[movable.ID]
		for _, direction := range nativeDirections {
			stand := Point{X: movable.Pos.X - direction.DX, Y: movable.Pos.Y - direction.DY}
			to := Point{X: movable.Pos.X + direction.DX, Y: movable.Pos.Y + direction.DY}
			if !grid.InBounds(stand.X, stand.Y) || !grid.InBounds(to.X, to.Y) ||
				blocked[[2]int{stand.X, stand.Y}] ||
				!grid.Passable(stand.X, stand.Y, movable.Pos.X, movable.Pos.Y) {
				continue
			}
			isSink := hasTarget && to == target
			// A pit is a warp and therefore blocked for *player* pathing.
			// It can receive only the matching boulder, not the player
			// or the other puzzle boulders.
			if !isSink && (blocked[[2]int{to.X, to.Y}] ||
				!grid.Passable(movable.Pos.X, movable.Pos.Y, to.X, to.Y)) {
				continue
			}
			if isSink {
				occupiedByBoulder := false
				for _, other := range n.movables {
					if other.Pos == to {
						occupiedByBoulder = true
						break
					}
				}
				if occupiedByBoulder {
					continue
				}
			}
			walk, err := FindNativePath(grid, n.player.X, n.player.Y, stand.X, stand.Y, blocked)
			if err != nil {
				continue
			}
			out = append(out, nativeSinkCandidate{
				movableIndex: j,
				sink:         isSink,
				push: NativePush{
					MovableID: movable.ID, Stand: stand,
					From: movable.Pos, To: to, Walk: walk, Direction: direction,
				},
			})
		}
	}
	return out
}
