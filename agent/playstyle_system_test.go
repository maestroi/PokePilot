package agent

import (
	"strings"
	"testing"
)

func TestPlayStyleSystemNoteKeepsLegacyEmptyPromptUnchanged(t *testing.T) {
	if got := PlayStyleSystemNote(""); got != "" {
		t.Fatalf("legacy empty play style added system text: %q", got)
	}
}

func TestPlayStyleSystemNoteDescribesEveryExplicitProfile(t *testing.T) {
	cases := []struct {
		style string
		want  []string
	}{
		{PlayStyleSpeedrun, []string{"PLAY STYLE: SPEEDRUN", "direct required progression", "optional exploration"}},
		{PlayStyleAdventure, []string{"PLAY STYLE: ADVENTURE", "nearby exploration", "natural"}},
		{PlayStyleCompletionist, []string{
			"PLAY STYLE: COMPLETIONIST",
			"not yet owned in the Pokédex",
			"regardless of battle usefulness or party fullness",
			"capture stock",
			"unvisited areas",
			"unseen NPCs",
			"collect reachable items",
			"unbeaten trainers",
			"explicit run goal still determines when the run ends",
		}},
		{PlayStyleTeamBuilder, []string{"PLAY STYLE: TEAM BUILDER", "building and developing", "catch-up training", "party"}},
	}

	for _, tc := range cases {
		t.Run(tc.style, func(t *testing.T) {
			got := PlayStyleSystemNote(tc.style)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("PlayStyleSystemNote(%q) = %q, want substring %q", tc.style, got, want)
				}
			}
			if !strings.Contains(got, "Only choose objectives that are actually offered") {
				t.Fatalf("PlayStyleSystemNote(%q) lost legality boundary: %q", tc.style, got)
			}
		})
	}
}
