package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestRoute12SnorlaxTransitionOnlyClaimsWalkableConnectionBands is the
// regression for #958 and the map-0x18 route_replan_exhausted farm burst.
// Route 13's north border contains several component-scoped in-bounds bands,
// but only the real Route 12 seam is walkable. The Snorlax semantic action is
// allowed to bypass ordinary canExit reachability, so attaching it to padding
// bands makes those solid-ground edges selectable and strands the run at
// Route 13 (11,4).
func TestRoute12SnorlaxTransitionOnlyClaimsWalkableConnectionBands(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	var mem state.Mem
	prereqs := redRoutePrerequisites(g, romData, &mem)
	walkable := 0
	padding := 0
	for _, edge := range g.Edges[route13Map] {
		if edge.Kind != world.EdgeConnection || edge.To != route12Map {
			continue
		}
		transition, attached := prereqs.Transitions[edge]
		if g.ConnectionExitWalkable(edge) {
			walkable++
			if !attached || transition.ID != "red:route12_snorlax" {
				start, end, _ := world.ConnectionBand(edge)
				t.Fatalf("walkable Route 13 -> Route 12 band %d..%d lacks Snorlax transition: attached=%v transition=%+v", start, end, attached, transition)
			}
			continue
		}

		padding++
		if attached && transition.ID == "red:route12_snorlax" {
			start, end, _ := world.ConnectionBand(edge)
			t.Fatalf("non-walkable Route 13 -> Route 12 padding band %d..%d still owns Snorlax transition", start, end)
		}
	}
	if walkable == 0 {
		t.Fatal("Route 13 -> Route 12 exposed no walkable connection band")
	}
	if padding == 0 {
		t.Fatal("Route 13 -> Route 12 exposed no non-walkable padding band; regression fixture no longer exercises the bug")
	}
}

// TestClearedRoute12SnorlaxDropsComponentBypassAction is the regression for
// farm run-zm5v9uxm1o2n10yevwxd6jxbe: after EVENT_BEAT_ROUTE12_SNORLAX the
// seam is ordinary geometry. Leaving the wake/battle action attached lets
// routing treat the north band as a semantic pivot even when a live sprite has
// severed the only walkable approach, which then falls through to a spurious
// Cycling Road bicycle prerequisite.
func TestClearedRoute12SnorlaxDropsComponentBypassAction(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	var mem state.Mem
	addr := sym.EventFlags + uint16(eventBeatRoute12Snorlax)/8
	mem[addr] |= byte(1 << (uint16(eventBeatRoute12Snorlax) % 8))

	prereqs := redRoutePrerequisites(g, romData, &mem)
	for _, edge := range g.Edges[route13Map] {
		if edge.Kind != world.EdgeConnection || edge.To != route12Map {
			continue
		}
		if transition, ok := prereqs.Transitions[edge]; ok && transition.ID == "red:route12_snorlax" {
			start, end, _ := world.ConnectionBand(edge)
			t.Fatalf("cleared Snorlax still attached action on Route 13 -> Route 12 band %d..%d", start, end)
		}
	}
	for _, edge := range g.Edges[route12Map] {
		if edge.Kind != world.EdgeConnection || edge.To != route13Map {
			continue
		}
		if transition, ok := prereqs.Transitions[edge]; ok && transition.ID == "red:route12_snorlax" {
			start, end, _ := world.ConnectionBand(edge)
			t.Fatalf("cleared Snorlax still attached action on Route 12 -> Route 13 band %d..%d", start, end)
		}
	}
}
