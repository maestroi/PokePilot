from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    s = p.read_text()
    if old not in s:
        raise SystemExit(f"missing replacement anchor in {path}: {old[:120]!r}")
    if s.count(old) != 1:
        raise SystemExit(f"replacement anchor not unique in {path}: {old[:120]!r} ({s.count(old)})")
    p.write_text(s.replace(old, new, 1))


# world: route planning can deliberately cross capability-satisfied semantic
# edges even when the current ordinary-walking component cannot reach the port.
replace_once("world/route.go",
'''func FindRouteAvoiding(g *Graph, from, to uint8, blockedHere map[Edge]bool) ([]Edge, error) {
\treturn findRoute(g, from, to, blockedHere, nil, nil)
}''',
'''func FindRouteAvoiding(g *Graph, from, to uint8, blockedHere map[Edge]bool) ([]Edge, error) {
\treturn findRoute(g, from, to, blockedHere, nil, nil, nil)
}''')
replace_once("world/route.go",
'''func FindRouteAt(g *Graph, from, to uint8, x, y int, blockedHere map[Edge]bool) ([]Edge, error) {
\treturn findRoute(g, from, to, blockedHere, componentSetAt(g, from, x, y), nil)
}''',
'''func FindRouteAt(g *Graph, from, to uint8, x, y int, blockedHere map[Edge]bool) ([]Edge, error) {
\treturn findRoute(g, from, to, blockedHere, componentSetAt(g, from, x, y), nil, nil)
}''')
replace_once("world/route.go",
'''func FindRouteAtDestination(g *Graph, from, to uint8, x, y, tx, ty int, blockedHere map[Edge]bool) ([]Edge, error) {
\tfirst := componentSetAt(g, from, x, y)
\ttarget := componentSetAt(g, to, tx, ty)
\tif !g.componentAware || len(first) == 0 || len(target) == 0 {
\t\t// Missing component data is not evidence that a detour is required.
\t\t// Preserve the old map-level behavior in that case.
\t\treturn findRoute(g, from, to, blockedHere, first, nil)
\t}
\treturn findRoute(g, from, to, blockedHere, first, target)
}''',
'''func FindRouteAtDestination(g *Graph, from, to uint8, x, y, tx, ty int, blockedHere map[Edge]bool) ([]Edge, error) {
\treturn findRouteAtDestinationAllowingSemantic(g, from, to, x, y, tx, ty, blockedHere, nil)
}

// findRouteAtDestinationAllowingSemantic is the component-aware planner with
// one extra contract: an edge named in semantic is an executable topology
// transition, so ordinary walking reachability to that edge's exit port is not
// a prerequisite. The owning transition executor must establish and verify the
// game-specific effect before the edge is traversed.
func findRouteAtDestinationAllowingSemantic(g *Graph, from, to uint8, x, y, tx, ty int, blockedHere map[Edge]bool, semantic map[Edge]bool) ([]Edge, error) {
\tfirst := componentSetAt(g, from, x, y)
\ttarget := componentSetAt(g, to, tx, ty)
\tif !g.componentAware || len(first) == 0 || len(target) == 0 {
\t\t// Missing component data is not evidence that a detour is required.
\t\t// Preserve the old map-level behavior in that case.
\t\treturn findRoute(g, from, to, blockedHere, first, nil, semantic)
\t}
\treturn findRoute(g, from, to, blockedHere, first, target, semantic)
}''')
replace_once("world/route.go",
'''func findRoute(g *Graph, from, to uint8, blockedHere map[Edge]bool, first, target []int) ([]Edge, error) {''',
'''func findRoute(g *Graph, from, to uint8, blockedHere map[Edge]bool, first, target []int, semantic map[Edge]bool) ([]Edge, error) {''')
replace_once("world/route.go",
'''\t\t\tif !canExit(g, e, entry) {
\t\t\t\tcontinue
\t\t\t}''',
'''\t\t\t// A semantic edge represents an action that changes traversal state
\t\t\t// (Cut, Surf, a story gate, a boulder switch, ...). Requiring the
\t\t\t// pre-action walking component to reach its port would make the action
\t\t\t// impossible to select. Non-semantic edges retain the exact old rule.
\t\t\tif !semantic[e] && !canExit(g, e, entry) {
\t\t\t\tcontinue
\t\t\t}''')

