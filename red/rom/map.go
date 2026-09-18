package rom

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/gen1rom"
	"github.com/maestroi/pokepilot/worldmodel"
)

// ErrInvalidMapID identifies a slot that is not a playable Red map. Unused
// slots still have entries in the ROM pointer tables; their bytes can parse
// successfully and invent incoming warps if treated as real headers.
var ErrInvalidMapID = errors.New("invalid Red map id")

// Defined by pokered/constants/map_constants.asm.
func validMapID(id uint8) bool {
	if id >= 0xf8 {
		return false
	}
	switch id {
	case 0x0b, 0x69, 0x6a, 0x6b, 0x6d, 0x6e, 0x6f, 0x70,
		0x72, 0x73, 0x74, 0x75, 0xcc, 0xcd, 0xce, 0xe7,
		0xed, 0xee, 0xf1, 0xf2, 0xf3, 0xf4:
		return false
	}
	return true
}

// Banked symbol addresses from pokered.sym, written bank:addr.
const (
	mapHeaderPointersBank uint8  = 0x00
	mapHeaderPointersAddr uint16 = 0x01AE
	mapHeaderBanksBank    uint8  = 0x03
	mapHeaderBanksAddr    uint16 = 0x423D
	tilesetsBank          uint8  = 0x03
	tilesetsAddr          uint16 = 0x47BE
)

type Warp = gen1rom.Warp
type Sign = gen1rom.Sign
type Object = gen1rom.Object
type Connection = worldmodel.Connection

const (
	MovementWalk = gen1rom.MovementWalk
	MovementStay = gen1rom.MovementStay
)

// MapHeader preserves the existing Red API while the shared byte decoder lives
// in gen1rom. Red still owns valid ids and the pointer-table locations.
type MapHeader gen1rom.MapHeader

func bankedOffset(bank uint8, addr uint16) (int, error) {
	return gen1rom.BankedOffset(bank, addr)
}

func redHeaderRef(rom []byte, mapID uint8) (gen1rom.HeaderRef, error) {
	bankOff, err := bankedOffset(mapHeaderBanksBank, mapHeaderBanksAddr)
	if err != nil {
		return gen1rom.HeaderRef{}, err
	}
	bankAt := bankOff + int(mapID)
	if bankAt >= len(rom) {
		return gen1rom.HeaderRef{}, fmt.Errorf("map %02x: bank table offset %d exceeds ROM of %d bytes", mapID, bankAt, len(rom))
	}
	bank := rom[bankAt]

	ptrOff, err := bankedOffset(mapHeaderPointersBank, mapHeaderPointersAddr)
	if err != nil {
		return gen1rom.HeaderRef{}, err
	}
	ptrAt := ptrOff + int(mapID)*2
	if ptrAt+2 > len(rom) {
		return gen1rom.HeaderRef{}, fmt.Errorf("map %02x: header pointer offset %d exceeds ROM of %d bytes", mapID, ptrAt, len(rom))
	}
	addr := uint16(rom[ptrAt]) | uint16(rom[ptrAt+1])<<8
	return gen1rom.HeaderRef{Bank: bank, Addr: addr}, nil
}

// ParseMap reads one Red/Blue map using the shared Gen-I header/object format.
func ParseMap(rom []byte, mapID uint8) (MapHeader, error) {
	if !validMapID(mapID) {
		return MapHeader{ID: mapID}, fmt.Errorf("map %02x: %w", mapID, ErrInvalidMapID)
	}
	ref, err := redHeaderRef(rom, mapID)
	if err != nil {
		return MapHeader{ID: mapID}, err
	}
	h, err := gen1rom.ParseMapAt(rom, mapID, ref)
	return MapHeader(h), err
}

// Blocks returns the raw block ids for a map.
func Blocks(rom []byte, h MapHeader) ([]byte, error) {
	return gen1rom.Blocks(rom, gen1rom.MapHeader(h))
}
