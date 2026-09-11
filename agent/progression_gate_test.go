package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func TestJourneyProgressionBlockedRoute3UntilBoulderBadge(t *testing.T) {
	obs := Observation{Events: []string{state.EventGotPokedex.String()}}
	if !journeyProgressionBlocked(obs, route3Map) {
		t.Fatal("Route 3 should be blocked before the Boulder Badge")
	}
	if journeyProgressionBlocked(obs, route2Map) {
		t.Fatal("unrelated Route 2 journey should not be blocked")
	}

	obs.Badges = []string{state.BadgeBoulder.String()}
	if journeyProgressionBlocked(obs, route3Map) {
		t.Fatal("Route 3 should be available after the Boulder Badge")
	}
}

func TestOfferSuppressesPewterRoute3UntilBoulderBadge(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{0x02: {route3Map}})
	known.SawMap(0x02)
	obs := Observation{
		Map:        0x02,
		MapName:    "PEWTER_CITY",
		X:          10,
		Y:          18,
		PartyCount: 1,
		Party:      []PartyMon{{Level: 10, HP: 30, MaxHP: 30}},
	}

	plain, flee := offeredJourneyTo(obs, known, "route 3")
	if plain || flee {
		t.Fatalf("pre-Boulder Route 3 = plain:%v flee:%v, want both suppressed", plain, flee)
	}

	obs.Badges = []string{state.BadgeBoulder.String()}
	plain, flee = offeredJourneyTo(obs, known, "route 3")
	if !plain || !flee {
		t.Fatalf("post-Boulder Route 3 = plain:%v flee:%v, want both offered", plain, flee)
	}
}

func TestJourneyProgressionBlockedRoute2UntilPokedex(t *testing.T) {
	obs := Observation{}
	if !journeyProgressionBlocked(obs, route2Map) {
		t.Fatal("Route 2 should be blocked before the parcel is delivered")
	}
	obs.Events = []string{state.EventGotOaksParcel.String()}
	if !journeyProgressionBlocked(obs, route2Map) {
		t.Fatal("Route 2 should stay blocked while the parcel is only carried")
	}
	obs.Events = append(obs.Events, state.EventGotPokedex.String())
	if journeyProgressionBlocked(obs, route2Map) {
		t.Fatal("Route 2 should open once the Pokedex is in hand")
	}
}

func TestJourneyProgressionBlockedUsesSemanticLateGameFacts(t *testing.T) {
	tests := []struct {
		name  string
		mapID uint8
		fact  ProgressID
	}{
		{name: "Saffron", mapID: saffronCityMap, fact: ProgressSaffronGateOpen},
		{name: "Saffron Gym", mapID: saffronGymMap, fact: redProgressSilphRescueComplete},
		{name: "Cinnabar Gym", mapID: cinnabarGymMap, fact: ProgressSecretKeyOwned},
		{name: "Viridian Gym", mapID: viridianGymMap, fact: ProgressViridianGymOpen},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obs := Observation{}
			if !journeyProgressionBlocked(obs, tc.mapID) {
				t.Fatalf("map %#02x should be blocked before semantic prerequisite", tc.mapID)
			}
			obs.Story = ProgressState{{ID: tc.fact, Complete: true}}
			if journeyProgressionBlocked(obs, tc.mapID) {
				t.Fatalf("map %#02x should open after semantic prerequisite", tc.mapID)
			}
		})
	}
}

func TestPlaceProgressionBlockedSnorlaxWaypointsUntilPokeFlute(t *testing.T) {
	obs := Observation{}
	for _, place := range []string{"route 12 snorlax", "route 12 south of snorlax"} {
		if !placeProgressionBlocked(obs, place) {
			t.Fatalf("%q should be blocked before the Poké Flute", place)
		}
	}
	if placeProgressionBlocked(obs, "route 13") {
		t.Fatal("unrelated place should not be blocked")
	}

	obs.Story = ProgressState{{ID: redProgressPokeFluteAcquired, Complete: true}}
	for _, place := range []string{"route 12 snorlax", "route 12 south of snorlax"} {
		if placeProgressionBlocked(obs, place) {
			t.Fatalf("%q should no longer be progression-blocked after the Poké Flute", place)
		}
	}
}

func TestOfferNeverAdvertisesRoute12SnorlaxInteractionAsJourney(t *testing.T) {
	dest, ok := skill.Place("route 12 snorlax")
	if !ok {
		t.Fatal("route 12 snorlax interaction missing")
	}
	route12Map := dest.Map

	known := NewKnowledge(map[uint8][]uint8{route12Map: {}})
	known.SawMap(route12Map)
	obs := Observation{
		Map:        route12Map,
		MapName:    "ROUTE_12",
		X:          9,
		Y:          62,
		PartyCount: 1,
		Party:      []PartyMon{{Level: 30, HP: 80, MaxHP: 80}},
	}

	for _, story := range []ProgressState{
		nil,
		{{ID: redProgressPokeFluteAcquired, Complete: true}},
	} {
		obs.Story = story
		plain, flee := offeredJourneyTo(obs, known, "route 12 snorlax")
		if plain || flee {
			t.Fatalf("story-owned Route 12 Snorlax interaction = plain:%v flee:%v, want both suppressed", plain, flee)
		}
	}
}

func TestOfferSuppressesSaffronGymUntilSilphRescue(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{saffronCityMap: {saffronGymMap}})
	known.SawMap(saffronCityMap)
	obs := Observation{
		Map:        saffronCityMap,
		MapName:    "SAFFRON_CITY",
		PartyCount: 1,
		Party:      []PartyMon{{Level: 35, HP: 100, MaxHP: 100}},
		Story: ProgressState{
			{ID: ProgressSaffronGateOpen, Complete: true},
		},
	}

	plain, flee := offeredJourneyTo(obs, known, "saffron gym")
	if plain || flee {
		t.Fatalf("pre-Silph-rescue Saffron Gym = plain:%v flee:%v, want both suppressed", plain, flee)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: redProgressSilphRescueComplete, Complete: true})
	plain, flee = offeredJourneyTo(obs, known, "saffron gym")
	if !plain || !flee {
		t.Fatalf("post-Silph-rescue Saffron Gym = plain:%v flee:%v, want both offered", plain, flee)
	}
}