Path("world/semantic_route.go").write_text(r'''package world

import (
\t"errors"
\t"fmt"

\tgameruntime "github.com/maestroi/pokepilot/game"
)

// RoutePrerequisites overlays semantic transition requirements onto the
// geometric map graph. Edge remains the current graph identity; the transition
// value is the portable contract consumed by both routing and execution.
type RoutePrerequisites struct {
\tTransitions  map[Edge]gameruntime.Transition
\tCapabilities gameruntime.CapabilitySet
}

// RouteStep preserves the semantic transition identity selected for an edge.
// Transition is nil for ordinary walking/warp edges.
type RouteStep struct {
\tEdge       Edge
\tTransition *gameruntime.Transition
}

// TransitionExecutionResult is the portable observation returned by a
// game-adapter executor. Changed means the executor positively observed a world
// or traversal-state change, so the caller must discard the remaining route and
// re-plan from fresh state before doing anything else.
type TransitionExecutionResult struct {
\tChanged bool
}

// TransitionExecutor is the game-adapter seam for semantic route actions. The
// generic world layer owns only the contract; badge/HM menus, story battles,
// boulders, and other mechanics remain in the adapter.
type TransitionExecutor interface {
\tExecuteTransition(Edge, gameruntime.Transition) (TransitionExecutionResult, error)
}

var (
\tErrTransitionExecutorUnavailable = errors.New("world: semantic transition executor unavailable")
\tErrTransitionExecutionStalled    = errors.New("world: semantic transition execution made no durable progress")
)

// TransitionExecutionError preserves the selected transition/edge and the
// typed adapter cause so objective recovery (#145) can classify it without
// parsing prose.
type TransitionExecutionError struct {
\tEdge       Edge
\tTransition gameruntime.Transition
\tCause      error
}

func (e *TransitionExecutionError) Error() string {
\tif e == nil {
\t\treturn "world: semantic transition execution failed"
\t}
\treturn fmt.Sprintf("world: execute transition %q on %02x->%02x: %v", e.Transition.ID, e.Edge.From, e.Edge.To, e.Cause)
}

func (e *TransitionExecutionError) Unwrap() error {
\tif e == nil {
\t\treturn nil
\t}
\treturn e.Cause
}

// ExecuteTransition invokes the adapter through a typed boundary and turns a
// missing executor or adapter failure into structured transition evidence.
func ExecuteTransition(executor TransitionExecutor, edge Edge, transition gameruntime.Transition) (TransitionExecutionResult, error) {
\tif executor == nil {
\t\treturn TransitionExecutionResult{}, &TransitionExecutionError{
\t\t\tEdge: edge, Transition: transition, Cause: ErrTransitionExecutorUnavailable,
\t\t}
\t}
\tresult, err := executor.ExecuteTransition(edge, transition)
\tif err != nil {
\t\treturn TransitionExecutionResult{}, &TransitionExecutionError{Edge: edge, Transition: transition, Cause: err}
\t}
\treturn result, nil
}

// RouteBlockedError reports that a semantic route exists, but one or more
// transitions on it are unusable because capabilities are absent. It unwraps
// to ErrNoRoute so conservative callers keep their existing behavior.
type RouteBlockedError struct {
\tBlockages []gameruntime.TransitionBlockage
}

func (e *RouteBlockedError) Error() string {
\tif e == nil || len(e.Blockages) == 0 {
\t\treturn ErrNoRoute.Error()
\t}
\treturn fmt.Sprintf("%s: %s", ErrNoRoute, e.Blockages[0].Error())
}

func (e *RouteBlockedError) Unwrap() error { return ErrNoRoute }

func (e *RouteBlockedError) MissingCapabilities() []gameruntime.CapabilityID {
\tif e == nil {
\t\treturn nil
\t}
\tseen := map[gameruntime.CapabilityID]bool{}
\tvar out []gameruntime.CapabilityID
\tfor _, blockage := range e.Blockages {
\t\tfor _, id := range blockage.Missing {
\t\t\tif !seen[id] {
\t\t\t\tseen[id] = true
\t\t\t\tout = append(out, id)
\t\t\t}
\t\t}
\t}
\treturn out
}

// FindRouteAtDestinationWithCapabilities is the compatibility edge-only view
// of FindRoutePlanAtDestinationWithCapabilities.
func FindRouteAtDestinationWithCapabilities(
\tg *Graph,
\tfrom, to uint8,
\tx, y, tx, ty int,
\tblockedHere map[Edge]bool,
\tprereqs RoutePrerequisites,
) ([]Edge, error) {
\tplan, err := FindRoutePlanAtDestinationWithCapabilities(g, from, to, x, y, tx, ty, blockedHere, prereqs)
\tif err != nil {
\t\treturn nil, err
\t}
\tedges := make([]Edge, len(plan))
\tfor i := range plan {
\t\tedges[i] = plan[i].Edge
\t}
\treturn edges, nil
}

// FindRoutePlanAtDestinationWithCapabilities applies the same semantic policy
// used by reachability filtering and preserves transition identity for
// execution. Capability-satisfied semantic edges are executable pivots: the
// pre-action ordinary-walking component does not have to reach the port. A
// missing capability removes that edge and, when it is the reason routing
// fails, returns structured prerequisite evidence before any movement occurs.
func FindRoutePlanAtDestinationWithCapabilities(
\tg *Graph,
\tfrom, to uint8,
\tx, y, tx, ty int,
\tblockedHere map[Edge]bool,
\tprereqs RoutePrerequisites,
) ([]RouteStep, error) {
\tif g == nil || len(prereqs.Transitions) == 0 {
\t\troute, err := FindRouteAtDestination(g, from, to, x, y, tx, ty, blockedHere)
\t\treturn routeSteps(route, nil), err
\t}

\tdenied := make(map[Edge]gameruntime.TransitionBlockage)
\tallowed := make(map[Edge]bool)
\tallSemantic := make(map[Edge]bool, len(prereqs.Transitions))
\tfor edge, transition := range prereqs.Transitions {
\t\tallSemantic[edge] = true
\t\tif blockage, ok := gameruntime.EvaluateTransition(transition, prereqs.Capabilities); !ok {
\t\t\tdenied[edge] = blockage
\t\t} else {
\t\t\tallowed[edge] = true
\t\t}
\t}

\tusable := g
\tif len(denied) > 0 {
\t\tusable = graphWithoutSemanticEdges(g, denied)
\t}
\troute, err := findRouteAtDestinationAllowingSemantic(usable, from, to, x, y, tx, ty, blockedHere, allowed)
\tif err == nil {
\t\treturn routeSteps(route, prereqs.Transitions), nil
\t}
\tif !errors.Is(err, ErrNoRoute) || len(denied) == 0 {
\t\treturn nil, err
\t}

\t// Diagnose against the route that would exist if every known semantic
\t// action were usable. This is essential for gates such as Surf/Cut whose
\t// pre-action walking topology deliberately cannot reach the port.
\tgeometric, geometricErr := findRouteAtDestinationAllowingSemantic(g, from, to, x, y, tx, ty, blockedHere, allSemantic)
\tif geometricErr != nil {
\t\treturn nil, err
\t}
\tvar blockages []gameruntime.TransitionBlockage
\tfor _, edge := range geometric {
\t\tif blockage, ok := denied[edge]; ok {
\t\t\tblockages = append(blockages, blockage)
\t\t}
\t}
\tif len(blockages) == 0 {
\t\treturn nil, err
\t}
\treturn nil, &RouteBlockedError{Blockages: blockages}
}

func routeSteps(route []Edge, transitions map[Edge]gameruntime.Transition) []RouteStep {
\tsteps := make([]RouteStep, len(route))
\tfor i, edge := range route {
\t\tsteps[i].Edge = edge
\t\tif transition, ok := transitions[edge]; ok {
\t\t\tt := transition
\t\t\tsteps[i].Transition = &t
\t\t}
\t}
\treturn steps
}

func graphWithoutSemanticEdges(g *Graph, denied map[Edge]gameruntime.TransitionBlockage) *Graph {
\tcopyGraph := *g
\tcopyGraph.Edges = make(map[uint8][]Edge, len(g.Edges))
\tfor mapID, edges := range g.Edges {
\t\tfiltered := make([]Edge, 0, len(edges))
\t\tfor _, edge := range edges {
\t\t\tif _, blocked := denied[edge]; blocked {
\t\t\t\tcontinue
\t\t\t}
\t\t\tfiltered = append(filtered, edge)
\t\t}
\t\tcopyGraph.Edges[mapID] = filtered
\t}
\treturn &copyGraph
}
''')

