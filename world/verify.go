package world

import (
	"fmt"
	"sort"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/worldverify"
)

// ValidationSnapshot projects the current graph into the game-agnostic
// worldverify contract. The graph is still Red-backed today, but none of its
// native map ids, collision representation, or ROM structs cross the verifier
// boundary. Future game adapters can emit the same snapshot directly.
func ValidationSnapshot(g *Graph, transitions map[Edge]gameruntime.Transition, starts ...MapID) worldverify.Snapshot {
	snapshot := worldverify.Snapshot{Game: "pokemon-red"}
	if g == nil {
		return snapshot
	}

	mapIDs := map[MapID]bool{}
	for id := range g.Edges {
		mapIDs[id] = true
	}
	for id := range g.tiles {
		mapIDs[id] = true
	}
	for id := range g.comps {
		mapIDs[id] = true
	}
	ordered := make([]int, 0, len(mapIDs))
	for id := range mapIDs {
		ordered = append(ordered, int(id))
	}
	sort.Ints(ordered)

	for _, raw := range ordered {
		id := MapID(raw)
		d := g.tiles[id]
		components := uniqueComponents(g.comps[id])
		snapshot.Maps = append(snapshot.Maps, worldverify.Map{
			ID:            validationMapID(id),
			Width:         d.w,
			Height:        d.h,
			GeometryKnown: g.componentAware && g.comps[id] != nil,
			Components:    components,
		})
	}

	for _, raw := range ordered {
		from := MapID(raw)
		for index, edge := range g.Edges[from] {
			out := worldverify.Edge{
				ID:   validationEdgeID(edge, index),
				Kind: validationEdgeKind(edge.Kind),
				From: validationMapID(edge.From),
				To:   validationMapID(edge.To),
				Exit: worldverify.Port{
					Known:      g.componentAware && g.comps[edge.From] != nil,
					Components: append([]int(nil), g.exitComps[edge]...),
				},
				Entry: worldverify.Port{
					Known:      g.componentAware && g.comps[edge.To] != nil,
					Components: append([]int(nil), g.entryComps[edge]...),
				},
			}

			switch edge.Kind {
			case EdgeWarp:
				out.Exit.Point = &worldverify.Point{X: int(edge.WarpX), Y: int(edge.WarpY)}
				if x, y, ok := g.destWarpTile(edge); ok {
					out.Entry.Point = &worldverify.Point{X: x, Y: y}
				}
			case EdgeConnection:
				if start, end, ok := ConnectionBand(edge); ok {
					limit := g.tiles[edge.From].w
					if edge.Dir >= dirWest {
						limit = g.tiles[edge.From].h
					}
					out.BorderSpan = &worldverify.Span{Start: start, End: end, Limit: limit}
				}
			}

			out.Execution = validationExecutionEvidence(g, edge)

			if transition, ok := transitions[edge]; ok {
				requires := make([]worldverify.CapabilityID, 0, len(transition.Requires))
				for _, capability := range transition.Requires {
					requires = append(requires, worldverify.CapabilityID(capability))
				}
				out.Transition = &worldverify.Transition{
					ID:         transition.ID,
					Requires:   requires,
					Gate:       transition.Gate,
					PivotOnly:  transition.PivotOnly,
					PortBypass: transition.PortBypass,
				}
				if transition.PortBypass {
					// The action owns traversal at this seam, so pristine standing
					// components are not authoritative post-action geometry.
					out.Exit.Known = false
					out.Entry.Known = false
				}
				if !transition.Gate {
					out.Execution = &worldverify.ExecutionEvidence{
						Status: worldverify.ExecutionDynamicUnknown,
						Reason: "semantic action changes traversal or landing topology; execute and rebuild live geometry",
					}
				}
			}
			snapshot.Edges = append(snapshot.Edges, out)
		}
	}

	for _, failure := range g.ParseFailures() {
		snapshot.MapParseDiagnostics = append(snapshot.MapParseDiagnostics, worldverify.MapParseDiagnostic{
			Map:    validationMapID(failure.MapID),
			Error:  failure.Err.Error(),
			Reason: failure.Reason,
		})
	}
	for _, start := range starts {
		snapshot.StartMaps = append(snapshot.StartMaps, validationMapID(start))
	}
	return snapshot
}

