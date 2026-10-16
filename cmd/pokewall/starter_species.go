package main

import (
	"strings"

	redstarter "github.com/maestroi/pokepilot/red/starter"
)

// withStarterSpecies fills the dashboard species from the starter request and
// seed. Historical rows only stored the request ("random", "mewtwo"), so the
// Runs pane can say which Pokémon Oak handed over without a catalog rewrite.
func (row tileRow) withStarterSpecies() tileRow {
	row.StarterSpecies = resolvedStarterSpecies(row.Game, row.Starter, row.Seed)
	return row
}

func resolvedStarterSpecies(game, starter string, seed int64) string {
	game = strings.ToLower(strings.TrimSpace(game))
	switch game {
	case "tetris", "boxxle":
		return ""
	case "pokemon-yellow":
		return "pikachu"
	case "pokemon-gold", "pokemon-silver":
		switch name := strings.ToLower(strings.TrimSpace(starter)); name {
		case "chikorita", "cyndaquil", "totodile":
			return name
		default:
			return ""
		}
	}
	// Pokémon Red, Blue, and rows saved before game identity existed.
	if game != "" && game != "pokemon-red" && game != "pokemon-blue" {
		return ""
	}
	selection, err := redstarter.Resolve(starter, seed)
	if err != nil || selection.Species == "" {
		return ""
	}
	return string(selection.Species)
}