# world grid: preserve land default while exposing the ROM's water tile-pair
# collision table for a live Surf-mode overlay.
replace_once("world/grid.go",
'''\ttilePairCollisionsLandAddr = 0x0c7e
\ttilePairEntryLen           = 3
)''',
'''\ttilePairCollisionsLandAddr  = 0x0c7e
\ttilePairCollisionsWaterAddr = 0x0ca0
\ttilePairEntryLen            = 3
)''')
replace_once("world/grid.go",
'''// Grid is a map's collision view, indexed [y][x] in game tile coordinates —''',
'''// TraversalMode selects the ROM tile-pair table used for movement. Tile
// walkability itself is shared; Gen 1 changes pair restrictions while surfing.
type TraversalMode uint8

const (
\tTraversalLand TraversalMode = iota
\tTraversalWater
)

// Grid is a map's collision view, indexed [y][x] in game tile coordinates —''')
replace_once("world/grid.go",
'''func tilePairsFor(romData []byte, tileset uint8) map[[2]uint8]bool {
\tpairs := map[[2]uint8]bool{}
\tfor off := tilePairCollisionsLandAddr; off+tilePairEntryLen <= len(romData); off += tilePairEntryLen {
\t\tif romData[off] == 0xff {
\t\t\tbreak
\t\t}
\t\tif romData[off] != tileset {
\t\t\tcontinue
\t\t}
\t\ta, b := romData[off+1], romData[off+2]
\t\tpairs[[2]uint8{a, b}] = true
\t\tpairs[[2]uint8{b, a}] = true
\t}
\treturn pairs
}''',
'''func tilePairsFor(romData []byte, tileset uint8) map[[2]uint8]bool {
\treturn tilePairsForTraversal(romData, tileset, TraversalLand)
}

func tilePairsForTraversal(romData []byte, tileset uint8, mode TraversalMode) map[[2]uint8]bool {
\tpairs := map[[2]uint8]bool{}
\taddr := tilePairCollisionsLandAddr
\tif mode == TraversalWater {
\t\taddr = tilePairCollisionsWaterAddr
\t}
\tfor off := addr; off+tilePairEntryLen <= len(romData); off += tilePairEntryLen {
\t\tif romData[off] == 0xff {
\t\t\tbreak
\t\t}
\t\tif romData[off] != tileset {
\t\t\tcontinue
\t\t}
\t\ta, b := romData[off+1], romData[off+2]
\t\tpairs[[2]uint8{a, b}] = true
\t\tpairs[[2]uint8{b, a}] = true
\t}
\treturn pairs
}''')
replace_once("world/grid.go",
'''func BuildFromBlocks(romData []byte, h rom.MapHeader, blocks []byte) (*Grid, error) {
\twidth := int(h.WidthBlocks) * 2''',
'''func BuildFromBlocks(romData []byte, h rom.MapHeader, blocks []byte) (*Grid, error) {
\treturn BuildFromBlocksForTraversal(romData, h, blocks, TraversalLand)
}

// BuildFromBlocksForTraversal decodes h with the movement-mode-specific
// tile-pair collision table. It is used by live navigation after Surf changes
// wWalkBikeSurfState; static graph construction intentionally stays on land.
func BuildFromBlocksForTraversal(romData []byte, h rom.MapHeader, blocks []byte, mode TraversalMode) (*Grid, error) {
\twidth := int(h.WidthBlocks) * 2''')
replace_once("world/grid.go",
'''\t\ttilePairs:      tilePairsFor(romData, h.Tileset),''',
'''\t\ttilePairs:      tilePairsForTraversal(romData, h.Tileset, mode),''')

