// Package data contains stable Pokémon Red vocabulary shared by the Red
// profile and Red-owned runtime adapters. Raw indexes never leave this package
// as planner-facing identities.
package data

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
)

var itemTable = map[string]uint8{
	"pokeball": 0x04, "great ball": 0x03,
	"potion": 0x14, "super potion": 0x13, "hyper potion": 0x12, "max potion": 0x11,
	"antidote": 0x0B, "burn heal": 0x0C, "ice heal": 0x0D,
	"awakening": 0x0E, "parlyz heal": 0x0F, "full restore": 0x10,
	"repel": 0x1E, "escape rope": 0x1D,
	"silph scope": 0x48, "poke flute": 0x49,
	"hm03": 0xC6, "hm04": 0xC7,
}

var itemByID = reverse(itemTable)

func reverse(in map[string]uint8) map[uint8]string {
	out := make(map[uint8]string, len(in))
	for name, id := range in {
		out[id] = name
	}
	return out
}

func SpeciesCount() int { return gen1.SpeciesCount() }

func SpeciesName(raw uint8) (string, bool) { return gen1.SpeciesName(raw) }

func Species(raw uint8) (game.SpeciesID, bool) { return gen1.Species(raw) }

func SpeciesRaw(id game.SpeciesID) (uint8, bool) { return gen1.SpeciesRaw(id) }

func ItemName(raw uint8) (string, bool) {
	name, ok := itemByID[raw]
	return name, ok
}

func Item(raw uint8) (game.ItemID, bool) {
	name, ok := ItemName(raw)
	if !ok {
		return "", false
	}
	return game.ItemID(name), true
}

func ItemRaw(id game.ItemID) (uint8, bool) {
	raw, ok := itemTable[game.CanonicalID(string(id))]
	return raw, ok
}

// LegacyMutableItemTables exposes the canonical Red item maps only to the
// existing Red-owned agent initializers that register economy and TM/HM
// vocabulary. The returned maps are the package's actual maps, so those
// one-time init registrations are immediately visible through Item/ItemRaw.
// New profile code should prefer the typed lookup helpers above.
func LegacyMutableItemTables() (map[string]uint8, map[uint8]string) {
	return itemTable, itemByID
}
