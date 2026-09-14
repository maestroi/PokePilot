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

const (
	maxNavigationTransitions        = 64
	maxSemanticTransitionExecutions = 16
)

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

// legAt bans an edge from the exact tile it was taken from. Route 2 is the
// case that forced this: it connects Viridian to Pewter in one hop, but a
// ledge splits it across its full width, so that connection is unwalkable
// from the southern landing tile and perfectly walkable from the northern
// band the Viridian Forest exit leads to. Banning the edge outright — which
// this did until it was measured — makes the only real route to Pewter
// unplannable, because that route ends on the very edge that failed. So the
// ban is keyed by where it failed, and only the bans recorded at the
// current tile are handed to the planner.
type legAt struct {
	e    world.Edge
	m    uint8
	x, y uint8
}

// legFromMap bans an edge from an entire map, not just the one tile it was
// taken from. It exists for the case legAt cannot cover: a connection that
// always lands in a walkable component with no recorded onward edge, no
// matter which tile of the origin map the crossing starts from. Cerulean
// City <-> Route 4 <-> Cerulean's trashed house is this in the wild
// (run-dicjjitksq5s3txp8m0nzn4k9 round 16): the crossing into Route 4
// landed in the same dead-end component whether the walk to the border
// started at (0,19), (27,12), or (20,0), so a tile-scoped ban never matched
// the next attempt and the loop ran the full 17-transition guard budget
// before failing. A ban keyed by (edge, origin map) survives every tile the
// walker happens to approach from. Only safe for warps: a warp has one
// fixed source tile, so its outcome never depends on where the player
// approached from.
type legFromMap struct {
	e world.Edge
	m uint8
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

// blockVisitedMaps returns hard plus every first-hop edge that would return to
// a map this GoTo call has already departed from. hard is not mutated: the
// reverse ban is a PREFERENCE (don't bounce), while hard is measured geometry
// (this leg is unwalkable from this tile), and the caller drops one without
// the other.
//
// Banning only the single immediately-previous map (the original rule) misses
// longer bounce cycles: Cerulean City <-> Cerulean's trashed house <-> Route 4
// each look like "not the map I just left" from the other's perspective, so a
// route that treats either building as a component bridge can tick between
// the two forever, one hop of "progress" at a time, until the navigation
// guard's exact-position repeat happens to catch it (measured on
// run-dicjjitksq5s3txp8m0nzn4k9 round 16: 13 transitions bouncing
// 03->0f->03->3e->03->0f->03->3e before the guard fired). Banning every
// already-visited map as a preference closes the whole cycle, not just its
// last link.
func blockVisitedMaps(g *world.Graph, hard map[world.Edge]bool, current uint8, visited map[uint8]bool) map[world.Edge]bool {
	blocked := make(map[world.Edge]bool, len(hard))
	for e := range hard {
		blocked[e] = true
	}
	for _, e := range g.Edges[current] {
		if visited[e.To] {
			blocked[e] = true
		}
	}
	return blocked
}

// graphWithoutEdgesInto returns a shallow copy of g with every edge whose
// destination is a visited map removed from every map's edge list, not just
// the edges leaving `current`. It exists because FindRoutePlanAtDestination-
// WithCapabilities (via findRoute) only enforces a blocked/preferred edge set
// on the route's FIRST hop (see route.go's doc comment on FindRouteAvoiding):
// a soft ban fed in as `preferred` stops the walker's very next step from
// re-entering a visited map, but leaves the rest of the planned route free to
// cross back into one two or more hops later. BFS happily returns that
// "valid" full route since nothing downstream is banned, and GoTo, replanning
// after every single hop, keeps finding a fresh detour that legally avoids
// the immediate revisit while never actually making progress — the exact
// bounce blockVisitedMaps's own doc comment says it exists to close.
//
// MEASURED on run-2498k01zo2so83g93atmvf8f6x round 4: "go to route 22" from
// Route 4 (0f) plotted a full route that legally detoured through Cerulean's
// trashed house, Route 24, Route 25, and Cerulean's badge house — none of
// which lead anywhere near Route 22 — because each replan's first-hop-only
// ban let it dodge the direct return to Route 4/Cerulean while the deeper
// legs of each computed route still crossed back into them. Ten transitions
// later it landed on the exact tile it started from and the navigation guard
// fired. Removing visited-map edges from the WHOLE graph before planning
// makes a detour-only route genuinely fail with ErrNoRoute when no real
// forward path avoids revisiting, which hands off to the existing
// forcedRevisitBan/safeForcedBan fallback below — the mechanism already
// built to tell a real dead end from a revisit that is the only way through.
func graphWithoutEdgesInto(g *world.Graph, visited map[uint8]bool) *world.Graph {
	without := *g
	without.Edges = make(map[uint8][]world.Edge, len(g.Edges))
	for mapID, edges := range g.Edges {
		filtered := make([]world.Edge, 0, len(edges))
		for _, e := range edges {
			if !visited[e.To] {
				filtered = append(filtered, e)
			}
		}
		without.Edges[mapID] = filtered
	}
	return &without
}

// forcedRevisitBan decides whether a route computed WITHOUT the visited-maps
// preference is forced back into a map this call already departed from —
// evidence of a real bounce rather than a fresh route (see the goToWithTransitionExecutor
// dead-end comment for the two shapes measured in the wild: a one-way room,
// and a border crossing that always lands in the same isolated pocket
// regardless of which tile it is crossed at). ok is false when there is
// nothing to ban: no route, the first edge does not revisit, it is already
// banned, or banning it would seal the map's only remaining exit.
func forcedRevisitBan(g *world.Graph, retry []world.RouteStep, retryErr error, visitedMaps map[uint8]bool, deadEnds map[legFromMap]bool) (legFromMap, bool) {
	if retryErr != nil || len(retry) == 0 {
		return legFromMap{}, false
	}
	edge := retry[0].Edge
	if !visitedMaps[edge.To] {
		return legFromMap{}, false
	}
	forced := legFromMap{e: edge, m: edge.From}
	if deadEnds[forced] {
		return legFromMap{}, false
	}
	for _, e := range g.Edges[forced.m] {
		if e != forced.e && !deadEnds[legFromMap{e: e, m: forced.m}] {
			return forced, true
		}
	}
	return legFromMap{}, false
}

// safeForcedBan trials banning forced.e and reports whether the destination
// is still reachable without it. blockedHere applies only to a route's first
// hop by design (FindRouteAvoiding's doc comment: unwalkability from where
// the caller currently stands is not a property of the edge everywhere
// else), so adding forced.e to blockedHere alone is not a real ban for this
// check — a route that leaves forced.m and later returns to it would see the
// ban lifted on that later hop and route straight through the edge this is
// supposed to be testing without. The trial instead removes forced.e from
// forced.m's edge list outright, the same full-graph exclusion
// graphWithoutSemanticEdges already uses for capability-denied edges. g and
// blockedHere are not mutated.
//
// See the goToWithTransitionExecutor call site for why this check exists:
// forced.e can be the only capability-free path to dest, in which case
// banning it is never correct even though it looks exactly like the
// dead-end bounces this ban mechanism was built for.
func safeForcedBan(g *world.Graph, cur uint8, dest Destination, x, y uint8, blockedHere map[world.Edge]bool, forced legFromMap, prereqs world.RoutePrerequisites) ([]world.RouteStep, error, bool) {
	without := *g
	without.Edges = make(map[uint8][]world.Edge, len(g.Edges))
	for mapID, edges := range g.Edges {
		if mapID != forced.m {
			without.Edges[mapID] = edges
			continue
		}
		filtered := make([]world.Edge, 0, len(edges))
		for _, e := range edges {
			if e != forced.e {
				filtered = append(filtered, e)
			}
		}
		without.Edges[mapID] = filtered
	}
	route, err := world.FindRoutePlanAtDestinationWithCapabilities(
		&without, cur, dest.Map, int(x), int(y), int(dest.X), int(dest.Y), blockedHere, prereqs,
	)
	return route, err, err == nil
}

func newReplanExhaustedError(max int, cur, x, y uint8, dest Destination, last error) error {
	return fmt.Errorf("%w: %d re-plans from map %02x at (%d,%d) toward map %02x at (%d,%d), last leg: %w",
		ErrReplanExhausted, max, cur, x, y, dest.Map, dest.X, dest.Y, last)
}

// GoTo walks the player to dest, crossing maps as needed. The immutable graph
// is built once, but every planning pass overlays the current map's live WRAM
// block geometry before component routing. After every leg the current map and
// coordinates are re-read and the remaining route is re-planned, so an opened
// door, closed gate, or unexpected landing is observed rather than cached.
func GoTo(m *emu.Emu, romData []byte, dest Destination) error {
	return goToWithTransitionExecutor(m, romData, dest, newRedRouteTransitionExecutor(m, romData, nil))
}

func goToWithTransitionExecutor(m *emu.Emu, romData []byte, dest Destination, executor world.TransitionExecutor) error {
	g, err := world.BuildGraph(romData)
	if err != nil {
		return err
	}
	startX, startY := playerXY(m)
	guard := newNavigationGuard(dest, navigationState{
		Map: m.Peek8(sym.CurMap), X: startX, Y: startY,
	})

	failed := map[legAt]bool{}
	deadEnds := map[legFromMap]bool{}

	// A bound on re-plans. Each ban is a distinct (leg, tile) or (leg, map),
	// so this terminates on its own, but an unattended run should not
	// discover a pathological map by walking it for an hour.
	const maxReplans = 8
	replans := 0
	semanticExecutions := 0
	visitedMaps := map[uint8]bool{}

	for {
		if err := abortIfBattle(m); err != nil {
			return err
		}
		if err := waitOutScriptedMovement(m); err != nil {
			return err
		}
		cur := m.Peek8(sym.CurMap)
		x, y := playerXY(m)

		h, err := rom.ParseMap(romData, cur)
		if err != nil {
			return fmt.Errorf("skill: GoTo: parse live map %02x at (%d,%d): %w", cur, x, y, err)
		}
		liveGrid, err := liveMapGrid(m, romData, h)
		if err != nil {
			return fmt.Errorf("skill: GoTo: build live map %02x at (%d,%d): %w", cur, x, y, err)
		}
		routeGraph, err := g.WithMapGrid(cur, liveGrid)
		if err != nil {
			return fmt.Errorf("skill: GoTo: overlay live topology for map %02x: %w", cur, err)
		}
		blockedHere := map[world.Edge]bool{}
		for k := range failed {
			if k.m == cur && k.x == x && k.y == y {
				blockedHere[k.e] = true
			}
		}
		for k := range deadEnds {
			if k.m == cur {
				blockedHere[k.e] = true
			}
		}
		avoidingVisited := len(visitedMaps) > 0 && !visitedMaps[dest.Map]
		planGraph := routeGraph
		if avoidingVisited {
			planGraph = graphWithoutEdgesInto(routeGraph, visitedMaps)
		}

		var mem state.Mem
		state.Snapshot(m, &mem)
		prereqs := redRoutePrerequisites(routeGraph, romData, &mem)
		route, err := world.FindRoutePlanAtDestinationWithCapabilities(
			planGraph, cur, dest.Map, int(x), int(y), int(dest.X), int(dest.Y), blockedHere, prereqs,
		)
		// A dead-end map's only exit IS the reverse. Route 4's Pokemon
		// Center (map 0x44) has two warps and both land back on Route 4,
		// so banning the reverse bans every edge and the journey dies on
		// "world: no route" one step after the router deliberately routed
		// THROUGH the building to change walkable component (measured on
		// run-3uhjsoyo0gx3i12rdm7cjax5pl round 46). Going back out of a
		// one-way room is not a bounce, it is the only move, so retry
		// with the preference dropped and the measured bans kept. The
		// navigation guard still catches a real oscillation.
		if errors.Is(err, world.ErrNoRoute) && avoidingVisited {
			// Dropping the "don't revisit" preference and re-planning with only
			// the hard bans tells us what the ONLY forward move actually is. If
			// that move re-enters a map this call already departed from, it is
			// not a fresh route: it is the bounce the preference exists to
			// prevent, forced through because nothing else was possible. That
			// covers both shapes measured in the wild: a one-way room whose
			// every warp lands back where we came from (Mt Moon Center,
			// run-3uhjsoyo0gx3i12rdm7cjax5pl round 46), and a border crossing
			// that always lands in the same isolated pocket no matter which tile
			// of it you cross — MEASURED on Cerulean City <-> Route 4
			// (run-11cb1z9tng81m1nslvpe4yp65m): crossing from Cerulean tile
			// (0,18) landed at Route 4 (89,10) with no way onward but back, and
			// after leaving and re-approaching from a DIFFERENT Cerulean tile
			// (9,12), the crossing was offered again and landed in that same
			// pocket — a tile-scoped ban never catches a fresh tile, so the walk
			// kept finding new, unbanned ways back into the same dead end until
			// the guard's exact-position repeat finally fired, 18 hops in.
			// Banning the edge actually about to be retaken, from its own origin
			// map, closes every tile of that origin at once — a connection's
			// outcome usually depends on where it is crossed (Route 2's ledge
			// splits walkable from unwalkable tiles on the very same edge), but
			// a crossing we have MEASURED landing back in known territory needs
			// no further benefit of the doubt.
			//
			// Never ban a map's only remaining exit: deadEnds is keyed by (edge,
			// edge.From), so banning one forbids leaving that map by that edge
			// for the rest of this call; if every other edge from that map is
			// already banned too, this ban would seal it with no way out at all,
			// which can never be correct — the player got there somehow, and the
			// same way back out must stay legal. MEASURED on
			// run-3w2ibusy813gfmnierudpllie round 6: Route 24 has exactly two
			// edges (Cerulean, Route 25); Route 25 was already banned as a real
			// dead end, and banning the Cerulean edge next — fired while
			// standing back on Cerulean, which still had plenty of its OWN
			// untried edges and was never the problem — sealed Route 24
			// completely. The next visit to Route 24 then had nowhere at all to
			// go, and "go to route 2" died on "world: no route" even though
			// Route 24 -> Cerulean was the one genuinely open door.
			retry, retryErr := world.FindRoutePlanAtDestinationWithCapabilities(
				routeGraph, cur, dest.Map, int(x), int(y), int(dest.X), int(dest.Y), blockedHere, prereqs,
			)
			if forced, ok := forcedRevisitBan(routeGraph, retry, retryErr, visitedMaps, deadEnds); ok {
				// Banning forced.e is only safe if the destination stays
				// reachable without it. A crossing that always lands in a
				// dead-end pocket has other capability-free routes to fall
				// back on (that is what makes it a real dead end); a
				// crossing that merely bridges two components of the SAME
				// city on the way to the only remaining unblocked corridor
				// looks identical from here (both are "the only forward move
				// re-enters a map we departed"), but banning it strands the
				// destination behind whatever gated pivot happens to still be
				// in the graph (Route 9's Cut tree, in the wild:
				// run-2f9zwq0xjhcvp2wvui3kzvw12q, where the S.S. Anne leg
				// bounced Cerulean's trashed house <-> Cerulean City once,
				// this ban then sealed the ungated Route 5/Underground Path
				// leg to Vermilion, and the traveler failed on "missing
				// capabilities [can_cut]" via Route 9 despite never needing
				// Cut for this trip at all). safeForcedBan trials the ban
				// before committing: if the destination is only reachable
				// through a NEW missing capability (or not at all) once
				// forced.e is gone, the revisit was real progress, not a
				// bounce, so take it as-is instead of walling it off.
				if afterBan, afterBanErr, ok := safeForcedBan(routeGraph, cur, dest, x, y, blockedHere, forced, prereqs); ok {
					if replans++; replans > maxReplans {
						return newReplanExhaustedError(maxReplans, cur, x, y, dest, err)
					}
					deadEnds[forced] = true
					blockedHere[forced.e] = true
					retry, retryErr = afterBan, afterBanErr
				}
			}
			route, err = retry, retryErr
		}
		if err != nil {
			return fmt.Errorf("skill: GoTo: no route from map %02x at (%d,%d) to map %02x at (%d,%d): %w",
				cur, x, y, dest.Map, dest.X, dest.Y, err)
		}
		if len(route) == 0 {
			return walkWithinMap(m, romData, dest)
		}

		step := route[0]
		e := step.Edge
		if step.Transition != nil {
			execution, execErr := world.ExecuteTransition(executor, e, *step.Transition)
			if execErr != nil {
				return fmt.Errorf("skill: GoTo: %w", execErr)
			}
			if execution.Changed {
				semanticExecutions++
				if semanticExecutions > maxSemanticTransitionExecutions {
					return fmt.Errorf("skill: GoTo: %w", &world.TransitionExecutionError{
						Edge: e, Transition: *step.Transition, Cause: world.ErrTransitionExecutionStalled,
					})
				}
				continue // effect observed: discard stale route/topology and re-plan
			}
		}
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
		visitedMaps[e.From] = true
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
	// 0x21 is Route 22. (27,5) is the safe approach tile: the coordinate
	// trigger of the Route 22 rival battle is (29,4)/(29,5) —
	// Route22DefaultScript (pokered/scripts/Route22.asm:58) fires when
	// EVENT_ROUTE22_RIVAL_WANTS_BATTLE is set and the player stands on
	// either, starting the trainer battle with no dialogue box. The rival
	// object's home tile is (25,5) and its live position is (28,5). Landing
	// a journey on the trigger tile leaked the battle into the current
	// objective's boundary (run-2222o8lxtextzndtjyqs9hz0q). (27,5) is
	// walkable, adjacent to the rival (so TalkAt and the progression
	// approach both work from here), and reachable from the Viridian City
	// connection on the east edge. Measured 2026-09-14 (PROBE_MAP=0x21
	// PROBE_AT=29,5): row y=5 is open x21..x35.
	"route 22": {Map: 0x21, X: 27, Y: 5},
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

// interactionPlaces names targets owned by compound actions. They resolve via
// Place, but are not standalone travel objectives in PlaceNames.
var interactionPlaces = map[string]Destination{}

// Place maps a friendly name to a Destination.
func Place(name string) (Destination, bool) {
	d, ok := places[name]
	if !ok {
		d, ok = interactionPlaces[name]
	}
	return d, ok
}

// PlaceOnMap returns the named destination recorded for mapID, so a caller
// standing on a map can find the tile that map's objectives are written
// against without hardcoding coordinates a second time. Names are scanned in
// sorted order, so a map carrying more than one place resolves the same way
// every call. ok is false for a map with no named place.
func PlaceOnMap(mapID uint8) (Destination, bool) {
	for _, name := range PlaceNames() {
		if d := places[name]; d.Map == mapID {
			return d, true
		}
	}
	return Destination{}, false
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

// scriptedMovementBudget bounds waitOutScriptedMovement: a map script that
// holds wJoyIgnore while it drives the player itself (VermilionDock's SS
// Anne departure animation plus its own forced walk off the dock is the
// measured case: several ~120-frame delay loops for the smoke/horn/erase
// sequence, then a separate 3-tile simulated-joypad walk). It is a budget,
// not a prediction: exceeding it is an error carrying the same diagnostics
// Cutscene reports on timeout.
const scriptedMovementBudget = 4000

// waitOutScriptedMovement lets a map script that is driving the player
// itself finish before GoTo reads position or presses a direction.
//
// wJoyIgnore blocks our input exactly the way it blocks the pad during such
// a script, so a press GoTo sends while one is running does nothing: the
// tile-level walker sees the coordinate never change, reports the leg
// unwalkable, and GoTo permanently bans a leg that was never geometrically
// blocked. MEASURED on run-1xn9x6r8ubsh81iuw08xdoysxi round 28: landing back
// on the Vermilion dock after S.S. Anne sails fires
// VermilionDockSSAnneLeavesScript, which sets wJoyIgnore and does not clear
// it — clearing is the OWN later walk-out script's job — so GoTo tried to
// walk off the dock mid-cutscene, banned the only warp back to Vermilion
// City as unwalkable, and failed the whole run with "no route" once every
// edge was exhausted.
//
// This is deliberately generic: any map script that force-walks the player
// (Route 22's rival intro, the Oak's Parcel hand-over, ...) holds control
// the same way, and GoTo should wait it out rather than fight it, no matter
// which script it is. A battle or a text box is a different, already-handled
// condition, so this only waits when neither is up.
func waitOutScriptedMovement(m *emu.Emu) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.Controllable(&mem) || state.DecodeDialogue(&mem) != nil || state.DecodeBattle(&mem) != nil {
		return nil
	}
	if err := Cutscene(m, scriptedMovementBudget, func(*state.Mem) bool { return true }); err != nil {
		return fmt.Errorf("skill: GoTo: %w", err)
	}
	return nil
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
// current map, retrying around dynamic sprite obstacles up to maxRetries times.
// Its collision grid is decoded from the current wOverworldMap block buffer,
// so script-driven tile replacements are ordinary topology rather than
// learned blockers or story-specific collision patches.
func walkWithinMap(m *emu.Emu, romData []byte, dest Destination) error {
	cur := m.Peek8(sym.CurMap)
	sx, sy := playerXY(m)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return fmt.Errorf("skill: GoTo: parse map %02x at (%d,%d): %w", cur, sx, sy, err)
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return fmt.Errorf("skill: GoTo: build live map %02x at (%d,%d): %w", cur, sx, sy, err)
	}

	// planErr is the "no path at all" case: already described in full, so
	// it is returned as-is rather than re-wrapped as a walk failure.
	var planErr error
	err = walkAround(func() error { return movementInterruption(m) }, func() map[[2]int]bool { return spriteBlockers(m) },
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
	if err == nil {
		return nil
	}
	if err == planErr {
		return arriveBesideBlockedDestination(m, romData, dest, planErr)
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

// arriveBesideBlockedDestination is the last resort when walkAround's whole
// retry budget still finds no path to dest. A trainer that intercepts the
// player on the way in stays wherever the fight leaves it for the rest of
// the run — nothing here ever asks it to move again — so if dest is the
// tile a live sprite is actually standing on, the dead end is permanent.
// Landing beside it instead of failing forever matches the "stand beside a
// live object" contract besideDestination already gives every other
// approach in this package (TalkAt, Pickup). A dest blocked by real map
// geometry, with no sprite on it, is a genuine bug elsewhere rather than an
// interception, so that case still surfaces planErr unchanged.
func arriveBesideBlockedDestination(m *emu.Emu, romData []byte, dest Destination, planErr error) error {
	if !spriteBlockers(m)[[2]int{int(dest.X), int(dest.Y)}] {
		return planErr
	}
	beside, ok, err := besideDestination(m, romData, dest.X, dest.Y)
	if err != nil {
		return planErr
	}
	if !ok {
		return nil
	}
	if err := walkWithinMap(m, romData, beside); err != nil {
		return err
	}
	return nil
}
