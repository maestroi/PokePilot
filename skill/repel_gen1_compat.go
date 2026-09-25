package skill

const (
	ItemRepel      uint8 = 0x1e
	ItemSuperRepel uint8 = 0x38
	ItemMaxRepel   uint8 = 0x39
)

// RepelDuration preserves the public Gen-I helper while live execution gets
// item semantics from the active profile.
func RepelDuration(item uint8) (int, bool) {
	switch item {
	case ItemRepel:
		return 100, true
	case ItemSuperRepel:
		return 200, true
	case ItemMaxRepel:
		return 250, true
	default:
		return 0, false
	}
}