func validationExecutionEvidence(g *Graph, edge Edge) *worldverify.ExecutionEvidence {
	if g == nil || !g.componentAware || g.comps[edge.From] == nil || g.comps[edge.To] == nil {
		return &worldverify.ExecutionEvidence{
			Status: worldverify.ExecutionDynamicUnknown,
			Reason: "static collision geometry unavailable",
		}
	}
	if g.provider != nil {
		if _, ok := g.provider.ElevatorFloorForDestination(edge.From, edge.To); ok {
			return &worldverify.ExecutionEvidence{
				Status: worldverify.ExecutionDynamicUnknown,
				Reason: "elevator destination is selected by runtime menu/script state",
			}
		}
	}

	evidence := &worldverify.ExecutionEvidence{Status: worldverify.ExecutionProven}
	switch edge.Kind {
	case EdgeWarp:
		active := false
		for _, warp := range g.warps[edge.From] {
			if warp.X == edge.WarpX && warp.Y == edge.WarpY && !warp.Inert {
				active = true
				break
			}
		}
		if !active {
			return evidence
		}
		dx, dy, ok := g.destWarpTile(edge)
		if !ok {
			return evidence
		}
		entry := standingComponentAt(g, edge.To, dx, dy)
		if len(entry) == 0 {
			return evidence
		}
		seen := map[int]bool{}
		for _, delta := range [][2]int{{0, -1}, {-1, 0}, {1, 0}, {0, 1}} {
			sx, sy := int(edge.WarpX)+delta[0], int(edge.WarpY)+delta[1]
			for _, component := range standingComponentAt(g, edge.From, sx, sy) {
				if component <= 0 || seen[component] {
					continue
				}
				seen[component] = true
				evidence.Paths = append(evidence.Paths, worldverify.ExecutionPath{
					ExitComponent:  component,
					EntryComponent: entry[0],
					ExitPoint:      worldverify.Point{X: int(edge.WarpX), Y: int(edge.WarpY)},
					EntryPoint:     worldverify.Point{X: dx, Y: dy},
				})
			}
		}
	case EdgeConnection:
		connection, ok := g.connections[edge]
		if !ok {
			return evidence
		}
		src, dst := g.tiles[edge.From], g.tiles[edge.To]
		n := src.w
		if edge.Dir >= dirWest {
			n = src.h
		}
		start, end := connectionBandRange(edge, n)
		for i := start; i <= end; i++ {
			sx, sy, tx, ty := g.connectionSeamTile(edge, connection, i)
			if sx < 0 || sy < 0 || sx >= src.w || sy >= src.h ||
				tx < 0 || ty < 0 || tx >= dst.w || ty >= dst.h {
				continue
			}
			exit := standingComponentAt(g, edge.From, sx, sy)
			entry := standingComponentAt(g, edge.To, tx, ty)
			if len(exit) == 0 || len(entry) == 0 {
				continue
			}
			evidence.Paths = append(evidence.Paths, worldverify.ExecutionPath{
				ExitComponent:  exit[0],
				EntryComponent: entry[0],
				ExitPoint:      worldverify.Point{X: sx, Y: sy},
				EntryPoint:     worldverify.Point{X: tx, Y: ty},
			})
		}
	default:
		return &worldverify.ExecutionEvidence{
			Status: worldverify.ExecutionDynamicUnknown,
			Reason: "edge kind has no static local-navigation proof",
		}
	}
	return evidence
}

// VerifyGraph performs the portable structural and capability-state checks for
// a built graph. Callers that need custom verifier limits can use
// ValidationSnapshot and worldverify.Verify directly.
func VerifyGraph(g *Graph, transitions map[Edge]gameruntime.Transition, starts ...uint8) worldverify.Report {
	wide := make([]MapID, len(starts))
	for i, start := range starts {
		wide[i] = MapID(start)
	}
	return worldverify.Verify(ValidationSnapshot(g, transitions, wide...), worldverify.Options{})
}

// VerifyGraphMaps is the wide-map-id variant for adapters such as Gen II.
func VerifyGraphMaps(g *Graph, transitions map[Edge]gameruntime.Transition, starts ...MapID) worldverify.Report {
	return worldverify.Verify(ValidationSnapshot(g, transitions, starts...), worldverify.Options{})
}

func validationMapID(id MapID) worldverify.MapID {
	return worldverify.MapID(fmt.Sprintf("%02x", id))
}

func validationEdgeID(edge Edge, index int) string {
	kind := "warp"
	if edge.Kind == EdgeConnection {
		kind = "connection"
	}
	return fmt.Sprintf("%02x:%s:%d:%02x", edge.From, kind, index, edge.To)
}

func validationEdgeKind(kind EdgeKind) worldverify.EdgeKind {
	switch kind {
	case EdgeWarp:
		return worldverify.EdgeWarp
	case EdgeConnection:
		return worldverify.EdgeConnection
	default:
		return worldverify.EdgeOther
	}
}

func uniqueComponents(grid [][]int) []int {
	seen := map[int]bool{}
	for _, row := range grid {
		for _, component := range row {
			if component > 0 {
				seen[component] = true
			}
		}
	}
	out := make([]int, 0, len(seen))
	for component := range seen {
		out = append(out, component)
	}
	sort.Ints(out)
	return out
}
