package skill

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
	redprofile "github.com/maestroi/pokepilot/red/profile"
)

// StatAwareMove is the default deterministic fight policy. Real supported ROMs
// select their profile-owned generation strategy. The Red fallback exists only
// for historical synthetic Gen-I ROM fixtures whose bytes intentionally do not
// identify a registered cartridge.
func StatAwareMove(romData []byte) MovePolicy {
	profile, _, err := profiles.Detect(romData)
	if err == nil {
		if strategy, ok := profile.(game.BattleCombatStrategy); ok {
			return statAwareMoveWithStrategy(romData, strategy)
		}
		// A recognized future profile that has not supplied combat mechanics
		// must not accidentally inherit Red rules.
		return FirstUsableMove
	}
	return statAwareMoveWithStrategy(romData, redprofile.New())
}
