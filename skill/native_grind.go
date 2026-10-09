package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
)

// NativeGrindPair picks two adjacent floor tiles on the current native map to
// pace between while training: the pair nearest the player (breadth-first over
// the live grid) whose tiles are walkable both ways, are not warps, and are not
// occupied by a live object. Measured from the live block buffer, so a grind
// spot needs no hand-picked coordinates. On maps where every floor tile rolls
// encounters (caves, towers) any such pair is a valid hunt.
func NativeGrindPair(m *emu.Emu, romData []byte) (a, b [2]uint8, err error) {
	profile, err := nativeRoutingProfileFor(m)
	if err != nil {
		return a, b, err
	}
	provider := profile.NativeMapProvider(romData)
	if provider == nil {
		return a, b, fmt.Errorf("skill: grind pair: nil native map provider")
	}
	if _, err := waitLiveMapSettled(m, profile); err != nil {
		return a, b, err
	}
	here := profile.DecodeOverworld(m)
	grid, live, header, err := nativeLiveGrid(m, profile, provider, here.NativeMapID)
	if err != nil {
		return a, b, err
	}
	blocked := nativeRuntimeBlockers(live, header, nil)
	pair, ok := nearestGrindPair(grid, int(here.X), int(here.Y), blocked)
	if !ok {
		return a, b, fmt.Errorf("skill: grind pair: no two adjacent floor tiles reachable from (%d,%d) on map %#04x", here.X, here.Y, here.NativeMapID)
	}
	return [2]uint8{uint8(pair[0][0]), uint8(pair[0][1])}, [2]uint8{uint8(pair[1][0]), uint8(pair[1][1])}, nil
}

func nearestGrindPair(grid *world.NativeGrid, x, y int, blocked map[[2]int]bool) ([2][2]int, bool) {
	steps := [][2]int{{0, -1}, {-1, 0}, {1, 0}, {0, 1}}
	floor := func(c [2]int) bool {
		return grid.InBounds(c[0], c[1]) && grid.Walkable(c[0], c[1]) && !grid.WarpTriggers(c[0], c[1]) && !blocked[c]
	}
	start := [2]int{x, y}
	seen := map[[2]int]bool{start: true}
	queue := [][2]int{start}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		for _, d := range steps {
			next := [2]int{at[0] + d[0], at[1] + d[1]}
			if !grid.Passable(at[0], at[1], next[0], next[1]) {
				continue
			}
			if floor(at) && floor(next) && grid.Passable(next[0], next[1], at[0], at[1]) {
				return [2][2]int{at, next}, true
			}
			if !seen[next] && !blocked[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return [2][2]int{}, false
}
