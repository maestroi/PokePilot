package rom

import (
	"fmt"

	"github.com/maestroi/pokepilot/gen1rom"
	"github.com/maestroi/pokepilot/worldmodel"
)

const (
	yellowFirstIndoorMap = 0x25
	yellowForestTileset  = 3
	yellowSurfWaterTile  = 0x14
)

type EncounterCell struct {
	X uint8
	Y uint8
}

func HasWildSpecies(romData []byte, mapID uint8, habitat string, species uint8) (bool, error) {
	encounters, err := WildEncounters(romData)
	if err != nil {
		return false, err
	}
	for _, enc := range encounters {
		if enc.MapID == mapID && enc.Habitat == habitat && enc.Species == species {
			return true, nil
		}
	}
	return false, nil
}

func hasWildHabitat(romData []byte, mapID uint8, habitat string) (bool, error) {
	encounters, err := WildEncounters(romData)
	if err != nil {
		return false, err
	}
	for _, enc := range encounters {
		if enc.MapID == mapID && enc.Habitat == habitat {
			return true, nil
		}
	}
	return false, nil
}

// GrassEncounterCells returns walkable coordinates on which Yellow performs
// grass encounter rolls. Outdoors and the FOREST tileset require the
// tileset's grass collision tile; indoor non-FOREST maps with a grass wild
// table (caves/dungeons) roll on every ordinary walkable tile.
func GrassEncounterCells(romData []byte, mapID uint8) ([]EncounterCell, error) {
	hasGrass, err := hasWildHabitat(romData, mapID, gen1rom.HabitatGrass)
	if err != nil {
		return nil, err
	}
	if !hasGrass {
		return nil, nil
	}

	h, err := ParseMap(romData, mapID)
	if err != nil {
		return nil, err
	}
	spec, err := h.WorldGridSpec(romData, nil, worldmodel.TraversalLand)
	if err != nil {
		return nil, err
	}
	if len(spec.Walkable) != spec.Width*spec.Height ||
		len(spec.CollisionTile) != spec.Width*spec.Height {
		return nil, fmt.Errorf("yellow rom: map %#02x has invalid encounter grid payload", mapID)
	}

	allWalkable := mapID >= yellowFirstIndoorMap && h.Tileset != yellowForestTileset
	grassTile := uint8(0)
	if !allWalkable {
		base, err := gen1rom.BankedOffset(yellowTilesetsBank, yellowTilesetsAddr)
		if err != nil {
			return nil, err
		}
		entry := base + int(h.Tileset)*tilesetEntryLen
		if entry+tilesetEntryLen > len(romData) {
			return nil, fmt.Errorf("yellow rom: tileset %d entry exceeds ROM", h.Tileset)
		}
		grassTile = romData[entry+10]
	}

	out := make([]EncounterCell, 0, spec.Width*spec.Height/4)
	for y := 0; y < spec.Height; y++ {
		for x := 0; x < spec.Width; x++ {
			i := y*spec.Width + x
			if !spec.Walkable[i] {
				continue
			}
			if !allWalkable && spec.CollisionTile[i] != grassTile {
				continue
			}
			out = append(out, EncounterCell{X: uint8(x), Y: uint8(y)})
		}
	}
	return out, nil
}


// WaterEncounterCells returns Yellow Surf encounter coordinates on a map with
// a non-zero water encounter table. The water traversal grid supplies the
// game-specific land/water pair semantics; the tile identity remains
// Yellow-owned here.
func WaterEncounterCells(romData []byte, mapID uint8) ([]EncounterCell, error) {
	hasWater, err := hasWildHabitat(romData, mapID, gen1rom.HabitatWater)
	if err != nil {
		return nil, err
	}
	if !hasWater {
		return nil, nil
	}
	h, err := ParseMap(romData, mapID)
	if err != nil {
		return nil, err
	}
	spec, err := h.WorldGridSpec(romData, nil, worldmodel.TraversalWater)
	if err != nil {
		return nil, err
	}
	out := make([]EncounterCell, 0, spec.Width*spec.Height/4)
	for y := 0; y < spec.Height; y++ {
		for x := 0; x < spec.Width; x++ {
			i := y*spec.Width + x
			if !spec.Walkable[i] {
				continue
			}
			if spec.FieldTile[i] != yellowSurfWaterTile && spec.CollisionTile[i] != yellowSurfWaterTile {
				continue
			}
			out = append(out, EncounterCell{X: uint8(x), Y: uint8(y)})
		}
	}
	return out, nil
}
