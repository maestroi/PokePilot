package skill

import (
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	fuchsiaCityMap          uint8 = 0x07
	route12Map              uint8 = 0x17
	route13Map              uint8 = 0x18
	route14Map              uint8 = 0x19
	route15Map              uint8 = 0x1A
	route15Gate1FMap        uint8 = 0xB8
	fuchsiaMartMap          uint8 = 0x98
	fuchsiaPokemonCenterMap uint8 = 0x9A
	wardensHouseMap         uint8 = 0x9B
	safariZoneGateMap       uint8 = 0x9C
	fuchsiaGymMap           uint8 = 0x9D
	safariZoneEastMap       uint8 = 0xD9
	safariZoneNorthMap      uint8 = 0xDA
	safariZoneWestMap       uint8 = 0xDB
	safariZoneCenterMap     uint8 = 0xDC
	safariZoneSecretHouse   uint8 = 0xDE

	goldTeethItem        uint8 = 0x40
	pokeFluteItemFuchsia uint8 = 0x49
	hm03SurfItem         uint8 = 0xC6
	hm04StrengthItem     uint8 = 0xC7

	soulBadgeMask uint8 = 1 << 4
)

// FuchsiaProgressionAvailable reports whether the post-Pokemon-Tower story
// slice can sensibly begin or resume on mapID. The route deliberately uses
// Lavender -> Route 12 -> Routes 13/14/15 -> Fuchsia: it exercises the Poké
// Flute Snorlax gate without introducing the Bicycle/Cycling Road dependency.
func FuchsiaProgressionAvailable(mapID uint8) bool {
	switch mapID {
	case mrFujisHouseMap, lavenderTownMap, lavenderPokemonCenterMap,
		route12Map, route13Map, route14Map, route15Map, route15Gate1FMap,
		fuchsiaCityMap, fuchsiaMartMap, fuchsiaPokemonCenterMap, wardensHouseMap,
		safariZoneGateMap, fuchsiaGymMap,
		safariZoneEastMap, safariZoneNorthMap, safariZoneWestMap, safariZoneCenterMap,
		safariZoneSecretHouse:
		return true
	default:
		return false
	}
}

// FuchsiaProgressionReady enforces the handoff from #32. Owning the Poké
// Flute is the only story prerequisite this slice should assume; reaching a
// particular map is handled independently by FuchsiaProgressionAvailable so
// interrupted runs remain resumable.
func FuchsiaProgressionReady(mem *state.Mem) bool {
	_, count := bagEntry(mem, pokeFluteItemFuchsia)
	return count > 0
}

// FuchsiaProgressionComplete is the concrete positive postcondition for #33.
// Koga's battle alone is insufficient, as are either Safari reward alone: the
// slice is complete only once the Soul Badge, HM03 Surf, and HM04 Strength are
// all positively present in RAM/inventory.
func FuchsiaProgressionComplete(mem *state.Mem) bool {
	if mem.U8(sym.ObtainedBadges)&soulBadgeMask == 0 {
		return false
	}
	_, surf := bagEntry(mem, hm03SurfItem)
	_, strength := bagEntry(mem, hm04StrengthItem)
	return surf > 0 && strength > 0
}