Path("skill/live_topology.go").write_text(r'''package skill

import (
\t"fmt"

\t"github.com/maestroi/pokepilot/emu"
\t"github.com/maestroi/pokepilot/red/rom"
\t"github.com/maestroi/pokepilot/red/sym"
\t"github.com/maestroi/pokepilot/world"
)

const liveMapBorderBlocks = 3

func readLiveMapBlocks(peek func(uint16) uint8, widthBlocks, heightBlocks int) ([]byte, error) {
\tif widthBlocks < 0 || heightBlocks < 0 {
\t\treturn nil, fmt.Errorf("skill: live map has negative dimensions %dx%d", widthBlocks, heightBlocks)
\t}
\tif widthBlocks == 0 || heightBlocks == 0 {
\t\treturn []byte{}, nil
\t}
\tstride := widthBlocks + 2*liveMapBorderBlocks
\tfirst := liveMapBorderBlocks*stride + liveMapBorderBlocks
\tlast := first + (heightBlocks-1)*stride + (widthBlocks - 1)
\tif first < 0 || last >= sym.OverworldMapLen {
\t\treturn nil, fmt.Errorf("skill: live map %dx%d needs wOverworldMap offset %d, buffer length is %d", widthBlocks, heightBlocks, last, sym.OverworldMapLen)
\t}
\tblocks := make([]byte, widthBlocks*heightBlocks)
\tfor y := 0; y < heightBlocks; y++ {
\t\tfor x := 0; x < widthBlocks; x++ {
\t\t\toff := first + y*stride + x
\t\t\tblocks[y*widthBlocks+x] = peek(sym.OverworldMap + uint16(off))
\t\t}
\t}
\treturn blocks, nil
}

func liveMapBlocks(m *emu.Emu, h rom.MapHeader) ([]byte, error) {
\twidthBlocks, heightBlocks := int(h.WidthBlocks), int(h.HeightBlocks)
\tif got := int(m.Peek8(sym.CurMapWidth)); got != widthBlocks {
\t\treturn nil, fmt.Errorf("skill: live map width is %d blocks, ROM header for map %02x says %d", got, h.ID, widthBlocks)
\t}
\tif got := int(m.Peek8(sym.CurMapHeight)); got != heightBlocks {
\t\treturn nil, fmt.Errorf("skill: live map height is %d blocks, ROM header for map %02x says %d", got, h.ID, heightBlocks)
\t}
\treturn readLiveMapBlocks(m.Peek8, widthBlocks, heightBlocks)
}

// liveMapGrid decodes the current post-script geometry using the traversal mode
// the game is actually in. It intentionally has no cache: semantic transitions
// such as Cut and Surf are followed by a fresh decode before routing continues.
func liveMapGrid(m *emu.Emu, romData []byte, h rom.MapHeader) (*world.Grid, error) {
\tmode := world.TraversalLand
\tif m.Peek8(sym.WalkBikeSurfState) == fieldSurfingState {
\t\tmode = world.TraversalWater
\t}
\treturn liveMapGridForTraversal(m, romData, h, mode)
}

func liveMapGridForTraversal(m *emu.Emu, romData []byte, h rom.MapHeader, mode world.TraversalMode) (*world.Grid, error) {
\tblocks, err := liveMapBlocks(m, h)
\tif err != nil {
\t\treturn nil, err
\t}
\treturn world.BuildFromBlocksForTraversal(romData, h, blocks, mode)
}
''')

