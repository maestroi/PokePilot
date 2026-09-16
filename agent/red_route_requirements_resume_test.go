package agent

import (
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestRedRouteRequirementsResumeBeyondSnorlaxDoesNotReblockExit(t *testing.T) {
	for _, currentMap := range []uint8{route13Map, route14Map, route15Map, fuchsiaCityMap} {
		t.Run(fmt.Sprintf("map_%02x", currentMap), func(t *testing.T) {
			blockages := redRouteRequirements(Observation{Map: currentMap})
			for _, mapID := range []uint8{route12Map, route13Map, route14Map, route15Map, fuchsiaCityMap} {
				if routeRequirementsBlockMap(blockages, mapID) {
					t.Fatalf("checkpoint on map %#02x reblocked Poké Flute region map %#02x", currentMap, mapID)
				}
			}
			for _, place := range []PlaceID{"route 12 snorlax", "route 12 south of snorlax"} {
				if routeRequirementsBlockPlace(blockages, place) {
					t.Fatalf("checkpoint on map %#02x reblocked already-crossed Snorlax waypoint %q", currentMap, place)
				}
			}
		})
	}
}

func TestRedRouteRequirementsStillBlocksSnorlaxRegionBeforeCrossing(t *testing.T) {
	blockages := redRouteRequirements(Observation{Map: lavenderTownMap})
	for _, mapID := range []uint8{route12Map, route13Map, route14Map, route15Map, fuchsiaCityMap} {
		if !routeRequirementsBlockMap(blockages, mapID) {
			t.Fatalf("pre-crossing checkpoint did not block Poké Flute region map %#02x", mapID)
		}
	}
	for _, place := range []PlaceID{"route 12 snorlax", "route 12 south of snorlax"} {
		if !routeRequirementsBlockPlace(blockages, place) {
			t.Fatalf("pre-crossing checkpoint did not block Snorlax waypoint %q", place)
		}
	}
}

func routeRequirementsBlockMap(blockages []RouteBlockage, mapID uint8) bool {
	for _, blockage := range blockages {
		destination, ok := skill.Place(string(blockage.Destination))
		if ok && destination.Map == mapID {
			return true
		}
	}
	return false
}

func routeRequirementsBlockPlace(blockages []RouteBlockage, place PlaceID) bool {
	for _, blockage := range blockages {
		if blockage.Destination == place {
			return true
		}
	}
	return false
}
