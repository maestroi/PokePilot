package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

var (
	yellowDestinationsOnce sync.Once
	yellowDestinations     []CatalogDestination
	yellowDestinationsErr  error
)

func yellowObjectiveCatalog(romData []byte, obs Observation) (ObjectiveCatalog, error) {
	catalog := ObjectiveCatalog{
		CurrentCenter: strings.Contains(strings.ToUpper(obs.MapName), "POKECENTER"),
	}
	if obs.PartyCount == 0 {
		catalog.Starters = []CatalogStarter{{Species: "pikachu"}}
	}

	yellowDestinationsOnce.Do(func() {
		yellowDestinations, yellowDestinationsErr = buildYellowDestinations(romData)
	})
	if yellowDestinationsErr != nil {
		return ObjectiveCatalog{}, yellowDestinationsErr
	}
	catalog.Destinations = append(catalog.Destinations, yellowDestinations...)

	// Mart inventory is static ROM data but relevant only on the current map.
	// A map without a mart is ordinary, not an observation failure.
	if ids, err := yellowrom.MartItems(romData, obs.Map); err == nil && len(ids) > 0 {
		shop := &CatalogShop{}
		for _, raw := range ids {
			name, err := yellowrom.ItemName(romData, raw)
			if err != nil || strings.TrimSpace(name) == "" {
				continue
			}
			semantic := game.CanonicalID(name)
			shop.Items = append(shop.Items, CatalogShopItem{Item: ItemID(semantic), Name: semantic})
		}
		if len(shop.Items) > 0 {
			catalog.Shop = shop
		}
	}

	return normalizeObjectiveCatalog(catalog), nil
}

func buildYellowDestinations(romData []byte) ([]CatalogDestination, error) {
	out := make([]CatalogDestination, 0, len(yellowrom.MapIDs()))
	for _, mapID := range yellowrom.MapIDs() {
		name := yellowrom.MapName(mapID)
		if name == "" {
			continue
		}
		h, err := yellowrom.ParseMap(romData, mapID)
		if err != nil {
			return nil, fmt.Errorf("Yellow destination %s: parse map: %w", name, err)
		}
		grid, err := world.Build(romData, h)
		if err != nil {
			return nil, fmt.Errorf("Yellow destination %s: build grid: %w", name, err)
		}
		x, y, ok := representativeYellowTile(grid)
		if !ok {
			continue
		}
		place := PlaceID(semanticLocation(name))
		out = append(out, CatalogDestination{
			Place:    place,
			Location: yellowLocationID(yellowprofile.GameID, mapID),
			X:        uint8(x),
			Y:        uint8(y),
			Center:   strings.Contains(name, "POKECENTER"),
		})
	}
	return out, nil
}

// representativeYellowTile chooses a stable point on the largest walkable
// component, closest to map center. A map with multiple disconnected pockets
// (Route 4 is the classic Gen-I example) therefore does not pick a tiny sealed
// pocket merely because its first tile appears earlier in scan order.
func representativeYellowTile(grid *world.Grid) (int, int, bool) {
	if grid == nil || grid.Width <= 0 || grid.Height <= 0 {
		return 0, 0, false
	}
	components := world.Components(grid)
	counts := map[int]int{}
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if id := components[y][x]; id > 0 {
				counts[id]++
			}
		}
	}
	largest, largestCount := 0, 0
	for id, count := range counts {
		if count > largestCount || count == largestCount && id < largest {
			largest, largestCount = id, count
		}
	}
	if largest == 0 {
		return 0, 0, false
	}

	cx, cy := (grid.Width-1)/2, (grid.Height-1)/2
	bestX, bestY, bestDistance := -1, -1, int(^uint(0)>>1)
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if components[y][x] != largest {
				continue
			}
			d := absInt(x-cx) + absInt(y-cy)
			if d < bestDistance || d == bestDistance && (bestY < 0 || y < bestY || y == bestY && x < bestX) {
				bestX, bestY, bestDistance = x, y, d
			}
		}
	}
	return bestX, bestY, bestX >= 0
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
