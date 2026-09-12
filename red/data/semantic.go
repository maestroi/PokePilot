// Package data contains stable Pokémon Red vocabulary shared by the Red
// profile and Red-owned runtime adapters. Raw indexes never leave this package
// as planner-facing identities.
package data

import "github.com/maestroi/pokepilot/game"

var speciesTable = map[string]uint8{
	"rhydon": 0x01, "kangaskhan": 0x02, "nidoran♂": 0x03, "clefairy": 0x04,
	"spearow": 0x05, "voltorb": 0x06, "nidoking": 0x07, "slowbro": 0x08,
	"ivysaur": 0x09, "exeggutor": 0x0A, "lickitung": 0x0B, "exeggcute": 0x0C,
	"grimer": 0x0D, "gengar": 0x0E, "nidoran♀": 0x0F, "nidoqueen": 0x10,
	"cubone": 0x11, "rhyhorn": 0x12, "lapras": 0x13, "arcanine": 0x14,
	"mew": 0x15, "gyarados": 0x16, "shellder": 0x17, "tentacool": 0x18,
	"gastly": 0x19, "scyther": 0x1A, "staryu": 0x1B, "blastoise": 0x1C,
	"pinsir": 0x1D, "tangela": 0x1E, "growlithe": 0x21, "onix": 0x22,
	"fearow": 0x23, "pidgey": 0x24, "slowpoke": 0x25, "kadabra": 0x26,
	"graveler": 0x27, "chansey": 0x28, "machoke": 0x29, "mr.mime": 0x2A,
	"hitmonlee": 0x2B, "hitmonchan": 0x2C, "arbok": 0x2D, "parasect": 0x2E,
	"psyduck": 0x2F, "drowzee": 0x30, "golem": 0x31, "magmar": 0x33,
	"electabuzz": 0x35, "magneton": 0x36, "koffing": 0x37, "mankey": 0x39,
	"seel": 0x3A, "diglett": 0x3B, "tauros": 0x3C, "farfetch'd": 0x40,
	"venonat": 0x41, "dragonite": 0x42, "doduo": 0x46, "poliwag": 0x47,
	"jynx": 0x48, "moltres": 0x49, "articuno": 0x4A, "zapdos": 0x4B,
	"ditto": 0x4C, "meowth": 0x4D, "krabby": 0x4E, "vulpix": 0x52,
	"ninetales": 0x53, "pikachu": 0x54, "raichu": 0x55, "dratini": 0x58,
	"dragonair": 0x59, "kabuto": 0x5A, "kabutops": 0x5B, "horsea": 0x5C,
	"seadra": 0x5D, "sandshrew": 0x60, "sandslash": 0x61, "omanyte": 0x62,
	"omastar": 0x63, "jigglypuff": 0x64, "wigglytuff": 0x65, "eevee": 0x66,
	"flareon": 0x67, "jolteon": 0x68, "vaporeon": 0x69, "machop": 0x6A,
	"zubat": 0x6B, "ekans": 0x6C, "paras": 0x6D, "poliwhirl": 0x6E,
	"poliwrath": 0x6F, "weedle": 0x70, "kakuna": 0x71, "beedrill": 0x72,
	"dodrio": 0x74, "primeape": 0x75, "dugtrio": 0x76, "venomoth": 0x77,
	"dewgong": 0x78, "caterpie": 0x7B, "metapod": 0x7C, "butterfree": 0x7D,
	"machamp": 0x7E, "golduck": 0x80, "hypno": 0x81, "golbat": 0x82,
	"mewtwo": 0x83, "snorlax": 0x84, "magikarp": 0x85, "muk": 0x88,
	"kingler": 0x8A, "cloyster": 0x8B, "electrode": 0x8D, "clefable": 0x8E,
	"weezing": 0x8F, "persian": 0x90, "marowak": 0x91, "haunter": 0x93,
	"abra": 0x94, "alakazam": 0x95, "pidgeotto": 0x96, "pidgeot": 0x97,
	"starmie": 0x98, "bulbasaur": 0x99, "venusaur": 0x9A, "tentacruel": 0x9B,
	"goldeen": 0x9D, "seaking": 0x9E, "ponyta": 0xA3, "rapidash": 0xA4,
	"rattata": 0xA5, "raticate": 0xA6, "nidorino": 0xA7, "nidorina": 0xA8,
	"geodude": 0xA9, "porygon": 0xAA, "aerodactyl": 0xAB, "magnemite": 0xAD,
	"charmander": 0xB0, "squirtle": 0xB1, "charmeleon": 0xB2, "wartortle": 0xB3,
	"charizard": 0xB4, "oddish": 0xB9, "gloom": 0xBA, "vileplume": 0xBB,
	"bellsprout": 0xBC, "weepinbell": 0xBD, "victreebel": 0xBE,
}

var itemTable = map[string]uint8{
	"pokeball": 0x04, "great ball": 0x03,
	"potion": 0x14, "super potion": 0x13, "hyper potion": 0x12, "max potion": 0x11,
	"antidote": 0x0B, "burn heal": 0x0C, "ice heal": 0x0D,
	"awakening": 0x0E, "parlyz heal": 0x0F, "full restore": 0x10,
	"repel": 0x1E, "escape rope": 0x1D,
	"silph scope": 0x48, "poke flute": 0x49,
	"hm03": 0xC6, "hm04": 0xC7,
}

var speciesByID = reverse(speciesTable)
var itemByID = reverse(itemTable)

func reverse(in map[string]uint8) map[uint8]string {
	out := make(map[uint8]string, len(in))
	for name, id := range in {
		out[id] = name
	}
	return out
}

func SpeciesCount() int { return len(speciesTable) }

func SpeciesName(raw uint8) (string, bool) {
	name, ok := speciesByID[raw]
	return name, ok
}

func Species(raw uint8) (game.SpeciesID, bool) {
	name, ok := SpeciesName(raw)
	if !ok {
		return "", false
	}
	return game.SpeciesID(name), true
}

func SpeciesRaw(id game.SpeciesID) (uint8, bool) {
	raw, ok := speciesTable[game.CanonicalID(string(id))]
	return raw, ok
}

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
