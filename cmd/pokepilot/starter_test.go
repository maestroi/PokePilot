package main

import (
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestFarmStarterFor(t *testing.T) {
	for _, c := range []struct {
		name string
		want skill.Starter
	}{
		{"", skill.StarterSquirtle}, // historic default: an older spec omitting the field
		{"squirtle", skill.StarterSquirtle},
		{"charmander", skill.StarterCharmander},
		{"bulbasaur", skill.StarterBulbasaur},
		{"mew", skill.StarterSquirtle},
		{"mewtwo", skill.StarterSquirtle},
		{"random", skill.StarterSquirtle},
		{"random:any", skill.StarterSquirtle},
		{"nonsense", skill.StarterSquirtle},
	} {
		if got := farmStarterFor(c.name); got != c.want {
			t.Errorf("farmStarterFor(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStarterFromNameAcceptsExperiments(t *testing.T) {
	for _, name := range []string{"mew", "mewtwo", "random", "random:basic", "random:any"} {
		got, ok := starterFromName(name)
		if !ok || got != skill.StarterSquirtle {
			t.Errorf("starterFromName(%q) = (%v, %v), want middle ball", name, got, ok)
		}
	}
	if _, ok := starterFromName("missingno"); ok {
		t.Fatal("starterFromName(missingno): expected rejection")
	}
}
