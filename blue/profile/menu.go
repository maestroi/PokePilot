package profile

import (
	"github.com/maestroi/pokepilot/game"
)

func (p *Profile) DecodeMenuCursor(r game.MemoryReader) game.MenuCursorState {
	return p.engine.DecodeMenuCursor(r)
}

func (p *Profile) DecodeTwoOption(r game.MemoryReader) (game.TwoOptionState, bool) {
	return p.engine.DecodeTwoOption(r)
}

func (p *Profile) DecodeStartMenu(r game.MemoryReader) game.StartMenuState {
	return p.engine.DecodeStartMenu(r)
}
