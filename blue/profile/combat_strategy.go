package profile

import "github.com/maestroi/pokepilot/game"

var _ game.BattleCombatStrategy = (*Profile)(nil)

func (p *Profile) EvaluateCombatMove(
	romData []byte,
	attacker, defender game.BattleCombatant,
	nativeMoveID uint16,
	currentPP uint8,
) (game.BattleMoveEvaluation, game.BattleMoveRole, error) {
	return p.engine.EvaluateCombatMove(romData, attacker, defender, nativeMoveID, currentPP)
}

func (p *Profile) IncomingTypeRisk(romData []byte, enemy, candidate game.BattleCombatant) int {
	return p.engine.IncomingTypeRisk(romData, enemy, candidate)
}

func (p *Profile) PreferSetupMove(
	b game.BattleState,
	setup, attack game.BattleMoveEvaluation,
) bool {
	return p.engine.PreferSetupMove(b, setup, attack)
}

func (p *Profile) IsFieldMove(nativeMoveID uint16) bool {
	return p.engine.IsFieldMove(nativeMoveID)
}
