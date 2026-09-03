package skill

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// Destination is a concrete place: a map and a standing position on it.
type Destination struct {
	Map  uint8
	X, Y uint8
}

// ErrBattle is returned by GoTo when a wild battle interrupts the route. GoTo
// never fights or flees; it aborts and reports the battle.
var ErrBattle = errors.New("skill: battle interrupted the route")

// ErrReplanExhausted reports that GoTo spent its whole re-plan budget and
// gave up. It is a terminal give-up, not a recoverable single-leg failure:
// it wraps the last failed leg's error (usually ErrLegUnwalkable) so a
// caller still sees WHY the last attempt died, but errors.Is on
// ErrReplanExhausted is the unambiguous "stop retrying" signal.
var ErrReplanExhausted = errors.New("skill: route re-plan budget exhausted")

// ErrNavigationStalled reports that GoTo is cycling through an already-seen
// player state or has crossed an unreasonable number of maps without reaching
// its destination. It aborts only the current navigation call.
var ErrNavigationStalled = errors.New("skill: navigation made no progress")

const maxNavigationTransitions = 64

type navigationState struct {
	Map  uint8
	X, Y uint8
}

type navigationGuard struct {
	dest        Destination
	seen        map[navigationState]bool
	trace       []navigationState
	transitions int
}

func newNavigationGuard(dest Destination, start navigationState) *navigationGuard {
	return &navigationGuard{
		dest:  dest,
		seen:  map[navigationState]bool{start: true},
		trace: []navigationState{start},
	}
}

func (g *navigationGuard) observe(now navigationState) error {
	g.transitions++
	g.trace = append(g.trace, now)
	if g.seen[now] {
		return fmt.Errorf("%w: repeated map %02x at (%d,%d) after %d transitions toward map %02x at (%d,%d); trace: %s",
			ErrNavigationStalled, now.Map, now.X, now.Y, g.transitions,
			g.dest.Map, g.dest.X, g.dest.Y, formatNavigationTrace(g.trace))
	}
	if g.transitions > maxNavigationTransitions {
		return fmt.Errorf("%w: exceeded %d successful map transitions at map %02x (%d,%d) toward map %02x at (%d,%d); trace: %s",
			ErrNavigationStalled, maxNavigationTransitions, now.Map, now.X, now.Y,
			g.dest.Map, g.dest.X, g.dest.Y, formatNavigationTrace(g.trace))
	}
	g.seen[now] = true
	return nil
}

func formatNavigationTrace(trace []navigationState) string {
	parts := make([]string, len(trace))
	for i, s := range trace {
		parts[i] = fmt.Sprintf("%02x(%d,%d)", s.Map, s.X, s.Y)
	}
	return strings.Join(parts, " -> ")
}

// blockImmediateReverse excludes every first-hop edge that would return to
// the map just left. It mutates only the caller's per-route block set; no
// reversal is learned as persistent geometry.
func blockImmediateReverse(g *world.Graph, blocked map[world.Edge]bool, current, previous uint8) {
	for _, e := range g.Edges[current] {
		if e.To == previous {
			blocked[e] = true
		}
	}
}

func newReplanExhaustedError(max int, cur, x, y uint8, dest Destination, last error) error {
	return fmt.Errorf("%w: %d re-plans from map %02x at (%d,%d) toward map %02x at (%d,%d), last leg: %w",
		ErrReplanExhausted, max, cur, x, y, dest.Map, dest.X, dest.Y, last)
}

