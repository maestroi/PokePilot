package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// TestOfferKeepsDoorsSeenFromVisitedMaps: Cerulean is a door off Route 4, so
// the run learns it exists by standing on Route 4. Walking on into Mt. Moon
// must not un-learn it — that amnesia left a run in Mt. Moon with no forward
// destination on the menu at all (run-3t5kvlk55zvbjkkkno3ses4g6, 2026-09-07).
func TestOfferKeepsDoorsSeenFromVisitedMaps(t *testing.T) {
	const (
		route4   = 0x0F
		mtMoon1F = 0x3B
		cerulean = 0x03
	)
	known := NewKnowledge(map[uint8][]uint8{
		route4:   {mtMoon1F, cerulean},
		mtMoon1F: {route4},
	})
	known.SawMap(route4)
	known.SawMap(mtMoon1F)

	obs := Observation{Map: mtMoon1F, MapName: "MT_MOON_1F", PartyCount: 1}
	if !offersPlace(Offer(obs, known), "cerulean city") {
		t.Fatal("Cerulean, a door off the visited Route 4, dropped off the menu inside Mt. Moon")
	}
}

// TestOfferNarrowsTravelMenuToNearestPlaces: the whole known world is not a
// set of plans. Standing in Pewter with the route back to Pallet all visited,
// the menu keeps the frontier (Route 3/4, Mt. Moon, Cerulean) and drops the
// far end of the road behind it.
func TestOfferNarrowsTravelMenuToNearestPlaces(t *testing.T) {
	// The road as the game connects it: Pallet - Route 1 - Viridian -
	// Route 2 - Pewter - Route 3 - Route 4 - Cerulean, with Mt. Moon's three
	// floors hanging off Route 4.
	chain := []uint8{0x00, 0x0C, 0x01, 0x0D, 0x02, 0x0E, 0x0F, 0x03}
	adjacency := map[uint8][]uint8{}
	for i, m := range chain {
		if i > 0 {
			adjacency[m] = append(adjacency[m], chain[i-1])
		}
		if i < len(chain)-1 {
			adjacency[m] = append(adjacency[m], chain[i+1])
		}
	}
	adjacency[0x0F] = append(adjacency[0x0F], 0x3B) // Route 4 -> Mt. Moon 1F
	adjacency[0x3B] = []uint8{0x0F, 0x3C}
	adjacency[0x3C] = []uint8{0x3B, 0x3D}
	adjacency[0x3D] = []uint8{0x3C}
	// Buildings, so the known world is bigger than the road.
	adjacency[0x01] = append(adjacency[0x01], 0x29, 0x2A) // Viridian center, mart
	adjacency[0x02] = append(adjacency[0x02], 0x3A, 0x36) // Pewter center, gym

	known := NewKnowledge(adjacency)
	// Everything walked except Cerulean, which is still just a door seen from
	// Route 4 — the frontier this menu exists to reach.
	for _, m := range []uint8{0x00, 0x0C, 0x01, 0x0D, 0x02, 0x0E, 0x0F, 0x3B, 0x3C, 0x3D, 0x29, 0x2A, 0x3A, 0x36} {
		known.SawMap(m)
	}

	// Boulder + the Pokedex, so the Route 2 and Route 3 gates are open and
	// distance is the only thing narrowing the menu.
	offered := Offer(Observation{
		Map: 0x02, MapName: "PEWTER_CITY", PartyCount: 1,
		Badges: []string{state.BadgeBoulder.String()},
		Events: []string{state.EventGotPokedex.String()},
	}, known)
	for _, near := range []string{"route 3", "route 4", "cerulean city"} {
		if !offersPlace(offered, near) {
			t.Errorf("%q, on the frontier ahead of Pewter, was not offered", near)
		}
	}
	if offersPlace(offered, "pallet town") {
		t.Error("Pallet Town, four maps behind Pewter, was still on the travel menu")
	}
	places := map[string]bool{}
	for _, o := range offered {
		if o.Kind == KindGoTo {
			places[o.Place] = true
		}
	}
	if len(places) > journeyPlaceLimit {
		t.Errorf("travel menu held %d places, want at most %d", len(places), journeyPlaceLimit)
	}
}

func offersPlace(objs []Objective, place string) bool {
	for _, o := range objs {
		if o.Kind == KindGoTo && o.Place == place {
			return true
		}
	}
	return false
}
