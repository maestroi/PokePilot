package agent

import (
	"sort"

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
	catalog := ObjectiveCatalog{
		Starters: []CatalogStarter{
			{Starter: skill.StarterCharmander, Species: "charmander"},
			{Starter: skill.StarterSquirtle, Species: "squirtle"},
			{Starter: skill.StarterBulbasaur, Species: "bulbasaur"},
		},
		CurrentCenter: isCenter(obs.MapName),
	}

	for _, name := range skill.PlaceNames() {
		destination, ok := skill.Place(name)
		if !ok {
			continue
		}
		entry := CatalogDestination{
			Place:    PlaceID(name),
			Location: redLocationID(obs.GameID, destination.Map),
			Kind:     destination.Kind,
			Center:   isCenter(state.MapName(destination.Map)),
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
			Place:    gym.Place,
			Location: redLocationID(obs.GameID, gym.Map),
			Complete: hasBadge(obs, gym.Badge),
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
