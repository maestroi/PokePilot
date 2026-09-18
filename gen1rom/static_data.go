package gen1rom

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/gen1"
)

const (
	HabitatGrass = "grass"
	HabitatWater = "water"
	wildSlots    = 10
)

type WildLayout struct {
	PointerBank uint8
	PointerAddr uint16
	MapCount    int
}

type WildEncounter struct {
	MapID   uint8
	Habitat string
	Species uint8
	Level   uint8
}

func WildEncounters(rom []byte, layout WildLayout) ([]WildEncounter, error) {
	base, err := BankedOffset(layout.PointerBank, layout.PointerAddr)
	if err != nil {
		return nil, fmt.Errorf("WildDataPointers: %w", err)
	}
	if layout.MapCount < 0 || base+2*layout.MapCount > len(rom) {
		return nil, fmt.Errorf("WildDataPointers at %#x for %d maps exceeds ROM of %d bytes", base, layout.MapCount, len(rom))
	}
	var out []WildEncounter
	for mapID := 0; mapID < layout.MapCount; mapID++ {
		p := base + 2*mapID
		addr := uint16(rom[p]) | uint16(rom[p+1])<<8
		if addr == 0 || addr == 0xffff {
			continue
		}
		rec, err := BankedOffset(layout.PointerBank, addr)
		if err != nil {
			return nil, fmt.Errorf("wild data map %#02x: %w", mapID, err)
		}
		grass, next, err := readWildHalf(rom, rec, uint8(mapID), HabitatGrass)
		if err != nil {
			return nil, err
		}
		out = append(out, grass...)
		water, _, err := readWildHalf(rom, next, uint8(mapID), HabitatWater)
		if err != nil {
			return nil, err
		}
		out = append(out, water...)
	}
	return out, nil
}

func readWildHalf(rom []byte, off int, mapID uint8, habitat string) ([]WildEncounter, int, error) {
	if off >= len(rom) {
		return nil, 0, fmt.Errorf("wild %s rate for map %#02x at %#x exceeds ROM of %d bytes", habitat, mapID, off, len(rom))
	}
	rate := rom[off]
	off++
	if rate == 0 {
		return nil, off, nil
	}
	if off+2*wildSlots > len(rom) {
		return nil, 0, fmt.Errorf("wild %s slots for map %#02x at %#x exceed ROM of %d bytes", habitat, mapID, off, len(rom))
	}
	out := make([]WildEncounter, 0, wildSlots)
	for i := 0; i < wildSlots; i++ {
		level, species := rom[off], rom[off+1]
		off += 2
		if species != 0 {
			out = append(out, WildEncounter{MapID: mapID, Habitat: habitat, Species: species, Level: level})
		}
	}
	return out, off, nil
}

type MoveLayout struct {
	Offset   int
	EntryLen int
}

type Move struct {
	ID       uint8
	Effect   uint8
	Power    uint8
	Type     uint8
	Accuracy uint8
	PP       uint8
}

func LookupMove(rom []byte, id uint8, layout MoveLayout) (Move, error) {
	if id == 0 {
		return Move{}, fmt.Errorf("move id 0 is the empty slot")
	}
	if layout.EntryLen < 6 {
		return Move{}, fmt.Errorf("move entry length %d is shorter than 6", layout.EntryLen)
	}
	off := layout.Offset + (int(id)-1)*layout.EntryLen
	if off < 0 || off+layout.EntryLen > len(rom) {
		return Move{}, fmt.Errorf("move %d at offset %#x exceeds ROM of %d bytes", id, off, len(rom))
	}
	e := rom[off : off+layout.EntryLen]
	if e[0] != id {
		return Move{}, fmt.Errorf("move table entry %d has animation %d, want %d", id, e[0], id)
	}
	return Move{ID: id, Effect: e[1], Power: e[2], Type: e[3], Accuracy: e[4], PP: e[5]}, nil
}

type SpeciesLayout struct {
	NamesOffset       int
	NameLength        int
	InternalCount     int
	PokedexOrderOffset int
	PokedexOrderLen   int
	BaseStatsOffset   int
	BaseStatsEntryLen int
}

