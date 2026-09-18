package rom

import "github.com/maestroi/pokepilot/gen1rom"

// MartItems returns the item ids a map's mart clerk stocks, in shelf order.
// The script format is shared Gen I; Red owns only the map/header location.
func MartItems(romData []byte, mapID uint8) ([]uint8, error) {
	h, err := ParseMap(romData, mapID)
	if err != nil {
		return nil, err
	}
	return gen1rom.MartItems(romData, gen1rom.MapHeader(h))
}

// MartClerkPosition returns the ROM home coordinate of the object whose text
// script owns the current map's mart shelf.
func MartClerkPosition(romData []byte, mapID uint8) (uint8, uint8, error) {
	h, err := ParseMap(romData, mapID)
	if err != nil {
		return 0, 0, err
	}
	return gen1rom.MartClerkPosition(romData, gen1rom.MapHeader(h))
}
