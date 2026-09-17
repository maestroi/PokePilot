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
func ValidationSnapshot(g *Graph, transitions map[Edge]gameruntime.Transition, starts ...uint8) worldverify.Snapshot {
	snapshot := worldverify.Snapshot{Game: "pokemon-red"}
	if g == nil {
		return snapshot
	}

	mapIDs := map[uint8]bool{}
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
		id := uint8(raw)
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
		from := uint8(raw)
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
			}
			snapshot.Edges = append(snapshot.Edges, out)
		}
	}

	for _, start := range starts {
		snapshot.StartMaps = append(snapshot.StartMaps, validationMapID(start))
	}
	return snapshot
}

// VerifyGraph performs the portable structural and capability-state checks for
// a built graph. Callers that need custom verifier limits can use
// ValidationSnapshot and worldverify.Verify directly.
func VerifyGraph(g *Graph, transitions map[Edge]gameruntime.Transition, starts ...uint8) worldverify.Report {
	return worldverify.Verify(ValidationSnapshot(g, transitions, starts...), worldverify.Options{})
}

func validationMapID(id uint8) worldverify.MapID {
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