// GoTo walks the player to dest, crossing maps as needed. The graph is built
// once; after every leg the current map and coordinates are re-read from RAM
// and the remaining route is re-planned, so a leg that lands the player
// somewhere unexpected is recovered from rather than assumed.
func GoTo(m *emu.Emu, romData []byte, dest Destination) error {
	g, err := world.BuildGraph(romData)
	if err != nil {
		return err
	}
	startX, startY := playerXY(m)
	guard := newNavigationGuard(dest, navigationState{
		Map: m.Peek8(sym.CurMap), X: startX, Y: startY,
	})

	// Legs the map graph offers but the tile-level pathfinder cannot walk
	// FROM A GIVEN TILE. Route 2 is the case that forced this: it connects
	// Viridian to Pewter in one hop, but a ledge splits it across its full
	// width, so that connection is unwalkable from the southern landing
	// tile and perfectly walkable from the northern band the Viridian
	// Forest exit leads to. Banning the edge outright — which this did
	// until it was measured — makes the only real route to Pewter
	// unplannable, because that route ends on the very edge that failed.
	// So the ban is keyed by where it failed, and only the bans recorded
	// at the current tile are handed to the planner.
	type legAt struct {
		e    world.Edge
		m    uint8
		x, y uint8
	}
	failed := map[legAt]bool{}

	// A bound on re-plans. Each ban is a distinct (leg, tile), so this
	// terminates on its own, but an unattended run should not discover a
	// pathological map by walking it for an hour.
	const maxReplans = 8
	replans := 0
	var previousMap uint8
	havePreviousMap := false

	for {
		if err := abortIfBattle(m); err != nil {
			return err
		}
		cur := m.Peek8(sym.CurMap)
		x, y := playerXY(m)

		blockedHere := map[world.Edge]bool{}
		for k := range failed {
			if k.m == cur && k.x == x && k.y == y {
				blockedHere[k.e] = true
			}
		}
		if havePreviousMap && dest.Map != previousMap {
			blockImmediateReverse(g, blockedHere, cur, previousMap)
		}

		route, err := world.FindRouteAtDestination(
			g, cur, dest.Map, int(x), int(y), int(dest.X), int(dest.Y), blockedHere,
		)
		if err != nil {
			return fmt.Errorf("skill: GoTo: no route from map %02x at (%d,%d) to map %02x at (%d,%d): %w",
				cur, x, y, dest.Map, dest.X, dest.Y, err)
		}
		if len(route) == 0 {
			return walkWithinMap(m, romData, dest)
		}

		e := route[0]
		if err := Traverse(m, romData, e); err != nil {
			k := legAt{e: e, m: cur, x: x, y: y}
			if errors.Is(err, ErrLegUnwalkable) && !failed[k] {
				if replans++; replans > maxReplans {
					return newReplanExhaustedError(maxReplans, cur, x, y, dest, err)
				}
				failed[k] = true
				continue // re-plan without this leg, from this tile
			}
			return fmt.Errorf("skill: GoTo: %w", err)
		}
		previousMap, havePreviousMap = e.From, true
		nowX, nowY := playerXY(m)
		if err := guard.observe(navigationState{
			Map: m.Peek8(sym.CurMap), X: nowX, Y: nowY,
		}); err != nil {
			return fmt.Errorf("skill: GoTo: %w", err)
		}
	}
}

