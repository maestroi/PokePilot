package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestStarterOf(t *testing.T) {
	if got := starterOf(Objective{Kind: KindStarter}); got != skill.StarterSquirtle {
		t.Fatalf("empty Starter = %d, want Squirtle (historic default)", got)
	}
	if got := starterOf(Objective{Kind: KindStarter, Starter: "squirtle"}); got != skill.StarterSquirtle {
		t.Fatalf("squirtle = %d, want Squirtle", got)
	}
	if got := starterOf(Objective{Kind: KindStarter, Starter: "charmander"}); got != skill.StarterCharmander {
		t.Fatalf("charmander = %d, want Charmander", got)
	}
	if got := starterOf(Objective{Kind: KindStarter, Starter: "bulbasaur"}); got != skill.StarterBulbasaur {
		t.Fatalf("bulbasaur = %d, want Bulbasaur", got)
	}
}