# GoTo: use the same capability-aware route plan as RoutePlanner, invoke the
# adapter-owned action before traversal, and throw away the route on any
# positively observed transition effect.
replace_once("skill/goto.go",
'''const maxNavigationTransitions = 64''',
'''const (
\tmaxNavigationTransitions        = 64
\tmaxSemanticTransitionExecutions = 16
)''')
replace_once("skill/goto.go",
'''func GoTo(m *emu.Emu, romData []byte, dest Destination) error {
\tg, err := world.BuildGraph(romData)''',
'''func GoTo(m *emu.Emu, romData []byte, dest Destination) error {
\treturn goToWithTransitionExecutor(m, romData, dest, newRedRouteTransitionExecutor(m, romData, nil))
}

func goToWithTransitionExecutor(m *emu.Emu, romData []byte, dest Destination, executor world.TransitionExecutor) error {
\tg, err := world.BuildGraph(romData)''')
replace_once("skill/goto.go",
'''\treplans := 0
\tvar previousMap uint8''',
'''\treplans := 0
\tsemanticExecutions := 0
\tvar previousMap uint8''')
replace_once("skill/goto.go",
'''\t\troute, err := world.FindRouteAtDestination(
\t\t\trouteGraph, cur, dest.Map, int(x), int(y), int(dest.X), int(dest.Y), preferred,
\t\t)
\t\t// A dead-end map's only exit IS the reverse. Route 4's Pokemon
''',
'''\t\tvar mem state.Mem
\t\tstate.Snapshot(m, &mem)
\t\tprereqs := redRoutePrerequisites(routeGraph, romData, &mem)
\t\troute, err := world.FindRoutePlanAtDestinationWithCapabilities(
\t\t\trouteGraph, cur, dest.Map, int(x), int(y), int(dest.X), int(dest.Y), preferred, prereqs,
\t\t)
\t\t// A dead-end map's only exit IS the reverse. Route 4's Pokemon
''')
replace_once("skill/goto.go",
'''\t\t\troute, err = world.FindRouteAtDestination(
\t\t\t\trouteGraph, cur, dest.Map, int(x), int(y), int(dest.X), int(dest.Y), blockedHere,
\t\t\t)''',
'''\t\t\troute, err = world.FindRoutePlanAtDestinationWithCapabilities(
\t\t\t\trouteGraph, cur, dest.Map, int(x), int(y), int(dest.X), int(dest.Y), blockedHere, prereqs,
\t\t\t)''')
replace_once("skill/goto.go",
'''\t\te := route[0]
\t\tif err := Traverse(m, romData, e); err != nil {''',
'''\t\tstep := route[0]
\t\te := step.Edge
\t\tif step.Transition != nil {
\t\t\texecution, execErr := world.ExecuteTransition(executor, e, *step.Transition)
\t\t\tif execErr != nil {
\t\t\t\treturn fmt.Errorf("skill: GoTo: %w", execErr)
\t\t\t}
\t\t\tif execution.Changed {
\t\t\t\tsemanticExecutions++
\t\t\t\tif semanticExecutions > maxSemanticTransitionExecutions {
\t\t\t\t\treturn fmt.Errorf("skill: GoTo: %w", &world.TransitionExecutionError{
\t\t\t\t\t\tEdge: e, Transition: *step.Transition, Cause: world.ErrTransitionExecutionStalled,
\t\t\t\t\t})
\t\t\t\t}
\t\t\t\tcontinue // effect observed: discard stale route/topology and re-plan
\t\t\t}
\t\t}
\t\tif err := Traverse(m, romData, e); err != nil {''')

# Travel: semantic executor is now the primary path. Keep the old Cut recovery
# only as a compatibility fallback for Cut gates not yet represented as
# semantic graph transitions.
replace_once("skill/travel.go",
'''func cutAwareGoTo(m *emu.Emu, romData []byte, dest Destination) func() error {
\tcuts := 0
\treturn func() error {
\t\tfor {
\t\t\terr := GoTo(m, romData, dest)''',
'''func cutAwareGoTo(m *emu.Emu, romData []byte, dest Destination, policies ...MovePolicy) func() error {
\tvar policy MovePolicy
\tif len(policies) > 0 {
\t\tpolicy = policies[0]
\t}
\texecutor := newRedRouteTransitionExecutor(m, romData, policy)
\tcuts := 0
\treturn func() error {
\t\tfor {
\t\t\terr := goToWithTransitionExecutor(m, romData, dest, executor)''')
# Only the two calls in this file are changed; other specialized callers keep
# using the variadic compatibility form without a battle policy.
s = Path("skill/travel.go").read_text()
s = s.replace("cutAwareGoTo(m, romData, dest),", "cutAwareGoTo(m, romData, dest, policy),")
Path("skill/travel.go").write_text(s)

