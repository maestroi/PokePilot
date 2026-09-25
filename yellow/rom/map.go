// Package rom owns Pokémon Yellow ROM layout and static-data decoding.
package rom

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/gen1rom"
)

var ErrInvalidMapID = errors.New("invalid Yellow map id")

type Warp = gen1rom.Warp
type Sign = gen1rom.Sign
type Object = gen1rom.Object
type Connection = gen1rom.Connection

const (
	MovementWalk = gen1rom.MovementWalk
	MovementStay = gen1rom.MovementStay
)

type MapHeader gen1rom.MapHeader

func validMapID(id uint8) bool {
	if int(id) >= len(yellowHeaderRefs) {
		return false
	}
	ref := yellowHeaderRefs[id]
	return ref.Bank != 0 || ref.Addr != 0
}

func MapName(id uint8) string {
	if int(id) >= len(yellowMapNames) {
		return ""
	}
	if !validMapID(id) {
		return ""
	}
	return yellowMapNames[id]
}

func MapIDs() []uint8 {
	ids := make([]uint8, 0, yellowPlayableMapCount)
	for id := 0; id < len(yellowHeaderRefs); id++ {
		if validMapID(uint8(id)) {
			ids = append(ids, uint8(id))
		}
	}
	return ids
}

func ParseMap(rom []byte, mapID uint8) (MapHeader, error) {
	if !validMapID(mapID) {
		return MapHeader{ID: mapID}, fmt.Errorf("map %02x: %w", mapID, ErrInvalidMapID)
	}
	h, err := gen1rom.ParseMapAt(rom, mapID, yellowHeaderRefs[mapID])
	return MapHeader(h), err
}

func Blocks(rom []byte, h MapHeader) ([]byte, error) {
	return gen1rom.Blocks(rom, gen1rom.MapHeader(h))
}
