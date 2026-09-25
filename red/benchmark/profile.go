// Package benchmark defines Pokémon Red benchmark milestones without leaking
// Red-specific progression vocabulary into the generic benchmark engine.
package benchmark

import (
	"strings"

	"github.com/maestroi/pokepilot/agent"
	core "github.com/maestroi/pokepilot/benchmark"
)

func Profile() core.Profile {
	return core.Profile{
		Game: "pokemon-red",
		Milestones: []core.MilestoneDefinition{
			{ID: "starter-obtained", Name: "Starter obtained", Match: func(obs agent.Observation) bool { return len(obs.Party) > 0 }},
			{ID: "brock", Name: "Brock / Boulder Badge", Goal: "badges:1"},
			{ID: "misty", Name: "Misty / Cascade Badge", Goal: "badges:2"},
			{ID: "rocket-hideout", Name: "Rocket Hideout", Goal: "progress:silph_scope_acquired"},
			{ID: "pokemon-tower", Name: "Pokémon Tower", Goal: "progress:poke_flute_acquired"},
			{ID: "fuchsia-koga", Name: "Fuchsia / Koga", Match: badge("Soul")},
			{ID: "surf-obtained", Name: "Surf obtained", Goal: "capability:surf"},
			{ID: "strength-obtained", Name: "Strength obtained", Goal: "capability:strength"},
			{ID: "silph-completed", Name: "Silph Co completed", Goal: "progress:silph_co_cleared"},
			{ID: "sabrina", Name: "Sabrina / Marsh Badge", Match: badge("Marsh")},
			{ID: "cinnabar", Name: "Cinnabar reached", Match: locationContains("cinnabar")},
			{ID: "blaine", Name: "Blaine / Volcano Badge", Match: badge("Volcano")},
			{ID: "giovanni", Name: "Giovanni / Earth Badge", Match: badge("Earth")},
			{ID: "victory-road", Name: "Victory Road cleared", Goal: "progress:victory_road_cleared"},
			{ID: "indigo-plateau", Name: "Indigo Plateau", Goal: "progress:indigo_plateau_ready"},
			{ID: "elite-four", Name: "Elite Four cleared", Goal: "progress:league_lance_defeated"},
			{ID: "champion", Name: "Champion defeated", Goal: "progress:league_champion_defeated"},
			{ID: "hall-of-fame", Name: "Hall of Fame", Goal: "elite-four"},
		},
	}
}

func GoalFor(id string) (string, bool) {
	id = normalize(id)
	if id == "hall-of-fame" || id == "halloffame" || id == "hof" || id == "champion-hall-of-fame" {
		return "elite-four", true
	}
	if id == "brock" {
		return "badges:1", true
	}
	if id == "misty" {
		return "badges:2", true
	}
	if id == "koga" || id == "fuchsia" || id == "fuchsia-koga" {
		return "badges:5", true
	}
	if id == "sabrina" {
		return "badges:6", true
	}
	if id == "blaine" {
		return "badges:7", true
	}
	if id == "giovanni" {
		return "badges:8", true
	}
	if id == "cinnabar" {
		return "reach:cinnabar island", true
	}
	return Profile().GoalFor(id)
}

func badge(name string) func(agent.Observation) bool {
	return func(obs agent.Observation) bool {
		for _, got := range obs.Badges {
			if strings.EqualFold(got, name) {
				return true
			}
		}
		return false
	}
}

func locationContains(fragment string) func(agent.Observation) bool {
	fragment = strings.ToLower(fragment)
	return func(obs agent.Observation) bool {
		return strings.Contains(strings.ToLower(obs.MapName), fragment) ||
			strings.Contains(strings.ToLower(string(obs.Location)), fragment)
	}
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
