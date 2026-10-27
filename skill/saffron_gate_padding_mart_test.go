package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestSaffronBorderPaddingBandsCarryDrinkGate locks the shared invariant
// behind run-aqbnxwh5p4i4hqy3h80mcvc / triage bfda8e7f35489e76: every
// component-scoped Saffron border band must carry red:saffron_guard_drink,
// including proven-unwalkable padding. Skipping Gate attachment on padding
// left those bands as free map-adjacency for mapsConnected, so a Diglett's
// Cave Hyper Potion restock treated Saffron Mart as a two-hop
// ErrRouteReplanRequired lure past a completed Fuchsia route.
func TestSaffronBorderPaddingBandsCarryDrinkGate(t *testing.T) {
	romData := badgeFourROM(t)
	g, err := routeGraphForROM(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	mem := new(state.Mem)
	prereqs := redRoutePrerequisites(g, romData, mem)

	var bands, gated int
	for _, edge := range g.Edges[semanticRoute6Map] {
		if edge.Kind != world.EdgeConnection || edge.To != semanticSaffronCityMap {
			continue
		}
		bands++
		tr, ok := prereqs.Transitions[edge]
		if !ok || tr.ID != "red:saffron_guard_drink" || !tr.Gate {
			t.Fatalf("Route 6 -> Saffron band %+v missing drink gate (ok=%v transition=%+v)", edge, ok, tr)
		}
		gated++
	}
	if bands < 2 {
		t.Fatalf("expected multiple Route 6 -> Saffron bands, got %d", bands)
	}
	if gated != bands {
		t.Fatalf("gated %d of %d Route 6 -> Saffron bands", gated, bands)
	}
}

// TestHyperPotionMartSelectionDoesNotLureThroughSaffronPadding is the
// mart-facing symptom of the same invariant: from Diglett's Cave with Cut and
// the Poke Flute but no Saffron drink, Saffron Mart must not win as an
// ErrRouteReplanRequired frontier. Yellow stocks Hyper Potion at Fuchsia, so
// that completed route becomes the answer; Red does not, so stock recovery
// correctly reports no reachable mart instead of a Saffron lure.
func TestHyperPotionMartSelectionDoesNotLureThroughSaffronPadding(t *testing.T) {
	romData := badgeFourROM(t)

	mem := fieldTestMem(FieldCut, true, true, true)
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade |
		1<<state.BadgeThunder | 1<<state.BadgeRainbow
	setRouteTestEvent(mem, state.EventGotPokedex)
	setTestBag(mem, [2]uint8{hm01Item, 1}, [2]uint8{0x49, 1}) // HM01 + Poke Flute
	cut, ok := FieldMoveSpecFor(FieldCut)
	if !ok {
		t.Fatal("Cut field move missing")
	}
	mem[sym.PartyMon1+sym.MonMoves] = cut.MoveID
	mem[sym.CurMap] = diglettsCaveMap
	mem[sym.XCoord] = 5
	mem[sym.YCoord] = 5

	g, err := cachedRouteGraph(romData)
	if err != nil {
		t.Fatal(err)
	}
	g, err = withAsleepRoute16Snorlax(g, romData, mem)
	if err != nil {
		t.Fatal(err)
	}
	g, err = withSurfSeaTopology(g, romData, mem)
	if err != nil {
		t.Fatal(err)
	}
	prereqs := redRoutePrerequisites(g, romData, mem)
	if prereqs.Capabilities.Has(capCanEnterSaffron) {
		t.Fatal("fixture must not include can_enter_saffron")
	}
	if !prereqs.Capabilities.Has(capCanCut) || !prereqs.Capabilities.Has(capCanClearSnorlax) {
		t.Fatalf("fixture missing Cut/Snorlax capabilities: %v", prereqs.Capabilities)
	}

	const hyper = uint8(0x12)
	saffronRoute, saffronErr := world.FindRoutePlanAtDestinationWithCapabilities(
		g, diglettsCaveMap, uint8(0xB4), 5, 5, standardMartCounterX, standardMartCounterY, nil, prereqs,
	)
	if saffronErr == nil || errors.Is(saffronErr, world.ErrRouteReplanRequired) {
		t.Fatalf("Saffron Mart still looks reachable/replanable without the drink: err=%v plan=%+v", saffronErr, saffronRoute)
	}

	mart, found, err := nearestStockMart(romData, mem, hyper)
	if err != nil {
		t.Fatalf("nearestStockMart: %v", err)
	}
	if found && mart.mapID == 0xB4 {
		t.Fatalf("Saffron Mart still won Hyper Potion selection without the drink")
	}
	if martStocksItem(romData, 0x98, hyper) {
		if !found || mart.mapID != 0x98 {
			t.Fatalf("nearest Hyper Potion mart = %s (%#02x) found=%v, want fuchsia (%#02x)",
				mart.name, mart.mapID, found, 0x98)
		}
		return
	}
	if found {
		t.Fatalf("unexpected reachable Hyper Potion mart %s (%#02x) on a ROM whose Fuchsia shelf lacks it",
			mart.name, mart.mapID)
	}
}
