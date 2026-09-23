package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/world"
)

// edgeTargetForConnection is edgeTarget constrained to the component-scoped
// source-border band selected by the route graph. This is the execution half
// of world.ConnectionBand: without it the router could choose the good Route 4
// landing component and Traverse would still walk to the nearest tile anywhere
// on the aggregate border, recreating the exact dead-end #307 describes.
func edgeTargetForConnection(g *world.Grid, e world.Edge, sx, sy int, blocked map[[2]int]bool) (int, int, error) {
	return edgeTargetForConnectionExcluding(g, e, sx, sy, blocked, nil)
}

func edgeTargetForConnectionExcluding(g *world.Grid, e world.Edge, sx, sy int, blocked, excluded map[[2]int]bool) (int, int, error) {
	start, end, scoped := world.ConnectionBand(e)
	if !scoped {
		return edgeTargetExcluding(g, e.Dir, sx, sy, blocked, excluded)
	}

	var edge [][2]int
	add := func(i int) {
		switch e.Dir {
		case 0:
			if i >= 0 && i < g.Width {
				edge = append(edge, [2]int{i, 0})
			}
		case 1:
			if i >= 0 && i < g.Width {
				edge = append(edge, [2]int{i, g.Height - 1})
			}
		case 2:
			if i >= 0 && i < g.Height {
				edge = append(edge, [2]int{0, i})
			}
		case 3:
			if i >= 0 && i < g.Height {
				edge = append(edge, [2]int{g.Width - 1, i})
			}
		}
	}
	if e.Dir > 3 {
		return 0, 0, fmt.Errorf("skill: Traverse: unknown connection dir %d", e.Dir)
	}
	for i := start; i <= end; i++ {
		add(i)
	}

	var best [2]int
	bestLen := -1
	for _, t := range edge {
		if !g.Walkable(t[0], t[1]) || blocked[[2]int{t[0], t[1]}] || excluded[[2]int{t[0], t[1]}] {
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
		return 0, 0, fmt.Errorf("skill: Traverse: no reachable walkable tile on the %s edge band %d..%d from (%d,%d)",
			dirName(e.Dir), start, end, sx, sy)
	}
	return best[0], best[1], nil
}

func edgeTargetExcluding(g *world.Grid, dir uint8, sx, sy int, blocked, excluded map[[2]int]bool) (int, int, error) {
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
		if !g.Walkable(t[0], t[1]) || blocked[t] || excluded[t] {
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
		return 0, 0, fmt.Errorf("skill: Traverse: no reachable walkable tile on the %s edge from (%d,%d)", dirName(dir), sx, sy)
	}
	return best[0], best[1], nil
}
