package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

var _ game.BattleStateDecoder = (*Profile)(nil)

const gen2PPMask byte = 0x3f

func battleBE16(reader game.MemoryReader, addr uint16) uint16 {
	return uint16(reader.Peek8(addr))<<8 | uint16(reader.Peek8(addr+1))
}

func battleKind(raw byte) game.BattleKind {
	switch raw {
	case 1:
		return game.BattleWild
	case 2:
		return game.BattleTrainer
	default:
		return game.BattleNone
	}
}

func decodeBattleMon(reader game.MemoryReader, base uint16) (species, level uint8, hp, maxHP, attack, defense, speed, spAtk, spDef uint16, type1, type2 uint8, moves [4]game.BattleMove) {
	species = reader.Peek8(base)
	level = reader.Peek8(base + 0x0d)
	hp = battleBE16(reader, base+0x10)
	maxHP = battleBE16(reader, base+0x12)
	attack = battleBE16(reader, base+0x14)
	defense = battleBE16(reader, base+0x16)
	speed = battleBE16(reader, base+0x18)
	spAtk = battleBE16(reader, base+0x1a)
	spDef = battleBE16(reader, base+0x1c)
	type1 = reader.Peek8(base + 0x1e)
	type2 = reader.Peek8(base + 0x1f)
	for i := range moves {
		moves[i] = game.BattleMove{
			ID: reader.Peek8(base + 0x02 + uint16(i)),
			PP: reader.Peek8(base+0x08+uint16(i)) & gen2PPMask,
		}
	}
	return
}

// gsDisabledMoveSlot returns the zero-based player move slot currently
// Disabled by Disable, or -1 when none is. wPlayerDisableCount's high nibble
// stores the 1-based index (engine/battle/core.asm MoveInfoBox /
// MoveSelectionScreen).
func gsDisabledMoveSlot(reader game.MemoryReader) int {
	if reader == nil {
		return -1
	}
	slot := int(reader.Peek8(sym.PlayerDisableCount) >> 4)
	if slot < 1 || slot > 4 {
		return -1
	}
	return slot - 1
}

// DecodeBattleState projects Gold/Silver's live battle_struct values onto the
// portable combat state. The struct layout and stat-level bases are pinned to
// the supported retail Gold/Silver rev0 builds through gs/sym.
func (*Profile) DecodeBattleState(reader game.MemoryReader) (game.BattleState, bool) {
	if reader == nil {
		return game.BattleState{}, false
	}
	mode := reader.Peek8(sym.BattleMode)
	kind := battleKind(mode)
	if kind == game.BattleNone {
		return game.BattleState{}, false
	}

	activeSpecies, activeLevel, activeHP, activeMaxHP, activeAttack, activeDefense, activeSpeed, activeSpAtk, activeSpDef, activeType1, activeType2, moves :=
		decodeBattleMon(reader, sym.BattleMon)
	enemySpecies, enemyLevel, enemyHP, enemyMaxHP, enemyAttack, enemyDefense, enemySpeed, enemySpAtk, enemySpDef, enemyType1, enemyType2, _ :=
		decodeBattleMon(reader, sym.EnemyMon)
	if disabled := gsDisabledMoveSlot(reader); disabled >= 0 {
		moves[disabled].Disabled = true
	}

	return game.BattleState{
		Kind:                 kind,
		EnemySpecies:         enemySpecies,
		EnemyHP:              enemyHP,
		EnemyMaxHP:           enemyMaxHP,
		EnemyLevel:           enemyLevel,
		ActiveSpecies:        activeSpecies,
		ActiveHP:             activeHP,
		ActiveLevel:          activeLevel,
		ActiveMaxHP:          activeMaxHP,
		Moves:                moves,
		ActiveAttack:         activeAttack,
		ActiveDefense:        activeDefense,
		ActiveSpecial:        activeSpAtk,
		EnemyAttack:          enemyAttack,
		EnemyDefense:         enemyDefense,
		EnemySpecial:         enemySpAtk,
		ActiveSpecialAttack:  activeSpAtk,
		ActiveSpecialDefense: activeSpDef,
		EnemySpecialAttack:   enemySpAtk,
		EnemySpecialDefense:  enemySpDef,
		ActiveSpeed:          activeSpeed,
		EnemySpeed:           enemySpeed,
		ActiveAttackMod:      reader.Peek8(sym.PlayerStatLevels),
		ActiveDefenseMod:     reader.Peek8(sym.PlayerStatLevels + 1),
		EnemyAttackMod:       reader.Peek8(sym.EnemyStatLevels),
		EnemyDefenseMod:      reader.Peek8(sym.EnemyStatLevels + 1),
		EnemyType1:           enemyType1,
		EnemyType2:           enemyType2,
		ActiveType1:          activeType1,
		ActiveType2:          activeType2,
		ActiveReflect:        reader.Peek8(sym.PlayerScreens)&(1<<sym.PlayerScreensReflectBit) != 0,
	}, true
}

// DecodeBattleResult maps Gold/Silver's WIN/LOSE/DRAW byte while ignoring bit 7,
// which the cartridge independently uses to report a full capture box.
func (*Profile) DecodeBattleResult(reader game.MemoryReader) game.BattleResult {
	if reader == nil {
		return game.BattleDraw
	}
	switch reader.Peek8(sym.BattleResult) &^ 0x80 {
	case 0:
		return game.BattleWon
	case 1:
		return game.BattleLost
	default:
		return game.BattleDraw
	}
}
