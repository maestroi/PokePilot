package skill

import (
	"strings"
	"testing"

	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

func TestYellowViridianVictoryRoadSharedMapIDs(t *testing.T) {
	for id, want := range map[uint8]string{
		viridianCityMap:        "VIRIDIAN_CITY",
		viridianGymMap:         "VIRIDIAN_GYM",
		route22Map:             "ROUTE_22",
		route23Map:             "ROUTE_23",
		victoryRoad1FMap:       "VICTORY_ROAD_1F",
		victoryRoad2FMap:       "VICTORY_ROAD_2F",
		victoryRoad3FMap:       "VICTORY_ROAD_3F",
		indigoPlateauMap:       "INDIGO_PLATEAU",
		indigoPlateauLobbyMap: "INDIGO_PLATEAU_LOBBY",
	} {
		if got := yellowrom.MapName(id); got != want {
			t.Fatalf("Yellow map %#02x = %q, want %q", id, got, want)
		}
	}
}

func TestYellowViridianVictoryRoadSharedControllerFactsMatchDecomp(t *testing.T) {
	gymObjects := yellowDecompText(t, "data/maps/objects/ViridianGym.asm")
	if !strings.Contains(gymObjects, "object_event 2, 1, SPRITE_GIOVANNI, STAY, DOWN, TEXT_VIRIDIANGYM_GIOVANNI, OPP_GIOVANNI, 3") {
		t.Fatal("Yellow Viridian Gym Giovanni moved from the shared controller coordinate")
	}

	gym := yellowDecompText(t, "scripts/ViridianGym.asm")
	for _, want := range []string{
		"set BIT_EARTHBADGE, [hl]",
		"SetEvents EVENT_2ND_ROUTE22_RIVAL_BATTLE, EVENT_ROUTE22_RIVAL_WANTS_BATTLE",
	} {
		if !strings.Contains(gym, want) {
			t.Fatalf("Yellow Viridian Gym no longer exposes shared postcondition %q", want)
		}
	}

	route22 := yellowDecompText(t, "scripts/Route22.asm")
	for _, want := range []string{
		"dbmapcoord 29, 4",
		"dbmapcoord 29, 5",
		"SetEvent EVENT_BEAT_ROUTE22_RIVAL_2ND_BATTLE",
	} {
		if !strings.Contains(route22, want) {
			t.Fatalf("Yellow Route 22 no longer exposes shared final-rival fact %q", want)
		}
	}

	for path, wants := range map[string][]string{
		"scripts/VictoryRoad1F.asm": {"dbmapcoord 17, 13", "cp PIKACHU_SPRITE_INDEX"},
		"scripts/VictoryRoad2F.asm": {"dbmapcoord 1, 16", "dbmapcoord 9, 16", "cp PIKACHU_SPRITE_INDEX"},
		"scripts/VictoryRoad3F.asm": {"dbmapcoord 3, 5", "dbmapcoord 23, 15", "cp PIKACHU_SPRITE_INDEX"},
	} {
		script := yellowDecompText(t, path)
		for _, want := range wants {
			if !strings.Contains(script, want) {
				t.Fatalf("Yellow %s no longer exposes shared Victory Road fact %q", path, want)
			}
		}
	}

	victoryRoad2F := yellowDecompText(t, "data/maps/objects/VictoryRoad2F.asm")
	if !strings.Contains(victoryRoad2F, "object_event 23, 16, SPRITE_BOULDER, STAY, BOULDER_MOVEMENT_BYTE_2, TEXT_VICTORYROAD2F_BOULDER3") {
		t.Fatal("Yellow Victory Road 2F hole boulder moved from the shared object slot/coordinate")
	}
}
