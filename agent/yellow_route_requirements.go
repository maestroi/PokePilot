package agent

import (
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/skill"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

// RouteRequirements composes the shared early-Kanto gates with Yellow's one
// extra mandatory Viridian story transition. Generic travel therefore cannot
// accidentally walk into the scripted Old Man demo, while the atomic Boulder
// progression remains free to own and complete that transition itself.
func (a *yellowObjectiveAdapter) RouteRequirements(obs Observation) []RouteBlockage {
	out := append([]RouteBlockage(nil), gen1EarlyRouteRequirements(obs, "yellow")...)

	if obs.Story.Has(gen1.ProgressPokedexAcquired) &&
		!obs.Story.Has(yellowprofile.ProgressYellowViridianCatchTraining) &&
		obs.Map != gen1Route2Map {
		for _, name := range skill.PlaceNames() {
			destination, ok := skill.Place(name)
			if !ok || destination.Map != gen1Route2Map {
				continue
			}
			out = append(out, redStoryRouteBlockage(
				PlaceID(name),
				"yellow:story:viridian_catch_training",
				RoutePrerequisiteLink{Progress: yellowprofile.ProgressYellowViridianCatchTraining},
			))
		}
	}

	return dedupeRouteBlockages(out)
}
