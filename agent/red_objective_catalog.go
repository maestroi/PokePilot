package agent

import (
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func init() {
	for _, id := range gen1Games {
		registerObjectiveCatalogProvider(id, &redObjectiveAdapter{})
	}
}

func (a *redObjectiveAdapter) ObjectiveCatalog(obs Observation) ObjectiveCatalog {
	return redObjectiveCatalog(obs)
}

func redObjectiveCatalog(obs Observation) ObjectiveCatalog {
	return gen1ObjectiveCatalog(obs, gen1CatalogFacts{
		Location: redLocationID,
		Starters: []CatalogStarter{
			{Starter: skill.StarterCharmander, Species: "charmander"},
			{Starter: skill.StarterSquirtle, Species: "squirtle"},
			{Starter: skill.StarterBulbasaur, Species: "bulbasaur"},
		},
		ChallengeProfiles: redProgressionChallengeProfiles(),
	})
}

// gen1CatalogFacts are the game-owned parts of a Gen-I objective catalog.
// Everything else (destinations by map, gyms, encounters, shop stock,
// interactables) is decoded from the shared engine and the cartridge's own
// ROM tables, so Red, Blue and Yellow build it with one function.
type gen1CatalogFacts struct {
	// Location names a native map in the game's knowledge topology.
	Location func(game.GameID, uint8) LocationID
	// Starters are the starter choices the game's opening offers.
	Starters []CatalogStarter
	// ChallengeProfiles are the story challenges the game's adapter can run.
	ChallengeProfiles []CatalogChallengeProfile
}

func gen1ObjectiveCatalog(obs Observation, facts gen1CatalogFacts) ObjectiveCatalog {
	catalog := ObjectiveCatalog{
		Starters:          facts.Starters,
		ChallengeProfiles: facts.ChallengeProfiles,
		CurrentCenter:     isCenter(obs.MapName),
	}

	for _, name := range skill.PlaceNames() {
		destination, ok := skill.Place(name)
		if !ok {
			continue
		}
		entry := CatalogDestination{
			Place:    PlaceID(name),
			Location: facts.Location(obs.GameID, destination.Map),
			Kind:     destination.Kind,
			Center:   strings.HasSuffix(name, "pokemon center") || isCenter(state.MapName(destination.Map)),
		}
		switch destination.Kind {
		case skill.DestinationArea:
			entry.Area = destination.Area
		case skill.DestinationExactTile, skill.DestinationInteraction:
			entry.X, entry.Y = destination.X, destination.Y
		}
		catalog.Destinations = append(catalog.Destinations, entry)
	}

	if gym, ok := skill.GymAt(obs.Map); ok {
		catalog.Challenges = append(catalog.Challenges, CatalogChallenge{
			Place:     gym.Place,
			Location:  facts.Location(obs.GameID, gym.Map),
			Complete:  hasBadge(obs, gym.Badge),
			Readiness: redGymReadinessProfile(gym.Badge),
		})
	}

	for _, wild := range obs.WildGrass {
		species, ok := SpeciesByName(wild.Name)
		if !ok {
			continue
		}
		catalog.LocalEncounters = append(catalog.LocalEncounters, CatalogEncounter{
			Species: species, MinLevel: wild.MinLevel, MaxLevel: wild.MaxLevel, Slots: wild.Slots,
		})
	}

	if isMart(obs.MapName) || len(obs.MartStock) > 0 {
		shop := &CatalogShop{}
		for _, name := range obs.MartStock {
			item, ok := ItemByName(name)
			if !ok {
				continue
			}
			shop.Items = append(shop.Items, CatalogShopItem{Item: item, Name: name})
		}
		catalog.Shop = shop
	}

	for _, object := range obs.MapObjects {
		entry := CatalogInteractable{
			X: object.X, Y: object.Y, Kind: CatalogInteractableKind(object.Kind),
			Challengeable: object.Challengeable, Defeated: object.Defeated,
		}
		if object.Kind == string(CatalogInteractableItem) {
			item, ok := ItemByName(object.Item)
			if !ok {
				continue
			}
			entry.Item = item
		}
		catalog.Interactables = append(catalog.Interactables, entry)
	}

	sort.SliceStable(catalog.Destinations, func(i, j int) bool {
		return catalog.Destinations[i].Place < catalog.Destinations[j].Place
	})
	return normalizeObjectiveCatalog(catalog)
}