type SpeciesBaseStats struct {
	Dex       uint8
	HP        uint8
	Attack    uint8
	Defense   uint8
	Speed     uint8
	Special   uint8
	Type1     uint8
	Type2     uint8
	CatchRate uint8
	Moves     [4]uint8
	Growth    uint8
}

func SpeciesName(rom []byte, species uint8, layout SpeciesLayout) (string, error) {
	if species == 0 || int(species) > layout.InternalCount {
		return "", fmt.Errorf("species %#02x outside internal table", species)
	}
	off := layout.NamesOffset + (int(species)-1)*layout.NameLength
	if layout.NameLength <= 0 || off < 0 || off+layout.NameLength > len(rom) {
		return "", fmt.Errorf("species %#02x name at %#x exceeds ROM of %d bytes", species, off, len(rom))
	}
	name := strings.TrimSpace(gen1.DecodeName(rom[off : off+layout.NameLength]))
	if name == "" || strings.HasPrefix(strings.ToUpper(name), "MISSINGNO") {
		return "", fmt.Errorf("species %#02x is not a canonical species", species)
	}
	return name, nil
}

func InternalSpeciesDexNumber(rom []byte, species uint8, layout SpeciesLayout) (uint8, error) {
	if species == 0 || int(species) > layout.PokedexOrderLen {
		return 0, fmt.Errorf("species %#02x outside Pokédex order", species)
	}
	at := layout.PokedexOrderOffset + int(species) - 1
	if at < 0 || at >= len(rom) {
		return 0, fmt.Errorf("PokedexOrder[%#02x] at %#x exceeds ROM of %d bytes", species, at, len(rom))
	}
	dex := rom[at]
	if dex == 0 || dex > 151 {
		return 0, fmt.Errorf("species %#02x maps to invalid Pokédex number %d", species, dex)
	}
	return dex, nil
}

func LookupSpeciesBaseStats(rom []byte, species uint8, layout SpeciesLayout) (SpeciesBaseStats, error) {
	dex, err := InternalSpeciesDexNumber(rom, species, layout)
	if err != nil {
		return SpeciesBaseStats{}, err
	}
	if layout.BaseStatsEntryLen < 20 {
		return SpeciesBaseStats{}, fmt.Errorf("base stats entry length %d is too short", layout.BaseStatsEntryLen)
	}
	at := layout.BaseStatsOffset + (int(dex)-1)*layout.BaseStatsEntryLen
	if at < 0 || at+layout.BaseStatsEntryLen > len(rom) {
		return SpeciesBaseStats{}, fmt.Errorf("base stats dex %d at %#x exceed ROM of %d bytes", dex, at, len(rom))
	}
	e := rom[at : at+layout.BaseStatsEntryLen]
	if e[0] != dex {
		return SpeciesBaseStats{}, fmt.Errorf("base stats dex %d identifies dex %d", dex, e[0])
	}
	out := SpeciesBaseStats{
		Dex: dex, HP: e[1], Attack: e[2], Defense: e[3], Speed: e[4], Special: e[5],
		Type1: e[6], Type2: e[7], CatchRate: e[8], Growth: e[19],
	}
	copy(out.Moves[:], e[15:19])
	return out, nil
}

type StringListLayout struct {
	Offset int
	Count  int
}

func ListName(rom []byte, id uint8, layout StringListLayout) (string, error) {
	if id == 0 || int(id) > layout.Count {
		return "", fmt.Errorf("string-list id %d outside 1..%d", id, layout.Count)
	}
	at := layout.Offset
	for n := 1; n <= layout.Count; n++ {
		if at >= len(rom) {
			return "", fmt.Errorf("string list entry %d at %#x exceeds ROM of %d bytes", n, at, len(rom))
		}
		end := at
		for end < len(rom) && rom[end] != 0x50 {
			end++
		}
		if end >= len(rom) {
			return "", fmt.Errorf("unterminated string list entry %d at %#x", n, at)
		}
		if n == int(id) {
			return strings.TrimSpace(gen1.DecodeTiles(rom[at:end])), nil
		}
		at = end + 1
	}
	return "", fmt.Errorf("string-list id %d not found", id)
}