// places is the single source of truth for the names Place accepts.
var places = map[string]Destination{
	"reds bedroom": {Map: 0x26, X: 3, Y: 6},
	"reds house":   {Map: 0x25, X: 3, Y: 2},
	"pallet town":  {Map: 0x00, X: 5, Y: 6},
	// 0x28 is Oak's lab. (5,3) is the open floor tile directly below Oak
	// (5,2); it is where GetStarter's cutscene leaves the player and it is
	// no NPC's home tile. The Pallet door into the lab is not a plain
	// warp: the lab's entry script force-walks the player on entry, so a
	// Travel that fails while on the lab is the normal entry and must be
	// resumed with Cutscene (OaksParcel in errand.go does this).
	"oak's lab":     {Map: 0x28, X: 5, Y: 3},
	"viridian city": {Map: 0x01, X: 23, Y: 26},
	// 0x0C is Route 1, between Viridian City (north) and Pallet Town
	// (south). (5,14) is open road in the map's middle, reachable from the
	// north-edge connection (measured 2026-08-28: PROBE_MAP=0x0c
	// PROBE_AT=5,14, nearest reachable north-edge tile (11,0)); the map's
	// tall grass is where Train grinds.
	"route 1": {Map: 0x0C, X: 5, Y: 14},
	// (3,3) is the tile BELOW the counter, not the counter itself: on map
	// 0x29 the nurse stands at (3,1) and (3,2) is a counter tile, which the
	// player can never stand on. Talking works across the counter.
	"viridian pokemon center": {Map: 0x29, X: 3, Y: 3},
	// 0x2A is the Viridian Mart. (2,5) is the open floor in front of the
	// counter: the clerk stands at (0,5) and (1,5) is a counter tile, which
	// the player can never stand on. (2,5) is also exactly where the entry
	// cutscene leaves the player: the city door warp lands at (3,7) and the
	// map script force-walks left 1, up 2, and the parcel box is shown from
	// that tile.
	"viridian mart": {Map: 0x2A, X: 2, Y: 5},
	// (8,71) sits in the open band of Route 2's south edge (x7-9), the
	// landing zone of the crossing from Viridian City's north edge (x17-19).
	"route 2": {Map: 0x0D, X: 8, Y: 71},
	// 0x21 is Route 22. (29,5) is the coordinate trigger of the Route 22
	// rival battle: Route22DefaultScript (pokered/scripts/Route22.asm:58)
	// fires when EVENT_ROUTE22_RIVAL_WANTS_BATTLE is set and the player
	// stands on (29,4) or (29,5); the rival object's home tile is (25,5).
	// Measured 2026-08-28 (PROBE_MAP=0x21 PROBE_AT=29,5): standable and
	// reachable from the Viridian City connection on the east edge; the
	// only other exit is the warp at (8,5) to Route 22 Gate.
	"route 22": {Map: 0x21, X: 29, Y: 5},
	// (14,8) is open plaza directly below the center door warp at (14,7).
	"pewter city": {Map: 0x02, X: 14, Y: 8},
	// 0x3a is the Pewter center, reached from Pewter City's door warp at
	// (13,25). It has the Viridian center's exact layout (tileset 0x06,
	// 7x4 blocks): the nurse (sprite 41) stands at (3,1) behind the counter
	// and (3,3) is the floor tile in front of it, the same stand-beside
	// pattern as the Viridian center's Place. NOTE: 0x34 — which Pewter
	// City's warps at (14,7) and (19,5) lead into — is NOT a center in this
	// ROM. It is a ticket-booth building whose clerk opens a "Would you
	// like to come in?" YES/NO box (¥50, "It's ¥50 for a child's ticket.")
	// that aborts Travel with ErrDialogueChoice, and whose nurse sprite is
	// hidden, so skill.Heal cannot run there. Measured 2026-08-29 by
	// answering the box in a dumped state and by parsing every map header
	// for the center layout.
	"pewter pokemon center": {Map: 0x3a, X: 3, Y: 3},
	// 0x36 is the gym, reached from Pewter City's door warp at (16,17).
	// Brock (sprite 12) stands at (4,1) in the top room and (4,2) is the
	// open floor tile directly below him.
	"pewter gym": {Map: 0x36, X: 4, Y: 2},
	// (17,43) is open floor in the forest's south. (16,43) is occupied by a
	// standing NPC, which the player can never walk onto.
	"viridian forest": {Map: 0x33, X: 17, Y: 43},
	// 0x0E is Route 3, between Pewter City (west edge) and Route 4 (north
	// edge). (59,1) is the far side past every trainer (they stand at
	// x<=33). Measured 2026-08-30: PROBE_MAP=0x0e PROBE_AT=0,9 PROBE_TO=59,1
	// reached it in 83 steps from the west entry; walkable, no object home
	// tile nearby. The north seam to Route 4 is x=57..63.
	"route 3": {Map: 0x0E, X: 59, Y: 1},
	// 0x0F is Route 4, between Mt. Moon (west) and Cerulean City (east).
	// (10,10) measured 2026-08-30: walkable, no object home tile, reached
	// in 13 steps from BOTH the Route 3 seam landing band (PROBE_AT=8,17)
	// and the cave exit (18,5). NOTE: this ROM's Route 4 is fragmented into
	// five disconnected walkable components; (10,10) is in the one that
	// holds the seam and the cave entrance. See RUNNOTES S8-6 before routing
	// anything else on this map.
	"route 4": {Map: 0x0F, X: 10, Y: 10},
	// 0x3B is Mt. Moon 1F. Route 4's warp (18,5) targets this map's warp 0,
	// which lands at (14,35); the exit warps (14,35)/(15,35) go back
	// outdoors (DestMap 0xFF resolves to Route 4). (20,18) measured
	// 2026-08-30: PROBE_MAP=0x3b PROBE_AT=14,35 PROBE_TO=20,18 reached it in
	// 23 steps; walkable, no object home tile nearby. Ladders down to B1F
	// at (5,5), (17,11), (25,15).
	"mt moon 1f": {Map: 0x3B, X: 20, Y: 18},
	// 0x3C is Mt. Moon B1F, reached from 1F's ladders (landing tiles (5,5),
	// (25,9), (25,15)). (14,16) measured 2026-08-30: PROBE_MAP=0x3c
	// PROBE_AT=5,5 PROBE_TO=14,16 reached it in 20 steps; walkable, no
	// object home tile nearby. Ladders down to B2F at (17,11), (21,17),
	// (13,27), (23,3). NOTE: this map's own exit warp (27,3) lands at Route
	// 4's (24,5), which sits in a sealed pocket of Route 4 that touches no
	// map edge — see RUNNOTES S8-6.
	"mt moon b1f": {Map: 0x3C, X: 14, Y: 16},
	// 0x3D is Mt. Moon B2F, reached from B1F's ladders (landing tiles
	// (25,9), (21,17), (15,27), (5,7)). (20,18) measured 2026-08-30:
	// PROBE_MAP=0x3d PROBE_AT=25,9 PROBE_TO=20,18 reached it in 14 steps;
	// walkable, no object home tile nearby.
	"mt moon b2f": {Map: 0x3D, X: 20, Y: 18},
	// 0x44 is the Mt. Moon Pokemon Center, reached from Route 4's warp
	// (11,5) -> warp 0 at (3,7). Same layout as the other Centers: the
	// nurse stands at (3,1) behind the counter and (3,3) is the floor tile
	// in front of it. Measured 2026-08-30: PROBE_MAP=0x44 PROBE_AT=3,7
	// PROBE_TO=3,3 reached it in 4 steps; walkable, no object home tile.
	"mt moon pokemon center": {Map: 0x44, X: 3, Y: 3},
	// 0x03 is Cerulean City, reached from Route 4's east edge. (5,18)
	// measured 2026-08-30: walkable, no object home tile, reached in 5
	// steps from the west seam landing (0,18) (PROBE_TO=5,18).
	"cerulean city": {Map: 0x03, X: 5, Y: 18},
	// 0x40 is the Cerulean Pokemon Center, reached from Cerulean City's
	// warp (19,17) -> warp 0 at (3,7). Same counter layout as the other
	// Centers: nurse at (3,1), (3,3) in front of it. Measured 2026-08-30:
	// PROBE_MAP=0x40 PROBE_AT=3,7 PROBE_TO=3,3 reached it in 4 steps;
	// walkable, no object home tile.
	"cerulean pokemon center": {Map: 0x40, X: 3, Y: 3},
	// 0x41 is the Cerulean Gym, reached from Cerulean City's warp (30,19)
	// -> warp 0 at (4,13). The gym leader stands at (4,2) — one row lower
	// than Brock's (4,1) — so the stand-beside tile is (4,3), directly
	// below him. Measured 2026-08-30: PROBE_MAP=0x41 PROBE_AT=4,13
	// PROBE_TO=4,3 reached it in 16 steps; walkable, and (4,2) is the
	// leader's home tile so the destination is one row below it.
	"cerulean gym": {Map: 0x41, X: 4, Y: 3},
}

