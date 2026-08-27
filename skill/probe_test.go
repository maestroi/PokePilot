package skill

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
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
//	PROBE_MAP=0x0d PROBE_ROUTE=0x2f go test ./skill -run TestProbe -v
//	PROBE_STATE=fixture/testdata/fixtures/post_starter.v4.state \
//		PROBE_ROUTE=0x02 go test ./skill -run TestProbe -v
//
// PROBE_STATE resolves relative to skill/ — go test runs a test in its own
// package directory, so a repo-root-relative path silently does not exist.
//
// PROBE_STATE is a save state (a fixture under
// skill/fixture/testdata/fixtures/, or one a run wrote). It answers the
// question the static probe cannot: where the player ACTUALLY is. The live
// map, tile, facing and controllable flag are read from that state's RAM,
// and PROBE_MAP and PROBE_AT then default to them — so "why did I end up
// here, and can I still get where I was going" is one command rather than a
// throwaway test that boots a fixture and prints.
//
// PROBE_MAP is a map id (0x0c or 12). Its warps and connections are always
// printed: which exit leads where is a fact in the ROM header, never
// something to assemble by hand. PROBE_ROUTE is another map id and reports
// the legs Travel would take to reach it; it needs no PROBE_AT, being a
// question about the map graph rather than about a tile.
//
// PROBE_AT is where you stand, and everything below it is optional with it.
// PROBE_TO reports a path to one tile instead of every map edge. PROBE_BLOCK
// is a ;-separated list of tiles to treat as occupied ("14,13;15,12"), which
// is how you ask what a sprite in the way actually costs you.
func TestProbe(t *testing.T) {
	spec, statePath := os.Getenv("PROBE_MAP"), os.Getenv("PROBE_STATE")
	if spec == "" && statePath == "" {
		t.Skip("set PROBE_MAP or PROBE_STATE to run the probe, e.g. PROBE_MAP=0x0c PROBE_AT=15,13")
	}
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM %s: %v", romPath, err)
	}

	// A save state answers where the player IS; PROBE_MAP and PROBE_AT then
	// default to it, so the common case names no coordinates at all. They
	// still override, which is how you ask "from where I am, what about
	// over there?".
	live := ""
	if statePath != "" {
		p := livePlayer(t, romPath, statePath)
		live = fmt.Sprintf("live: map %#04x (%d,%d) facing=%s controllable=%v",
			p.MapID, p.X, p.Y, p.Facing, p.Controllable)
		t.Log(live)
		if spec == "" {
			spec = fmt.Sprintf("%x", p.MapID)
		}
		if os.Getenv("PROBE_AT") == "" {
			t.Setenv("PROBE_AT", fmt.Sprintf("%d,%d", p.X, p.Y))
		}
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
	// The map's own exits, from the ROM header. Guessing which warp leads
	// where is the other half of what this probe exists to prevent: a map
	// has a handful of warps and at most four connections, so printing all
	// of them costs nothing and answers the question outright.
	t.Logf("map %#04x: %dx%d, %d warp(s), %d connection(s)",
		mapID, g.Width, g.Height, len(h.Warps), len(h.Connections))
	for _, w := range h.Warps {
		t.Logf("  warp (%d,%d) -> map %#04x warp %d", w.X, w.Y, w.DestMap, w.DestWarpID)
	}
	for _, c := range h.Connections {
		t.Logf("  connection %-5s -> map %#04x", dirName(c.Dir), c.MapID)
	}

	// PROBE_ROUTE answers "how do I get from this map to that one" with the
	// same search Travel uses, rather than with a chain of warps assembled
	// by hand. It needs no PROBE_AT: it is a question about the map graph.
	if spec := os.Getenv("PROBE_ROUTE"); spec != "" {
		to64, err := strconv.ParseUint(strings.TrimPrefix(spec, "0x"), 16, 8)
		if err != nil {
			t.Fatalf("PROBE_ROUTE %q is not a map id: %v", spec, err)
		}
		graph, err := world.BuildGraph(romData)
		if err != nil {
			t.Fatalf("build graph: %v", err)
		}
		route, err := world.FindRoute(graph, mapID, uint8(to64))
		if err != nil {
			t.Logf("route %#04x -> %#04x: %v", mapID, uint8(to64), err)
		}
		for i, e := range route {
			if e.Kind == world.EdgeWarp {
				t.Logf("  leg %d: map %#04x -warp(%d,%d)-> map %#04x", i+1, e.From, e.WarpX, e.WarpY, e.To)
			} else {
				t.Logf("  leg %d: map %#04x -%s edge-> map %#04x", i+1, e.From, dirName(e.Dir), e.To)
			}
			// Only the first leg: the rest start on maps nobody is standing
			// on, and whether they are walkable is a question about a tile
			// that does not exist yet. This is the leg that fails.
			if i != 0 || os.Getenv("PROBE_AT") == "" {
				continue
			}
			sx, sy := probeTile(t, "PROBE_AT")
			blocked := probeBlocked(t)
			if e.Kind == world.EdgeWarp {
				steps, err := world.FindPath(g, sx, sy, int(e.WarpX), int(e.WarpY), blocked)
				t.Logf("    from (%d,%d): %d step(s) to that warp, err=%v", sx, sy, len(steps), err)
				continue
			}
			tx, ty, err := edgeTarget(g, e.Dir, sx, sy, blocked)
			if err != nil {
				t.Logf("    from (%d,%d): that edge is unreachable: %v", sx, sy, err)
				continue
			}
			t.Logf("    from (%d,%d): nearest reachable tile on that edge is (%d,%d)", sx, sy, tx, ty)
		}
	}

	if os.Getenv("PROBE_AT") == "" {
		return // a topology question; no tile to stand on
	}
	sx, sy := probeTile(t, "PROBE_AT")
	blocked := probeBlocked(t)
	t.Logf("standing (%d,%d) walkable=%v, %d tile(s) treated as occupied",
		sx, sy, g.Walkable(sx, sy), len(blocked))

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

// liveState is where the player actually is, out of a save state: the half
// of "where can I walk" the ROM cannot answer. The static probe knows the
// geometry; only RAM knows which tile you are standing on after a walk went
// wrong.
type liveState struct {
	MapID        uint8
	X, Y         uint8
	Facing       string
	Controllable bool
}

func livePlayer(t *testing.T, romPath, statePath string) liveState {
	t.Helper()
	b, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("PROBE_STATE %s: %v", statePath, err)
	}
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatalf("open ROM %s: %v", romPath, err)
	}
	defer m.Close()
	if err := m.LoadState(b); err != nil {
		t.Fatalf("PROBE_STATE %s: LoadState: %v", statePath, err)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	p := state.DecodePlayer(&mem)
	return liveState{p.MapID, p.X, p.Y, p.Facing.String(), state.Controllable(&mem)}
}
