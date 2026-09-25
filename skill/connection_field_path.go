package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func connectionFieldTargets(g *world.Grid, e world.Edge) [][2]int {
	if g == nil || e.Kind != world.EdgeConnection || e.Dir > 3 {
		return nil
	}
	start, end, scoped := world.ConnectionBand(e)
	if !scoped {
		if e.Dir < 2 {
			start, end = 0, g.Width-1
		} else {
			start, end = 0, g.Height-1
		}
	}
	out := make([][2]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		switch e.Dir {
		case 0:
			if i >= 0 && i < g.Width {
				out = append(out, [2]int{i, 0})
			}
		case 1:
			if i >= 0 && i < g.Width {
				out = append(out, [2]int{i, g.Height - 1})
			}
		case 2:
			if i >= 0 && i < g.Height {
				out = append(out, [2]int{0, i})
			}
		case 3:
			if i >= 0 && i < g.Height {
				out = append(out, [2]int{g.Width - 1, i})
			}
		}
	}
	return out
}

// approachConnectionWithFieldPath reaches the selected connection's actual
// source band with ordinary local pathing plus Cut/Surf/Strength/forced
// movement. It is invoked only after land-only Traverse proved the band
// unreachable, so field actions remain demand-driven and edge-specific.
func approachConnectionWithFieldPath(m *emu.Emu, romData []byte, e world.Edge) error {
	if e.Kind != world.EdgeConnection {
		return world.ErrNoPath
	}
	if got := m.Peek8(sym.CurMap); got != e.From {
		return fmt.Errorf("skill: field-path connection approach on map %02x, edge starts on %02x", got, e.From)
	}
	h, err := rom.ParseMap(romData, e.From)
	if err != nil {
		return err
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return err
	}
	sx, sy := playerXY(m)
	blocked := currentObservedStationaryObjectBlockers(m, h)
	blocked = warpAvoidance(h, int(sx), int(sy), blocked)

	type ranked struct {
		dest    Destination
		actions int
		moves   int
	}
	var best *ranked
	for _, at := range connectionFieldTargets(grid, e) {
		if !grid.Walkable(at[0], at[1]) || blocked[at] {
			continue
		}
		dest := Destination{Map: e.From, X: uint8(at[0]), Y: uint8(at[1])}
		plan, perr := currentFieldPathPlan(m, romData, h, dest, blocked)
		if perr != nil {
			continue
		}
		actions := 0
		for _, step := range plan {
			if step.Action != fieldPathWalk {
				actions++
			}
		}
		cand := ranked{dest: dest, actions: actions, moves: len(plan)}
		if best == nil || cand.actions < best.actions || (cand.actions == best.actions && cand.moves < best.moves) {
			best = &cand
		}
	}
	if best == nil {
		return world.ErrNoPath
	}
	return walkWithinMap(m, romData, best.dest)
}
