package skill

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// TestProbe answers "can I get there from here?" without anyone reading a
// collision grid.
//
// Route 2 is 20x72 and Viridian Forest is 34x48. Reading either into a
// reasoning budget is hopeless and pointless: nothing is meant to read them
// but a BFS. Three agent-runner attempts on S5-3 died at 142k/150k context
// reconstructing reachability by hand and produced no commits; the fix was
// twenty lines once someone ran the search instead of simulating it.
//
// So: run the search, read the answer.
//
//	PROBE_MAP=0x0c PROBE_AT=15,13 go test ./skill -run TestProbe -v
//	PROBE_MAP=0x0c PROBE_AT=15,13 PROBE_TO=11,0 go test ./skill -run TestProbe -v
//
// PROBE_MAP is a map id (0x0c or 12). PROBE_AT is where you stand. PROBE_TO
// is optional: without it, every map edge is reported. PROBE_BLOCK is an
// optional ;-separated list of tiles to treat as occupied ("14,13;15,12"),
// which is how you ask what a sprite in the way actually costs you.
func TestProbe(t *testing.T) {
	spec := os.Getenv("PROBE_MAP")
	if spec == "" {
		t.Skip("set PROBE_MAP to run the probe, e.g. PROBE_MAP=0x0c PROBE_AT=15,13")
	}
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM %s: %v", romPath, err)
	}
	id64, err := strconv.ParseUint(strings.TrimPrefix(spec, "0x"), 16, 8)
	if err != nil {
		t.Fatalf("PROBE_MAP %q is not a map id: %v", spec, err)
	}
	mapID := uint8(id64)

	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		t.Fatalf("parse map %#04x: %v", mapID, err)
	}
	g, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("build map %#04x: %v", mapID, err)
	}
	sx, sy := probeTile(t, "PROBE_AT")
	blocked := probeBlocked(t)
	t.Logf("map %#04x: %dx%d, standing (%d,%d) walkable=%v, %d tile(s) treated as occupied",
		mapID, g.Width, g.Height, sx, sy, g.Walkable(sx, sy), len(blocked))

	// The ROM's own object table, so "is something standing there?" is
	// answered from data rather than from memory of a past run.
	for _, o := range h.Objects {
		if abs(int(o.X)-sx) <= 4 && abs(int(o.Y)-sy) <= 4 {
			t.Logf("  object sprite %d home tile (%d,%d)", o.SpriteID, o.X, o.Y)
		}
	}

	if to := os.Getenv("PROBE_TO"); to != "" {
		tx, ty := probeTile(t, "PROBE_TO")
		steps, err := world.FindPath(g, sx, sy, tx, ty, blocked)
		t.Logf("path (%d,%d) -> (%d,%d): %d step(s), err=%v", sx, sy, tx, ty, len(steps), err)
	} else {
		for dir := uint8(0); dir < 4; dir++ {
			tx, ty, err := edgeTarget(g, dir, sx, sy, blocked)
			if err != nil {
				t.Logf("%-5s edge: %v", dirName(dir), err)
				continue
			}
			t.Logf("%-5s edge: nearest reachable tile (%d,%d)", dirName(dir), tx, ty)
		}
	}
	t.Log(probeGrid(g, sx, sy, blocked))
}

// probeGrid renders a window around (sx,sy): '@' the player, '#' unwalkable,
// 'x' a tile the caller declared occupied, '.' open. A window, not the map:
// dumping 1440 tiles is the thing this exists to avoid.
func probeGrid(g *world.Grid, sx, sy int, blocked map[[2]int]bool) string {
	const radius = 8
	var b strings.Builder
	b.WriteString("grid window (@ = you, # = wall, x = occupied):\n")
	for y := sy - radius; y <= sy+radius; y++ {
		if y < 0 || y >= g.Height {
			continue
		}
		fmt.Fprintf(&b, "y=%3d ", y)
		for x := sx - radius; x <= sx+radius; x++ {
			switch {
			case x < 0 || x >= g.Width:
				b.WriteByte(' ')
			case x == sx && y == sy:
				b.WriteByte('@')
			case blocked[[2]int{x, y}]:
				b.WriteByte('x')
			case g.Walkable(x, y):
				b.WriteByte('.')
			default:
				b.WriteByte('#')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func probeTile(t *testing.T, env string) (int, int) {
	t.Helper()
	v := os.Getenv(env)
	x, y, ok := strings.Cut(v, ",")
	if !ok {
		t.Fatalf("%s=%q: want \"x,y\"", env, v)
	}
	xi, err := strconv.Atoi(strings.TrimSpace(x))
	if err != nil {
		t.Fatalf("%s: x: %v", env, err)
	}
	yi, err := strconv.Atoi(strings.TrimSpace(y))
	if err != nil {
		t.Fatalf("%s: y: %v", env, err)
	}
	return xi, yi
}

func probeBlocked(t *testing.T) map[[2]int]bool {
	t.Helper()
	out := map[[2]int]bool{}
	for _, tile := range strings.Split(os.Getenv("PROBE_BLOCK"), ";") {
		if strings.TrimSpace(tile) == "" {
			continue
		}
		x, y, ok := strings.Cut(tile, ",")
		if !ok {
			t.Fatalf("PROBE_BLOCK %q: want \"x,y;x,y\"", tile)
		}
		xi, err1 := strconv.Atoi(strings.TrimSpace(x))
		yi, err2 := strconv.Atoi(strings.TrimSpace(y))
		if err1 != nil || err2 != nil {
			t.Fatalf("PROBE_BLOCK %q: not a tile", tile)
		}
		out[[2]int{xi, yi}] = true
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
