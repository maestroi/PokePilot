package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeBattleMainMenu(r game.MemoryReader) game.BattleMainMenuState {
	return p.engine.DecodeBattleMainMenu(r)
}

func (p *Profile) BattleMainMenuEntryPosition(entry game.BattleMenuEntry) (game.BattleMenuPosition, bool) {
	return p.engine.BattleMainMenuEntryPosition(entry)
}
