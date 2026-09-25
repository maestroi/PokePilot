package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeBattleEscapeMenu(r game.MemoryReader) game.BattleEscapeMenuState {
	return p.engine.DecodeBattleEscapeMenu(r)
}

func (p *Profile) BattleEscapeRunPosition(kind game.BattleEscapeMenuKind) (game.BattleMenuPosition, bool) {
	return p.engine.BattleEscapeRunPosition(kind)
}
