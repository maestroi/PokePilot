package world

import (
	"fmt"
	"strconv"
	"strings"
)

// NativePush is the first verified push in a native (wide-map) Strength plan.
// The caller executes it, re-observes live sprite positions, then replans;
// planned later pushes are never blindly replayed against mutable map state.
type NativePush struct {
	MovableID int
	Stand     Point
	From      Point
	To        Point
	Walk      []NativeStep
	Direction NativeStep
}

type nativePushState struct {
	player   Point
	movables []Movable
	parent   int
	first    NativePush
}

func nativePushBlockers(fixed map[[2]int]bool, movables []Movable) map[[2]int]bool {
	out := make(map[[2]int]bool, len(fixed)+len(movables))
	for p, yes := range fixed {
		if yes {
			out[p] = true
		}
	}
	for _, m := range movables {
		out[[2]int{m.Pos.X, m.Pos.Y}] = true
	}
	return out
}

func nativePushKey(player Point, movables []Movable) string {
	var b strings.Builder
	b.Grow(16 + len(movables)*14)
	b.WriteString(strconv.Itoa(player.X))
	b.WriteByte(',')
	b.WriteString(strconv.Itoa(player.Y))
	for _, m := range movables {
		b.WriteByte('|')
		b.WriteString(strconv.Itoa(m.ID))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(m.Pos.X))
		b.WriteByte(',')
		b.WriteString(strconv.Itoa(m.Pos.Y))
	}
	return b.String()
}

// FindNativeStrengthPush performs a bounded breadth-first search over
// successive boulder positions and player positions. A successful state is
// one from which ANY requested goal tile is actually reachable without
// walking through a boulder or unrelated sprite. It returns only the first
// required push; callers must replan after the ROM performs that push.
//
// No game-specific tile values, sprite ids, badges or map ids live here.
func FindNativeStrengthPush(
	grid *NativeGrid,
	player Point,
	movables []Movable,
	fixed map[[2]int]bool,
	goals []Point,
	maxStates int,
) (NativePush, error) {
	if grid == nil || len(movables) == 0 || len(goals) == 0 || !grid.InBounds(player.X, player.Y) {
		return NativePush{}, ErrNoPath
	}
	if maxStates <= 0 {
		maxStates = 1500
	}
	ordered, err := normalizeMovables(movables)
	if err != nil {
		return NativePush{}, err
	}
	for _, movable := range ordered {
		if !grid.Walkable(movable.Pos.X, movable.Pos.Y) {
			return NativePush{}, fmt.Errorf("world: native Strength boulder %d is not on navigable floor", movable.ID)
		}
	}
	nodes := []nativePushState{{player: player, movables: ordered, parent: -1}}
	seen := map[string]bool{nativePushKey(player, ordered): true}
	for i := 0; i < len(nodes); i++ {
		n := nodes[i]
		blocked := nativePushBlockers(fixed, n.movables)
		for _, goal := range goals {
			if !grid.InBounds(goal.X, goal.Y) || blocked[[2]int{goal.X, goal.Y}] {
				continue
			}
			if _, err := FindNativePath(grid, n.player.X, n.player.Y, goal.X, goal.Y, blocked); err == nil {
				if i == 0 {
					return NativePush{}, ErrNoPath // No Strength push is necessary.
				}
				for nodes[i].parent > 0 {
					i = nodes[i].parent
				}
				return nodes[i].first, nil
			}
		}
		if len(nodes) >= maxStates {
			return NativePush{}, fmt.Errorf("world: native Strength search exceeded %d states: %w", maxStates, ErrPushPuzzleStateLimit)
		}
		for j, movable := range n.movables {
			for _, direction := range nativeDirections {
				stand := Point{X: movable.Pos.X - direction.DX, Y: movable.Pos.Y - direction.DY}
				to := Point{X: movable.Pos.X + direction.DX, Y: movable.Pos.Y + direction.DY}
				if !grid.InBounds(stand.X, stand.Y) || !grid.Walkable(to.X, to.Y) ||
					blocked[[2]int{stand.X, stand.Y}] || blocked[[2]int{to.X, to.Y}] ||
					!grid.Passable(stand.X, stand.Y, movable.Pos.X, movable.Pos.Y) ||
					!grid.Passable(movable.Pos.X, movable.Pos.Y, to.X, to.Y) {
					continue
				}
				walk, err := FindNativePath(grid, n.player.X, n.player.Y, stand.X, stand.Y, blocked)
				if err != nil {
					continue
				}
				nextMovables := append([]Movable(nil), n.movables...)
				nextMovables[j].Pos = to
				nextPlayer := movable.Pos
				key := nativePushKey(nextPlayer, nextMovables)
				if seen[key] {
					continue
				}
				seen[key] = true
				push := NativePush{
					MovableID: movable.ID,
					Stand: stand, From: movable.Pos, To: to,
					Walk: walk, Direction: direction,
				}
				nodes = append(nodes, nativePushState{
					player: nextPlayer, movables: nextMovables,
					parent: i, first: push,
				})
			}
		}
	}
	return NativePush{}, ErrNoPath
}