// Place maps a friendly name to a Destination.
func Place(name string) (Destination, bool) {
	d, ok := places[name]
	return d, ok
}

// PlaceNames returns every name Place accepts, sorted, so a caller can offer
// one objective per place without duplicating the list.
func PlaceNames() []string {
	names := make([]string, 0, len(places))
	for name := range places {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// abortIfBattle returns an error wrapping ErrBattle when a battle is active,
// carrying the current map and coordinates.
func abortIfBattle(m *emu.Emu) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) != nil {
		x, y := playerXY(m)
		return fmt.Errorf("skill: GoTo: battle on map %02x at (%d,%d): %w",
			m.Peek8(sym.CurMap), x, y, ErrBattle)
	}
	return nil
}

// walkWithinMap walks the player from their current position to dest on the
// current map, retrying around dynamic obstacles (tiles the static collision
// grid does not know about) up to maxRetries times.
func walkWithinMap(m *emu.Emu, romData []byte, dest Destination) error {
	cur := m.Peek8(sym.CurMap)
	sx, sy := playerXY(m)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return fmt.Errorf("skill: GoTo: parse map %02x at (%d,%d): %w", cur, sx, sy, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return fmt.Errorf("skill: GoTo: build map %02x at (%d,%d): %w", cur, sx, sy, err)
	}

	// planErr is the "no path at all" case: already described in full, so
	// it is returned as-is rather than re-wrapped as a walk failure.
	var planErr error
	err = walkAround(func() map[[2]int]bool { return spriteBlockers(m) },
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			x, y := playerXY(m)
			steps, err := world.FindPath(grid, int(x), int(y), int(dest.X), int(dest.Y), blocked)
			if err != nil {
				planErr = fmt.Errorf("skill: GoTo: no path on map %02x from (%d,%d) to (%d,%d): %w",
					cur, x, y, dest.X, dest.Y, err)
				return nil, planErr
			}
			return steps, nil
		}, func(steps []world.Step) error { return WalkPath(m, steps) },
		func() { m.StepFrames(npcWaitFrames) })
	if err == nil || err == planErr {
		return err
	}
	x, y := playerXY(m)
	if errors.Is(err, ErrBattleInterrupted) {
		return fmt.Errorf("skill: GoTo: battle on map %02x at (%d,%d): %w", cur, x, y, ErrBattle)
	}
	var eb *ErrBlocked
	if errors.As(err, &eb) {
		return fmt.Errorf("skill: GoTo: blocked on map %02x at (%d,%d) after %d retries: %w",
			cur, eb.At.X, eb.At.Y, maxWalkRetries, err)
	}
	return fmt.Errorf("skill: GoTo: walk on map %02x at (%d,%d): %w", cur, x, y, err)
}
