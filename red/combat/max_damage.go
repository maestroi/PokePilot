package combat

import "github.com/maestroi/pokepilot/red/rom"

// Move effects whose damage has no per-turn bound: the move keeps attacking
// on later turns without a new choice, reflects or stores the foe's damage,
// or picks another move at random.
const (
	mirrorMoveEffect    uint8 = 0x09
	bideEffect          uint8 = 0x1A
	thrashEffect        uint8 = 0x1B
	twoToFiveEffect     uint8 = 0x1D
	trappingEffect      uint8 = 0x2A
	attackTwiceEffect   uint8 = 0x2C
	twineedleEffect     uint8 = 0x4D
	rageEffect          uint8 = 0x51
	metronomeEffect     uint8 = 0x53
	moveCounter         uint8 = 0x44
	critDamageLevelMult       = 2 // Red doubles the attacker's level on a critical hit.
)

// MaxDamage bounds the most one use of mv can deal: the top random roll, a
// critical hit (Red doubles the level and ignores stat stages, so the bound
// uses the current stats and can undershoot after the attacker was debuffed)
// and every hit of a multi-hit move. ok is false for a move with no safe
// bound. Status moves deal 0.
func MaxDamage(romData []byte, attacker, defender Combatant, mv rom.Move) (uint16, bool) {
	switch mv.Effect {
	case rom.OHKOEffect, rom.ExplodeEffect, mirrorMoveEffect, bideEffect, thrashEffect, trappingEffect, rageEffect, metronomeEffect:
		return 0, false
	case rom.SuperFangEffect:
		return max(1, defender.HP/2), true
	case rom.SpecialDamageEffect:
		if mv.ID == movePsywave {
			return max(1, uint16(attacker.Level)+uint16(attacker.Level)/2), true
		}
		damage, _ := specialDamageExpectation(attacker.Level, mv.ID)
		return damage, true
	}
	if mv.ID == moveCounter {
		return 0, false
	}
	if mv.Power == 0 {
		return 0, true
	}
	atkStat, defStat := attacker.Special, defender.Special
	if IsPhysicalType(mv.Type) {
		atkStat, defStat = attacker.Attack, defender.Defense
	}
	atk, def := damageStats(atkStat, defStat)
	level := uint16(attacker.Level) * critDamageLevelMult
	if level > 0xff {
		level = 0xff
	}
	hit := uint32(neutralDamage(uint8(level), mv.Power, atk, def))
	if mv.Type == attacker.Type1 || mv.Type == attacker.Type2 {
		hit = hit * stabTenths / rom.NeutralEffect
	}
	eff, err := rom.TypeEffectiveness(romData, mv.Type, defender.Type1, defender.Type2)
	if err != nil {
		return 0, false
	}
	hit = hit * uint32(eff) / rom.NeutralEffect
	switch mv.Effect {
	case twoToFiveEffect:
		hit *= 5
	case attackTwiceEffect, twineedleEffect:
		hit *= 2
	}
	return uint16(min(hit, 0xffff)), true
}

const (
	sleepEffect    uint8 = 0x20
	paralyzeEffect uint8 = 0x43
)

// CaptureStatus reports that mv inflicts sleep or paralysis on defender,
// the two statuses Red's capture formula rewards. Paralysis moves respect
// the type chart (Thunder Wave never lands on a Ground type).
func CaptureStatus(romData []byte, defender Combatant, mv rom.Move) bool {
	switch mv.Effect {
	case sleepEffect:
		return true
	case paralyzeEffect:
		eff, err := rom.TypeEffectiveness(romData, mv.Type, defender.Type1, defender.Type2)
		return err == nil && eff > 0
	}
	return false
}