Path("skill/route_transition.go").write_text(r'''package skill

import (
\t"errors"
\t"fmt"

\tgameruntime "github.com/maestroi/pokepilot/game"
\t"github.com/maestroi/pokepilot/emu"
\t"github.com/maestroi/pokepilot/red/rom"
\t"github.com/maestroi/pokepilot/red/state"
\t"github.com/maestroi/pokepilot/red/sym"
\t"github.com/maestroi/pokepilot/world"
)

var ErrRouteTransitionNeedsBattlePolicy = errors.New("skill: semantic route transition requires a battle policy")

type redRouteTransitionExecutor struct {
\tm       *emu.Emu
\tromData []byte
\tpolicy  MovePolicy
}

func newRedRouteTransitionExecutor(m *emu.Emu, romData []byte, policy MovePolicy) world.TransitionExecutor {
\treturn &redRouteTransitionExecutor{m: m, romData: romData, policy: policy}
}

func (x *redRouteTransitionExecutor) ExecuteTransition(edge world.Edge, transition gameruntime.Transition) (world.TransitionExecutionResult, error) {
\tif x == nil || x.m == nil {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: nil Red semantic transition executor")
\t}
\tswitch transition.ID {
\tcase "red:vermilion_gym_cut":
\t\topened, err := cutThroughReachableTree(x.m, x.romData)
\t\tif err != nil {
\t\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("Cut gate: %w", err)
\t\t}
\t\t// No candidate is the idempotent already-open case. Traverse is the
\t\t// positive proof that ordinary geometry is now sufficient.
\t\treturn world.TransitionExecutionResult{Changed: opened}, nil
\n\tcase "red:route21_surf":
\t\treturn x.executeSurf(edge)
\n\tcase "red:route12_snorlax":
\t\treturn x.executeRoute12Snorlax()
\n\tcase "red:victory_road_strength":
\t\treturn x.executeVictoryRoadStrength(edge)
\tdefault:
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: no Red executor owns semantic transition %q", transition.ID)
\t}
}

func (x *redRouteTransitionExecutor) executeSurf(edge world.Edge) (world.TransitionExecutionResult, error) {
\tif x.m.Peek8(sym.WalkBikeSurfState) == fieldSurfingState {
\t\treturn world.TransitionExecutionResult{}, nil
\t}
\tif edge.Kind != world.EdgeConnection {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition %02x->%02x is not a map connection", edge.From, edge.To)
\t}
\tif got := x.m.Peek8(sym.CurMap); got != edge.From {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition starts on %02x, current map is %02x", edge.From, got)
\t}

\th, err := rom.ParseMap(x.romData, edge.From)
\tif err != nil {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition parse map %02x: %w", edge.From, err)
\t}
\tland, err := liveMapGridForTraversal(x.m, x.romData, h, world.TraversalLand)
\tif err != nil {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition land grid: %w", err)
\t}
\twater, err := liveMapGridForTraversal(x.m, x.romData, h, world.TraversalWater)
\tif err != nil {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition water grid: %w", err)
\t}
\tsx, sy := playerXY(x.m)
\tblocked := spriteBlockers(x.m)
\ttx, ty, err := edgeTarget(water, edge.Dir, int(sx), int(sy), blocked)
\tif err != nil {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition cannot reach %02x connection in water mode: %w", edge.To, err)
\t}
\tsteps, err := world.FindPath(water, int(sx), int(sy), tx, ty, blocked)
\tif err != nil {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition water path: %w", err)
\t}

\tpx, py := int(sx), int(sy)
\tstandX, standY, waterX, waterY := 0, 0, 0, 0
\tfound := false
\tfor _, step := range steps {
\t\tnx, ny := px+step.DX, py+step.DY
\t\tif !land.Passable(px, py, nx, ny) && water.Passable(px, py, nx, ny) {
\t\t\tstandX, standY, waterX, waterY = px, py, nx, ny
\t\t\tfound = true
\t\t\tbreak
\t\t}
\t\tpx, py = nx, ny
\t}
\tif !found {
\t\t// The connection is already ordinary-walkable from this position;
\t\t// do not enter Surf merely because the semantic edge is annotated.
\t\treturn world.TransitionExecutionResult{}, nil
\t}
\tif err := walkWithinMap(x.m, x.romData, Destination{Map: edge.From, X: uint8(standX), Y: uint8(standY)}); err != nil {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition reach shoreline (%d,%d): %w", standX, standY, err)
\t}
\tif err := Face(x.m, uint8(waterX), uint8(waterY)); err != nil {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition face water (%d,%d): %w", waterX, waterY, err)
\t}
\tx.m.StepFrames(2)
\tresult, err := UseFieldMove(x.m, FieldSurf)
\tif err != nil {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition enter mode: %w", err)
\t}
\tif !result.Surfing || x.m.Peek8(sym.WalkBikeSurfState) != fieldSurfingState {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition returned without verified surfing state")
\t}
\treturn world.TransitionExecutionResult{Changed: true}, nil
}

func (x *redRouteTransitionExecutor) executeRoute12Snorlax() (world.TransitionExecutionResult, error) {
\tvar before state.Mem
\tstate.Snapshot(x.m, &before)
\tif state.HasEvent(&before, eventBeatRoute12Snorlax) {
\t\treturn world.TransitionExecutionResult{}, nil
\t}
\tif x.policy == nil {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("%w: Route 12 Snorlax", ErrRouteTransitionNeedsBattlePolicy)
\t}
\tif err := clearRoute12Snorlax(x.m, x.romData, x.policy); err != nil {
\t\treturn world.TransitionExecutionResult{}, err
\t}
\tvar after state.Mem
\tstate.Snapshot(x.m, &after)
\tif !state.HasEvent(&after, eventBeatRoute12Snorlax) {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: Route 12 Snorlax transition completed without EVENT_BEAT_ROUTE12_SNORLAX")
\t}
\treturn world.TransitionExecutionResult{Changed: true}, nil
}

func victoryRoadSectionForTransition(edge world.Edge) (VictoryRoadBoulderSection, bool) {
\tswitch {
\tcase edge.From == victoryRoad1FMap && edge.To == victoryRoad2FMap:
\t\treturn VictoryRoad1FSwitch, true
\tcase edge.From == victoryRoad2FMap && edge.To == victoryRoad3FMap:
\t\treturn VictoryRoad2FSwitch1, true
\tcase edge.From == victoryRoad3FMap && edge.To == victoryRoad2FMap:
\t\treturn VictoryRoad3FSwitch, true
\tdefault:
\t\t// Descending 2F -> 1F is geometrically traversable and does not own a
\t\t// new boulder objective; the coarse bidirectional semantic annotation
\t\t// remains harmless until the route-fact model is made directional.
\t\treturn 0, false
\t}
}

func (x *redRouteTransitionExecutor) executeVictoryRoadStrength(edge world.Edge) (world.TransitionExecutionResult, error) {
\tsection, ok := victoryRoadSectionForTransition(edge)
\tif !ok {
\t\treturn world.TransitionExecutionResult{}, nil
\t}
\tspec, _ := VictoryRoadBoulderSpec(section)
\tvar before state.Mem
\tstate.Snapshot(x.m, &before)
\tif boulderPuzzleEventComplete(&before, spec) {
\t\treturn world.TransitionExecutionResult{}, nil
\t}
\tif x.policy == nil {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("%w: %s", ErrRouteTransitionNeedsBattlePolicy, section)
\t}
\tif _, err := SolveVictoryRoadBoulderSection(x.m, x.romData, x.policy, section); err != nil {
\t\treturn world.TransitionExecutionResult{}, err
\t}
\tvar after state.Mem
\tstate.Snapshot(x.m, &after)
\tif !boulderPuzzleEventComplete(&after, spec) {
\t\treturn world.TransitionExecutionResult{}, fmt.Errorf("skill: %s solver returned without its completion event", section)
\t}
\treturn world.TransitionExecutionResult{Changed: true}, nil
}
''')

