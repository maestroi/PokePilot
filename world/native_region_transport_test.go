package world

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func regionTransportTestWorld() (*NativeGraph, func(game.PlaceID) (uint16, bool), func(uint16) (game.RegionID, bool)) {
	// A fake warp from Johto to Kanto (1 -> 3) must not be usable as
	// free transit, even if 3 loops back into a closer Johto station.
	graph := &NativeGraph{Edges: map[uint16][]NativeEdge{
		1: {{Kind: EdgeConnection, From: 1, To: 2}, {Kind: EdgeWarp, From: 1, To: 3}},
		2: {},
		3: {{Kind: EdgeWarp, From: 3, To: 4}},
		4: {},
	}}
	ids := map[game.PlaceID]uint16{"olivine": 2, "vermilion": 3, "goldenrod": 4}
	resolve := func(place game.PlaceID) (uint16, bool) {
		id, ok := ids[place]
		return id, ok
	}
	region := func(id uint16) (game.RegionID, bool) {
		switch id {
		case 1, 2, 4:
			return game.RegionJohto, true
		case 3:
			return game.RegionKanto, true
		default:
			return "", false
		}
	}
	return graph, resolve, region
}

func TestNativeRegionTravelKeepsScriptedCrossingSeparate(t *testing.T) {
	graph, resolve, region := regionTransportTestWorld()
	options := []game.RegionTransportOption{
		{Service: "train", FromRegion: game.RegionJohto, ToRegion: game.RegionKanto,
			From: "goldenrod", To: "vermilion", Available: true},
		{Service: "boat", FromRegion: game.RegionJohto, ToRegion: game.RegionKanto,
			From: "olivine", To: "vermilion", Available: true, OneWay: true},
	}
	route, err := PlanNativeRegionRoute(graph, 1, game.RegionJohto, game.RegionKanto, options, resolve, region)
	if err != nil {
		t.Fatal(err)
	}
	if route.Transport.Service != "boat" || route.BoardingMap != 2 || route.ArrivalMap != 3 ||
		len(route.Approach) != 1 || route.Approach[0].To != 2 || !route.Transport.OneWay {
		t.Fatalf("got %+v, expected approach to Olivine then separate boat action", route)
	}
	// The train is on a map reached only by crossing into Kanto first. The
	// static warp via Kanto must not make its boarding point reachable.
	_, err = PlanNativeRegionRoute(graph, 1, game.RegionJohto, game.RegionKanto, options[:1], resolve, region)
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("fake cross-region warp accepted: %v", err)
	}
}

func TestNativeRegionTravelReportsPrerequisitesWithoutPretendBoarding(t *testing.T) {
	graph, resolve, region := regionTransportTestWorld()
	options := []game.RegionTransportOption{{
		Service: "boat", FromRegion: game.RegionJohto, ToRegion: game.RegionKanto,
		From: "olivine", To: "vermilion", Missing: []game.CapabilityID{"ss_ticket", "departure_day"},
	}}
	_, err := PlanNativeRegionRoute(graph, 1, game.RegionJohto, game.RegionKanto, options, resolve, region)
	var blocked *RouteBlockedError
	if !errors.As(err, &blocked) || !errors.Is(err, ErrNoRoute) {
		t.Fatalf("expected typed route blockage, got %v", err)
	}
	got := blocked.MissingCapabilities()
	if len(got) != 2 || got[0] != "ss_ticket" || got[1] != "departure_day" {
		t.Fatalf("missing = %v", got)
	}
	options[0].Available = true
	options[0].Missing = nil
	_, err = PlanNativeRegionRoute(graph, 1, game.RegionKanto, game.RegionJohto, options, resolve, region)
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("one-way trip incorrectly reversible: %v", err)
	}
}

func TestNativeRegionTravelFailsClosedOnUnknownMapsAndRegions(t *testing.T) {
	graph, resolve, region := regionTransportTestWorld()
	options := []game.RegionTransportOption{{
		Service: "boat", FromRegion: game.RegionJohto, ToRegion: game.RegionKanto,
		From: "olivine", To: "vermilion", Available: true,
	}}
	if _, err := PlanNativeRegionRoute(graph, 1, game.RegionJohto, game.RegionKanto, options, nil, region); !errors.Is(err, ErrRegionTransportUnsupported) {
		t.Fatalf("nil resolver should be unsupported: %v", err)
	}
	if _, err := PlanNativeRegionRoute(graph, 99, game.RegionJohto, game.RegionKanto, options, resolve, region); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("unknown start map: %v", err)
	}
	if _, err := PlanNativeRegionRoute(graph, 1, game.RegionJohto, game.RegionJohto, options, resolve, region); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("intra-region crossing: %v", err)
	}
	unknown := func(uint16) (game.RegionID, bool) { return "", false }
	if _, err := PlanNativeRegionRoute(graph, 1, game.RegionJohto, game.RegionKanto, options, resolve, unknown); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("unknown region must not be traversed: %v", err)
	}
}
