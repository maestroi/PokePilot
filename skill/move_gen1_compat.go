package skill

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/sym"
)

// playerXY is the legacy Gen-I position helper used by Red-owned story and
// progression controllers. Reusable movement/navigation reads position through
// game.OverworldDecoder instead.
func playerXY(m *emu.Emu) (uint8, uint8) {
	return m.Peek8(sym.XCoord), m.Peek8(sym.YCoord)
}
