package skill

import (
	"github.com/maestroi/pokepilot/profiles"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func yellowRomTables() romTableSet {
	return romTableSet{
		tilesetsBank:       0x03,
		tilesetsAddr:       0x4558,
		wildBank:           0x03,
		wildAddr:           0x4B95,
		passableSpriteSlot: pikaFollowerSlot,
		wram:               yellowWram(),
	}
}

// isYellowROM reports whether romData is the supported Yellow image. It goes
// through the profile registry, the one place game identity is decided, so
// this package never compares hashes or game names itself.
func isYellowROM(romData []byte) bool {
	profile, _, err := profiles.Detect(romData)
	if err != nil {
		return false
	}
	return profile != nil && profile.ID() == yellowprofile.GameID
}
