package skill

import (
	"fmt"
	"os"
	"strings"
	"testing"

	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

func yellowDecompText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile("../pokeyellow/" + path)
	if err != nil {
		t.Fatalf("read vendored Yellow decomp %s: %v", path, err)
	}
	return strings.Join(strings.Fields(string(data)), " ")
}

func TestYellowCinnabarSharedMapIDs(t *testing.T) {
	for id, want := range map[uint8]string{
		cinnabarIslandMap:    "CINNABAR_ISLAND",
		pokemonMansion1FMap:  "POKEMON_MANSION_1F",
		cinnabarGymMap:       "CINNABAR_GYM",
		pokemonMansion2FMap:  "POKEMON_MANSION_2F",
		pokemonMansion3FMap:  "POKEMON_MANSION_3F",
		pokemonMansionB1FMap: "POKEMON_MANSION_B1F",
	} {
		if got := yellowrom.MapName(id); got != want {
			t.Fatalf("Yellow map %#02x = %q, want %q", id, got, want)
		}
	}
}

func TestYellowCinnabarSharedControllerFactsMatchDecomp(t *testing.T) {
	hidden := yellowDecompText(t, "data/events/hidden_events.asm")
	switchName := map[uint8]string{
		pokemonMansion1FMap:  "Mansion1Script_Switches",
		pokemonMansion2FMap:  "Mansion2Script_Switches",
		pokemonMansion3FMap:  "Mansion3Script_Switches",
		pokemonMansionB1FMap: "Mansion4Script_Switches",
	}
	switches := append([]mansionSwitchSpec{mansion1FSwitch, mansion2FSwitch, mansion3FSwitch}, mansionB1FSwitches...)
	for _, sw := range switches {
		want := fmt.Sprintf("hidden_event %d, %d, %s, SPRITE_FACING_UP", sw.TargetX, sw.TargetY, switchName[sw.Map])
		if !strings.Contains(hidden, want) {
			t.Fatalf("Yellow decomp is missing shared Mansion switch %q", want)
		}
	}

	for _, quiz := range cinnabarQuizSpecs {
		answer := "FALSE"
		if quiz.AnswerMenuIndex == 1 {
			answer = "TRUE"
		}
		want := fmt.Sprintf(
			"hidden_event %d, %d, PrintCinnabarQuiz, (%s << 4) | %d",
			quiz.TargetX, quiz.TargetY, answer, quiz.Index,
		)
		if !strings.Contains(hidden, want) {
			t.Fatalf("Yellow decomp is missing shared Cinnabar quiz %q", want)
		}
	}

	b1f := yellowDecompText(t, "data/maps/objects/PokemonMansionB1F.asm")
	if !strings.Contains(b1f, "object_event 5, 13, SPRITE_POKE_BALL, STAY, NONE, TEXT_POKEMONMANSIONB1F_SECRET_KEY, SECRET_KEY") {
		t.Fatal("Yellow Mansion B1F Secret Key object moved from the shared controller coordinate")
	}

	gym := yellowDecompText(t, "scripts/CinnabarGym.asm")
	for _, want := range []string{
		"SetEvent EVENT_BEAT_BLAINE",
		"set BIT_VOLCANOBADGE, [hl]",
	} {
		if !strings.Contains(gym, want) {
			t.Fatalf("Yellow Cinnabar Gym no longer exposes shared Blaine postcondition %q", want)
		}
	}
}
