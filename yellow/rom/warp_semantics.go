package rom

// IsInertWarp reports Yellow warp-table entries that are present in ROM data
// but are marked "; inaccessible" in pokeyellow and do not fire when stepped
// on. They are ordinary walkable floor, not transition ports.
func IsInertWarp(mapID, x, y uint8) bool {
	switch mapID {
	case 0x06: // CELADON_CITY
		return x == 39 && y == 19
	case 0xb5: // SILPH_CO_1F
		return x == 16 && y == 10
	case 0xeb: // SILPH_CO_11F
		return x == 5 && y == 5
	default:
		return false
	}
}
