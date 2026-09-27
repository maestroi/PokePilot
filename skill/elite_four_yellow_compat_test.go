package skill

import (
	"strings"
	"testing"

	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

func TestYellowLeagueSharedMapIDs(t *testing.T) {
	for id, want := range map[uint8]string{
		loreleiRoomMap:   "LORELEIS_ROOM",
		brunoRoomMap:     "BRUNOS_ROOM",
		agathaRoomMap:    "AGATHAS_ROOM",
		lanceRoomMap:     "LANCES_ROOM",
		championsRoomMap: "CHAMPIONS_ROOM",
		hallOfFameMap:    "HALL_OF_FAME",
	} {
		if got := yellowrom.MapName(id); got != want {
			t.Fatalf("Yellow League map %#02x = %q, want %q", id, got, want)
		}
	}
}

func TestYellowLeagueSharedControllerGeometryMatchesDecomp(t *testing.T) {
	for path, want := range map[string]string{
		"data/maps/objects/LoreleisRoom.asm": "object_event 5, 2, SPRITE_LORELEI, STAY, DOWN, TEXT_LORELEISROOM_LORELEI, OPP_LORELEI, 1",
		"data/maps/objects/BrunosRoom.asm":   "object_event 5, 2, SPRITE_BRUNO, STAY, DOWN, TEXT_BRUNOSROOM_BRUNO, OPP_BRUNO, 1",
		"data/maps/objects/AgathasRoom.asm":  "object_event 5, 2, SPRITE_AGATHA, STAY, DOWN, TEXT_AGATHASROOM_AGATHA, OPP_AGATHA, 1",
		"data/maps/objects/LancesRoom.asm":   "object_event 6, 1, SPRITE_LANCE, STAY, DOWN, TEXT_LANCESROOM_LANCE, OPP_LANCE, 1",
	} {
		if got := yellowDecompText(t, path); !strings.Contains(got, want) {
			t.Fatalf("Yellow League geometry %s no longer matches shared controller fact %q", path, want)
		}
	}

	championObjects := yellowDecompText(t, "data/maps/objects/ChampionsRoom.asm")
	for _, want := range []string{
		"object_event 4, 2, SPRITE_BLUE, STAY, DOWN, TEXT_CHAMPIONSROOM_RIVAL",
		"object_event 3, 7, SPRITE_OAK, STAY, UP, TEXT_CHAMPIONSROOM_OAK",
	} {
		if !strings.Contains(championObjects, want) {
			t.Fatalf("Yellow Champion room no longer matches shared controller fact %q", want)
		}
	}
}

func TestYellowLeagueSharedEventsAndChampionBranchMatchDecomp(t *testing.T) {
	scripts := map[string][]string{
		"scripts/LoreleisRoom.asm": {
			"set BIT_STARTED_ELITE_4, [hl]",
			"CheckAndSetEvent EVENT_AUTOWALKED_INTO_LORELEIS_ROOM",
			"trainer EVENT_BEAT_LORELEIS_ROOM_TRAINER_0",
		},
		"scripts/BrunosRoom.asm": {
			"trainer EVENT_BEAT_BRUNOS_ROOM_TRAINER_0",
		},
		"scripts/AgathasRoom.asm": {
			"trainer EVENT_BEAT_AGATHAS_ROOM_TRAINER_0",
			"ld a, SCRIPT_CHAMPIONSROOM_PLAYER_ENTERS",
		},
		"scripts/LancesRoom.asm": {
			"trainer EVENT_BEAT_LANCES_ROOM_TRAINER_0",
			"SetEvent EVENT_BEAT_LANCE",
		},
		"scripts/ChampionsRoom.asm": {
			"ld a, OPP_RIVAL3",
			"ld a, [wRivalStarter]",
			"SetEvent EVENT_BEAT_CHAMPION_RIVAL",
		},
		"scripts/HallOfFame.asm": {
			"set BIT_UNUSED_BEAT_ELITE_4, [hl]",
			"ResetEventRange INDIGO_PLATEAU_EVENTS_START, INDIGO_PLATEAU_EVENTS_END, 1",
			"farcall SaveGameData",
		},
	}
	for path, wants := range scripts {
		script := yellowDecompText(t, path)
		for _, want := range wants {
			if !strings.Contains(script, want) {
				t.Fatalf("Yellow %s no longer exposes shared League fact %q", path, want)
			}
		}
	}
}

func TestYellowLeagueReadinessLevelsMatchTrainerData(t *testing.T) {
	parties := yellowDecompText(t, "data/trainers/parties.asm")
	for _, want := range []string{
		"LoreleiData: db $FF, 54, DEWGONG, 53, CLOYSTER, 54, SLOWBRO, 56, JYNX, 56, LAPRAS, 0",
		"BrunoData: db $FF, 53, ONIX, 55, HITMONCHAN, 55, HITMONLEE, 56, ONIX, 58, MACHAMP, 0",
		"AgathaData: db $FF, 56, GENGAR, 56, GOLBAT, 55, HAUNTER, 58, ARBOK, 60, GENGAR, 0",
		"LanceData: db $FF, 58, GYARADOS, 56, DRAGONAIR, 56, DRAGONAIR, 60, AERODACTYL, 62, DRAGONITE, 0",
		"Rival3Data: ; Champion's Room db $FF, 61, SANDSLASH, 59, ALAKAZAM, 61, EXEGGUTOR, 61, CLOYSTER, 63, NINETALES, 65, JOLTEON, 0",
		"db $FF, 61, SANDSLASH, 59, ALAKAZAM, 61, EXEGGUTOR, 61, MAGNETON, 63, CLOYSTER, 65, FLAREON, 0",
		"db $FF, 61, SANDSLASH, 59, ALAKAZAM, 61, EXEGGUTOR, 61, NINETALES, 63, MAGNETON, 65, VAPOREON, 0",
	} {
		if !strings.Contains(parties, want) {
			t.Fatalf("Yellow trainer data no longer matches League readiness fact %q", want)
		}
	}
}
