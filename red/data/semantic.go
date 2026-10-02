// Package data contains stable Pokémon Red vocabulary shared by the Red
// profile and Red-owned runtime adapters. Raw indexes never leave this package
// as planner-facing identities.
package data

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
)

// itemTable is the canonical Red item vocabulary: the real bag items
// (constants/item_constants.asm, $01-$53) plus the five HMs. Raw indexes never
// leave this package as planner-facing identities, and the planner's executable
// item whitelist is this table, so it must cover every item that can appear as
// a generic world pickup. A real pickup whose name is absent here is rejected
// at validation as an "unknown Red item" and stops the run (#2388: carbos in
// Safari Zone East).
//
// A few items are deliberately NOT in this table and are resolved through a
// specific trusted path instead: the fishing rods (old/good/super rod) via dex
// fishing and the old-rod NPC reward, and the master ball, which is withheld as
// a sensitive progression item. TMs ($C9-$FA) are the agent's dynamic
// vocabulary, registered at init time. The unused slots ($2C, $32) and the
// badge event-flag overloads ($15-$1C) are not bag items and are omitted.
var itemTable = map[string]uint8{
	// Capture (master ball is withheld; see the note above)
	"ultra ball": 0x02, "great ball": 0x03, "pokeball": 0x04,
	// Key items & progression
	"town map": 0x05, "bicycle": 0x06, "surfboard": 0x07, "safari ball": 0x08,
	"pokedex": 0x09, "escape rope": 0x1D, "old amber": 0x1F,
	// Healing
	"antidote": 0x0B, "burn heal": 0x0C, "ice heal": 0x0D,
	"awakening": 0x0E, "parlyz heal": 0x0F, "full restore": 0x10,
	"max potion": 0x11, "hyper potion": 0x12, "super potion": 0x13, "potion": 0x14,
	"full heal": 0x34, "revive": 0x35, "max revive": 0x36,
	// Repels
	"repel": 0x1E, "super repel": 0x38, "max repel": 0x39,
	// Evolution stones
	"moon stone": 0x0A, "fire stone": 0x20, "thunder stone": 0x21,
	"water stone": 0x22, "leaf stone": 0x2F,
	// Vitamins
	"hp up": 0x23, "protein": 0x24, "iron": 0x25, "carbos": 0x26, "calcium": 0x27,
	// Fossils, candies & key items
	"rare candy": 0x28, "dome fossil": 0x29, "helix fossil": 0x2A,
	"secret key": 0x2B, "bike voucher": 0x2D, "x accuracy": 0x2E,
	"card key": 0x30, "nugget": 0x31, "poke doll": 0x33,
	"guard spec": 0x37, "dire hit": 0x3A,
	// Battle consumables & key items
	"coin": 0x3B, "fresh water": 0x3C, "soda pop": 0x3D, "lemonade": 0x3E,
	"s.s. ticket": 0x3F, "gold teeth": 0x40,
	"x attack": 0x41, "x defend": 0x42, "x speed": 0x43, "x special": 0x44,
	"coin case": 0x45, "oaks parcel": 0x46, "itemfinder": 0x47,
	"silph scope": 0x48, "poke flute": 0x49,
	"lift key": 0x4A, "exp all": 0x4B,
	// (fishing rods old/good/super are resolved via dex fishing, not here)
	// PP restorers
	"pp up": 0x4F, "ether": 0x50, "max ether": 0x51, "elixer": 0x52, "max elixer": 0x53,
	// HMs
	"hm01": 0xC4, "hm02": 0xC5, "hm03": 0xC6, "hm04": 0xC7, "hm05": 0xC8,
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
