package main

import (
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

// farmStarterFor replaces agent.starterOf, which was removed when S6-7 made
// Objective.Starter a typed skill.Starter: agent no longer parses names, so
// the conversion moved to this spec/CLI boundary. The cases are the ones the
// original agent test covered, including the empty default.
func TestFarmStarterFor(t *testing.T) {
	for _, c := range []struct {
		name string
		want skill.Starter
	}{
		{"", skill.StarterSquirtle}, // historic default: an older spec omitting the field
		{"squirtle", skill.StarterSquirtle},
		{"charmander", skill.StarterCharmander},
		{"bulbasaur", skill.StarterBulbasaur},
		{"nonsense", skill.StarterSquirtle},
	} {
		if got := farmStarterFor(c.name); got != c.want {
			t.Errorf("farmStarterFor(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
