package gen1

import "strings"

// PostSurgeCeladonArea reports the shared Kanto area used by the staged
// Thunder -> Lavender -> Celadon progression. Map IDs in this corridor are
// stable across the supported Gen-I Kanto layouts; interiors are recognized
// by semantic map name so version-specific script bytes stay out of gen1.
func PostSurgeCeladonArea(mapID uint8, mapName string) bool {
	_ = mapID
	return strings.HasPrefix(mapName, "CELADON_") ||
		mapName == "GAME_CORNER" ||
		strings.HasPrefix(mapName, "GAME_CORNER_") ||
		strings.HasPrefix(mapName, "ROCKET_HIDEOUT_")
}

// PostSurgeLavenderReached is a geographic checkpoint, not a synthetic event.
// It stays true while proceeding west through the shared Lavender/Saffron/
// Celadon corridor so resumed runs never backtrack merely to replay Lavender.
func PostSurgeLavenderReached(mapID uint8, mapName string) bool {
	switch mapID {
	case 0x04, // Lavender Town
		0x13, // Route 8
		0x4f, // Route 8 gate
		0x50, // Underground Path Route 8
		0x79, // Underground Path west-east
		0x4d, // Underground Path Route 7
		0x4e, // Underground Path Route 7 copy
		0x4c, // Route 7 gate
		0x12, // Route 7
		0x0a: // Saffron City
		return true
	}
	return strings.HasPrefix(mapName, "LAVENDER_") ||
		strings.HasPrefix(mapName, "POKEMON_TOWER_") ||
		mapName == "MR_FUJIS_HOUSE" ||
		strings.HasPrefix(mapName, "SAFFRON_") ||
		strings.HasPrefix(mapName, "SILPH_CO_") ||
		PostSurgeCeladonArea(mapID, mapName)
}
