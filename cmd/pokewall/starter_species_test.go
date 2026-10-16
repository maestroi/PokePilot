package main

import (
	"testing"

	redstarter "github.com/maestroi/pokepilot/red/starter"
)

func TestResolvedStarterSpecies(t *testing.T) {
	random, err := redstarter.Resolve("random", 42)
	if err != nil {
		t.Fatal(err)
	}
	if got := resolvedStarterSpecies("pokemon-red", "random", 42); got != string(random.Species) {
		t.Fatalf("random species = %q, want %q", got, random.Species)
	}
	if got := resolvedStarterSpecies("pokemon-red", "mewtwo", 7); got != "mewtwo" {
		t.Fatalf("fixed species = %q, want mewtwo", got)
	}
	if got := resolvedStarterSpecies("pokemon-red", "squirtle", 1); got != "squirtle" {
		t.Fatalf("vanilla species = %q, want squirtle", got)
	}
	if got := resolvedStarterSpecies("pokemon-red", "", 1); got != "" {
		t.Fatalf("unset red starter = %q, want empty", got)
	}
	if got := resolvedStarterSpecies("pokemon-yellow", "squirtle", 1); got != "pikachu" {
		t.Fatalf("yellow species = %q, want pikachu", got)
	}
	if got := resolvedStarterSpecies("pokemon-gold", "cyndaquil", 1); got != "cyndaquil" {
		t.Fatalf("gold species = %q, want cyndaquil", got)
	}
	if got := resolvedStarterSpecies("tetris", "squirtle", 1); got != "" {
		t.Fatalf("tetris species = %q, want empty", got)
	}
	if got := resolvedStarterSpecies("boxxle", "", 1); got != "" {
		t.Fatalf("boxxle species = %q, want empty", got)
	}

	row := (tileRow{Game: "pokemon-red", Starter: "random", Seed: 42}).withStarterSpecies()
	if row.StarterSpecies != string(random.Species) {
		t.Fatalf("annotated species = %q, want %q", row.StarterSpecies, random.Species)
	}
}
