package agent

import (
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

// ObjectiveCatalog is the typed, game-supplied world vocabulary consumed by
// generic objective providers. Destinations and challenges carry semantic area
// identity only; native map coordinates stay inside the concrete adapter.
type ObjectiveCatalog struct {
	Starters        []CatalogStarter
	Destinations    []CatalogDestination
	Challenges      []CatalogChallenge
	LocalEncounters []CatalogEncounter
	Shop            *CatalogShop
	Interactables   []CatalogInteractable
	CurrentCenter   bool
}

type CatalogStarter struct {
	Starter skill.Starter
	Species SpeciesID
}

type CatalogDestination struct {
	Place    PlaceID
	Location LocationID
	X, Y     uint8
	Center   bool
}

type CatalogChallenge struct {
	Place    PlaceID
	Location LocationID
	Complete bool
}

type CatalogEncounter struct {
	Species  SpeciesID
	MinLevel uint8
	MaxLevel uint8
	Slots    int
}

type CatalogShopItem struct {
	Item ItemID
	Name string
}

type CatalogShop struct {
	Items []CatalogShopItem
}

type CatalogInteractableKind string

const (
	CatalogInteractablePerson  CatalogInteractableKind = "person"
	CatalogInteractableTrainer CatalogInteractableKind = "trainer"
	CatalogInteractableItem    CatalogInteractableKind = "item"
)

type CatalogInteractable struct {
	X, Y          uint8
	Kind          CatalogInteractableKind
	Item          ItemID
	Challengeable bool
	Defeated      bool
}

func objectiveCatalogEmpty(c ObjectiveCatalog) bool {
	return len(c.Starters) == 0 && len(c.Destinations) == 0 && len(c.Challenges) == 0 &&
		len(c.LocalEncounters) == 0 && c.Shop == nil && len(c.Interactables) == 0 && !c.CurrentCenter
}

func objectiveCatalogForObservation(obs Observation) ObjectiveCatalog {
	catalog := obs.Catalog
	if objectiveCatalogEmpty(catalog) {
		if provider, ok := objectiveCatalogProviderFor(obs.GameID); ok {
			catalog = provider.ObjectiveCatalog(obs)
		}
	}
	if len(catalog.LocalEncounters) == 0 {
		for _, wild := range obs.WildGrass {
			name := strings.ToLower(strings.TrimSpace(wild.Name))
			if name == "" {
				continue
			}
			catalog.LocalEncounters = append(catalog.LocalEncounters, CatalogEncounter{
				Species: SpeciesID(name), MinLevel: wild.MinLevel, MaxLevel: wild.MaxLevel, Slots: wild.Slots,
			})
		}
	}
	if catalog.Shop == nil && len(obs.MartStock) > 0 {
		shop := &CatalogShop{}
		for _, raw := range obs.MartStock {
			name := strings.ToLower(strings.TrimSpace(raw))
			if name == "" {
				continue
			}
			shop.Items = append(shop.Items, CatalogShopItem{Item: ItemID(name), Name: name})
		}
		catalog.Shop = shop
	}
	if len(catalog.Interactables) == 0 {
		for _, object := range obs.MapObjects {
			entry := CatalogInteractable{
				X: object.X, Y: object.Y, Kind: CatalogInteractableKind(object.Kind),
				Challengeable: object.Challengeable, Defeated: object.Defeated,
			}
			if object.Kind == string(CatalogInteractableItem) {
				name := strings.ToLower(strings.TrimSpace(object.Item))
				if name == "" || name == "unknown" {
					continue
				}
				entry.Item = ItemID(name)
			}
			catalog.Interactables = append(catalog.Interactables, entry)
		}
	}
	return normalizeObjectiveCatalog(catalog)
}

func normalizeObjectiveCatalog(c ObjectiveCatalog) ObjectiveCatalog {
	c.Starters = append([]CatalogStarter(nil), c.Starters...)
	c.Destinations = append([]CatalogDestination(nil), c.Destinations...)
	c.Challenges = append([]CatalogChallenge(nil), c.Challenges...)
	c.LocalEncounters = append([]CatalogEncounter(nil), c.LocalEncounters...)
	c.Interactables = append([]CatalogInteractable(nil), c.Interactables...)
	if c.Shop != nil {
		shop := &CatalogShop{Items: append([]CatalogShopItem(nil), c.Shop.Items...)}
		c.Shop = shop
	}

	sort.SliceStable(c.Destinations, func(i, j int) bool {
		if c.Destinations[i].Place != c.Destinations[j].Place {
			return c.Destinations[i].Place < c.Destinations[j].Place
		}
		if c.Destinations[i].Location != c.Destinations[j].Location {
			return c.Destinations[i].Location < c.Destinations[j].Location
		}
		if c.Destinations[i].Y != c.Destinations[j].Y {
			return c.Destinations[i].Y < c.Destinations[j].Y
		}
		return c.Destinations[i].X < c.Destinations[j].X
	})
	sort.SliceStable(c.Challenges, func(i, j int) bool { return c.Challenges[i].Place < c.Challenges[j].Place })
	sort.SliceStable(c.LocalEncounters, func(i, j int) bool { return c.LocalEncounters[i].Species < c.LocalEncounters[j].Species })
	if c.Shop != nil {
		sort.SliceStable(c.Shop.Items, func(i, j int) bool { return c.Shop.Items[i].Item < c.Shop.Items[j].Item })
	}
	return c
}

func (c ObjectiveCatalog) destination(place PlaceID) (CatalogDestination, bool) {
	for _, destination := range c.Destinations {
		if destination.Place == place {
			return destination, true
		}
	}
	return CatalogDestination{}, false
}

func (c ObjectiveCatalog) shopItem(name string) (ItemID, bool) {
	if c.Shop == nil {
		return "", false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	for _, item := range c.Shop.Items {
		if strings.EqualFold(item.Name, name) || strings.EqualFold(string(item.Item), name) {
			return item.Item, true
		}
	}
	return "", false
}