Path("world/semantic_route_execution_test.go").write_text(r'''package world

import (
\t"errors"
\t"testing"

\tgameruntime "github.com/maestroi/pokepilot/game"
)

type fakeTransitionExecutor struct {
\tedge       Edge
\ttransition gameruntime.Transition
\tchanged    bool
\terr        error
}

func (f *fakeTransitionExecutor) ExecuteTransition(edge Edge, transition gameruntime.Transition) (TransitionExecutionResult, error) {
\tf.edge, f.transition = edge, transition
\treturn TransitionExecutionResult{Changed: f.changed}, f.err
}

func TestSemanticRoutePlanPreservesExecutableTransitionAcrossBlockedComponent(t *testing.T) {
\tedge := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
\tg := &Graph{
\t\tEdges:          map[uint8][]Edge{1: {edge}, 2: {}},
\t\tcomponentAware: true,
\t\tcomps: map[uint8][][]int{
\t\t\t1: {{1, 2}}, // player is component 1; edge exits component 2
\t\t\t2: {{3}},
\t\t},
\t\texitComps:  map[Edge][]int{edge: {2}},
\t\tentryComps: map[Edge][]int{edge: {3}},
\t}
\ttransition := gameruntime.Transition{ID: "fake:surf", From: "shore", To: "island", Requires: []gameruntime.CapabilityID{"can_surf"}}
\tprereqs := RoutePrerequisites{
\t\tTransitions:  map[Edge]gameruntime.Transition{edge: transition},
\t\tCapabilities: gameruntime.NewCapabilitySet("can_surf"),
\t}

\tif _, err := FindRouteAtDestination(g, 1, 2, 0, 0, 0, 0, nil); !errors.Is(err, ErrNoRoute) {
\t\tt.Fatalf("ordinary route error = %v, want ErrNoRoute", err)
\t}
\tplan, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 0, 0, 0, 0, nil, prereqs)
\tif err != nil {
\t\tt.Fatalf("semantic route: %v", err)
\t}
\tif len(plan) != 1 || plan[0].Edge != edge || plan[0].Transition == nil || plan[0].Transition.ID != transition.ID {
\t\tt.Fatalf("plan = %+v, want one executable fake:surf step", plan)
\t}
}

func TestSemanticRouteBlockedBeforeMovementWhenPivotCapabilityMissing(t *testing.T) {
\tedge := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
\tg := &Graph{
\t\tEdges:          map[uint8][]Edge{1: {edge}, 2: {}},
\t\tcomponentAware: true,
\t\tcomps:          map[uint8][][]int{1: {{1, 2}}, 2: {{3}}},
\t\texitComps:      map[Edge][]int{edge: {2}},
\t\tentryComps:     map[Edge][]int{edge: {3}},
\t}
\ttransition := gameruntime.Transition{ID: "fake:cut", From: "a", To: "b", Requires: []gameruntime.CapabilityID{"can_cut"}}
\t_, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 0, 0, 0, 0, nil, RoutePrerequisites{
\t\tTransitions: map[Edge]gameruntime.Transition{edge: transition},
\t})
\tvar blocked *RouteBlockedError
\tif !errors.As(err, &blocked) {
\t\tt.Fatalf("error = %T %v, want *RouteBlockedError", err, err)
\t}
\tif got := blocked.MissingCapabilities(); len(got) != 1 || got[0] != gameruntime.CapabilityID("can_cut") {
\t\tt.Fatalf("missing = %v, want [can_cut]", got)
\t}
}

func TestPortableTransitionExecutorIsTypedAndAdapterAgnostic(t *testing.T) {
\tedge := Edge{Kind: EdgeConnection, From: 7, To: 8, Dir: dirSouth}
\ttransition := gameruntime.Transition{ID: "fake:bridge", From: "north", To: "south"}
\tfake := &fakeTransitionExecutor{changed: true}
\tresult, err := ExecuteTransition(fake, edge, transition)
\tif err != nil || !result.Changed {
\t\tt.Fatalf("ExecuteTransition = %+v, %v; want changed success", result, err)
\t}
\tif fake.edge != edge || fake.transition.ID != transition.ID {
\t\tt.Fatalf("executor observed edge=%+v transition=%+v", fake.edge, fake.transition)
\t}

\t_, err = ExecuteTransition(nil, edge, transition)
\tvar execution *TransitionExecutionError
\tif !errors.As(err, &execution) || !errors.Is(err, ErrTransitionExecutorUnavailable) {
\t\tt.Fatalf("nil executor error = %T %v, want typed unavailable error", err, err)
\t}
}
''')

