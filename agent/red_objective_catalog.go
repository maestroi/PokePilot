package agent

import (
	"sort"

	"github.com/maestroi/pokepilot/skill"
)

// ObjectiveCatalog makes Red's world vocabulary available to generic provider
// policy without requiring those providers to call Red-oriented skill discovery
// helpers themselves.
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
		catalog.Destinations = append(catalog.Destinations, CatalogDestination{
			Place:     PlaceID(name),
			NativeMap: destination.Map,
			X:         destination.X,
			Y:         destination.Y,
			Center:    isCenterNameForRed(destination.Map),
		})
	}

	if gym, ok := skill.GymAt(obs.Map); ok {
		catalog.Challenges = append(catalog.Challenges, CatalogChallenge{
			Place:     gym.Place,
			NativeMap: gym.Map,
			Complete:  hasBadge(obs, gym.Badge),
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

	// PlaceNames is stable today, but the adapter contract promises deterministic
	// provider input independently of Red table iteration details.
	sort.SliceStable(catalog.Destinations, func(i, j int) bool {
		return catalog.Destinations[i].Place < catalog.Destinations[j].Place
	})
	return normalizeObjectiveCatalog(catalog)
}

func isCenterNameForRed(mapID uint8) bool {
	for _, name := range skill.PlaceNames() {
		destination, ok := skill.Place(name)
		if ok && destination.Map == mapID && isCenterFromPlaceName(name) {
			return true
		}
	}
	return false
}

func isCenterFromPlaceName(name string) bool {
	// Red's place vocabulary names every healing destination as a Pokemon
	// Center. This helper stays Red-owned; generic provider code only sees the
	// resulting Center boolean.
	for _, marker := range []string{"pokecenter", "pokemon center"} {
		if containsFold(name, marker) {
			return true
		}
	}
	return false
}

func containsFold(s, want string) bool {
	if len(want) == 0 {
		return true
	}
	for i := 0; i+len(want) <= len(s); i++ {
		match := true
		for j := range want {
			a, b := s[i+j], want[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
