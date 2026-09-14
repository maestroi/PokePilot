package agent

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

func TestFilterRedServiceTalkObjectivesSuppressesMartClerk(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}

	const viridianMart = 0x2A
	clerkX, clerkY, err := rom.MartClerkPosition(romData, viridianMart)
	if err != nil {
		t.Fatalf("resolve Viridian Mart clerk: %v", err)
	}
	objectives := []Objective{
		{Kind: KindTalk, X: clerkX, Y: clerkY},
		{Kind: KindTalk, X: 5, Y: 5}, // ordinary youngster in Viridian Mart
		{Kind: KindGoTo, Place: "viridian city"},
	}

	got := filterRedServiceTalkObjectives(romData, Observation{Map: viridianMart}, objectives)
	if len(got) != 2 {
		t.Fatalf("filtered objective count = %d, want 2: %+v", len(got), got)
	}
	for _, o := range got {
		if o.Kind == KindTalk && o.X == clerkX && o.Y == clerkY {
			t.Fatalf("Mart clerk was still offered as a generic talk objective: %+v", got)
		}
	}
	if got[0] != (Objective{Kind: KindTalk, X: 5, Y: 5}) {
		t.Fatalf("ordinary Mart customer was filtered too: got[0]=%+v", got[0])
	}
}