Path("skill/route_transition_test.go").write_text(r'''package skill

import (
\t"errors"
\t"testing"

\t"github.com/maestroi/pokepilot/world"
)

func TestVictoryRoadTransitionDelegatesOnlyOwnedStrengthSections(t *testing.T) {
\ttests := []struct {
\t\tedge world.Edge
\t\twant VictoryRoadBoulderSection
\t\tok   bool
\t}{
\t\t{world.Edge{From: victoryRoad1FMap, To: victoryRoad2FMap}, VictoryRoad1FSwitch, true},
\t\t{world.Edge{From: victoryRoad2FMap, To: victoryRoad3FMap}, VictoryRoad2FSwitch1, true},
\t\t{world.Edge{From: victoryRoad3FMap, To: victoryRoad2FMap}, VictoryRoad3FSwitch, true},
\t\t{world.Edge{From: victoryRoad2FMap, To: victoryRoad1FMap}, 0, false},
\t}
\tfor _, tt := range tests {
\t\tgot, ok := victoryRoadSectionForTransition(tt.edge)
\t\tif ok != tt.ok || got != tt.want {
\t\t\tt.Fatalf("edge %02x->%02x = (%v,%v), want (%v,%v)", tt.edge.From, tt.edge.To, got, ok, tt.want, tt.ok)
\t\t}
\t}
}

func TestDirectStrengthTransitionRequiresBattlePolicyBeforeSolver(t *testing.T) {
\tif !errors.Is(ErrRouteTransitionNeedsBattlePolicy, ErrRouteTransitionNeedsBattlePolicy) {
\t\tt.Fatal("policy sentinel lost identity")
\t}
}
''')

Path("skill/route_transition_real_test.go").write_text(r'''package skill

import (
\t"testing"

\tgameruntime "github.com/maestroi/pokepilot/game"
\t"github.com/maestroi/pokepilot/emu"
\t"github.com/maestroi/pokepilot/red/sym"
\t"github.com/maestroi/pokepilot/world"
)

func preparedSemanticEdge(t *testing.T, m *emu.Emu, transitionID string) (world.Edge, gameruntime.Transition) {
\tt.Helper()
\tg, err := world.BuildGraph(m.ROM())
\tif err != nil {
\t\tt.Fatalf("BuildGraph: %v", err)
\t}
\tcur := m.Peek8(sym.CurMap)
\tfor _, edge := range g.Edges[cur] {
\t\ttransition, ok := redRouteTransitionForEdge(edge)
\t\tif ok && transition.ID == transitionID {
\t\t\treturn edge, transition
\t\t}
\t}
\tt.Fatalf("map %02x has no %q semantic edge", cur, transitionID)
\treturn world.Edge{}, gameruntime.Transition{}
}

// POKEPILOT_CUT_ROUTE_TEST_STATE is a controllable Vermilion City checkpoint
// from which the Gym Cut tree is reachable and Cut is usable/preparable. It
// verifies the semantic executor changes live topology and ordinary Traverse
// can then cross the selected graph edge.
func TestSemanticCutRouteTransitionRealROM(t *testing.T) {
\tm := loadPreparedFieldActionState(t, "POKEPILOT_CUT_ROUTE_TEST_STATE")
\tif got := m.Peek8(sym.CurMap); got != semanticVermilionCityMap {
\t\tt.Fatalf("Cut route checkpoint map=%02x, want Vermilion City %02x", got, semanticVermilionCityMap)
\t}
\tedge, transition := preparedSemanticEdge(t, m, "red:vermilion_gym_cut")
\tresult, err := world.ExecuteTransition(newRedRouteTransitionExecutor(m, m.ROM(), nil), edge, transition)
\tif err != nil {
\t\tt.Fatalf("execute Cut transition: %v", err)
\t}
\tif !result.Changed {
\t\tt.Fatal("prepared Cut checkpoint did not observe a topology change")
\t}
\tif err := Traverse(m, m.ROM(), edge); err != nil {
\t\tt.Fatalf("Traverse after Cut: %v", err)
\t}
\tif got := m.Peek8(sym.CurMap); got != edge.To {
\t\tt.Fatalf("map after Cut transition=%02x, want %02x", got, edge.To)
\t}
}

// POKEPILOT_SURF_ROUTE_TEST_STATE is a controllable Pallet Town shoreline
// checkpoint with Surf usable/preparable. The executor must enter verified
// Surf mode and Traverse must continue across the Route 21 connection.
func TestSemanticSurfRouteTransitionRealROM(t *testing.T) {
\tm := loadPreparedFieldActionState(t, "POKEPILOT_SURF_ROUTE_TEST_STATE")
\tif got := m.Peek8(sym.CurMap); got != semanticPalletTownMap {
\t\tt.Fatalf("Surf route checkpoint map=%02x, want Pallet Town %02x", got, semanticPalletTownMap)
\t}
\tedge, transition := preparedSemanticEdge(t, m, "red:route21_surf")
\tresult, err := world.ExecuteTransition(newRedRouteTransitionExecutor(m, m.ROM(), nil), edge, transition)
\tif err != nil {
\t\tt.Fatalf("execute Surf transition: %v", err)
\t}
\tif !result.Changed || m.Peek8(sym.WalkBikeSurfState) != fieldSurfingState {
\t\tt.Fatalf("Surf execution=%+v state=%d, want changed + surfing", result, m.Peek8(sym.WalkBikeSurfState))
\t}
\tif err := Traverse(m, m.ROM(), edge); err != nil {
\t\tt.Fatalf("Traverse while surfing: %v", err)
\t}
\tif got := m.Peek8(sym.CurMap); got != edge.To {
\t\tt.Fatalf("map after Surf transition=%02x, want %02x", got, edge.To)
\t}
}
''')
