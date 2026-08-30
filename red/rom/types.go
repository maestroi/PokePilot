package rom

import "fmt"

// The type chart lives in the ROM, so it is read rather than typed. Gen 1's
// chart has eighty-two entries and a hand-copied one would be wrong in some
// corner nobody tested for a long time — the same argument that replaced the
// hand-written species names.
//
// TypeEffects (pokered.sym, bank 0f:6474) is a flat list of three-byte
// entries — attacking type, defending type, multiplier — terminated by 0xFF.
// engine/battle/core.asm:5129 walks it with the move's type in b and the
// defender's two types in d and e.
const (
	typeEffectsOffset = 0x0F*0x4000 + (0x6474 - 0x4000)
	typeEffectEntry   = 3
	typeEffectsEnd    = 0xFF
	// typeEffectsMax bounds the scan so a mislocated table cannot run the
	// length of the ROM. The real table is 82 entries.
	typeEffectsMax = 256
)

// NeutralEffect is the multiplier for a pairing the chart does not mention:
// ordinary damage. Multipliers are tenths, the way the ROM stores them
// (constants/battle_constants.asm: SUPER_EFFECTIVE = 20, so 20 means x2).
const NeutralEffect = 10

// TypeEffectiveness returns the damage multiplier, in tenths, for a move of
// type moveType against a defender of def1/def2.
//
// Both of the defender's types are applied, which is what makes Gen 1's
// chart so lopsided: the ROM's loop does not stop at the first match
// (core.asm:5142 pushes and continues), so WATER into a ROCK/GROUND ONIX is
// 20 x 20 / 10 = 40 — quadruple damage — and a GROUND move into a
// ROCK/FLYING Golbat is 0 whatever the other half says. A single-type mon
// stores the same type in both bytes, and applying it twice would square the
// multiplier, so an identical pair is applied once.
func TypeEffectiveness(romData []byte, moveType, def1, def2 uint8) (int, error) {
	m1, err := typePairEffect(romData, moveType, def1)
	if err != nil {
		return 0, err
	}
	if def2 == def1 {
		return m1, nil
	}
	m2, err := typePairEffect(romData, moveType, def2)
	if err != nil {
		return 0, err
	}
	return m1 * m2 / NeutralEffect, nil
}

// typePairEffect is one lookup: the multiplier in tenths for moveType
// against a single defending type, NeutralEffect when the chart is silent.
func typePairEffect(romData []byte, moveType, defType uint8) (int, error) {
	for i := 0; i < typeEffectsMax; i++ {
		off := typeEffectsOffset + i*typeEffectEntry
		if off+typeEffectEntry > len(romData) {
			return 0, fmt.Errorf("rom: type chart entry %d at offset %d exceeds ROM of %d bytes", i, off, len(romData))
		}
		if romData[off] == typeEffectsEnd {
			return NeutralEffect, nil
		}
		if romData[off] == moveType && romData[off+1] == defType {
			return int(romData[off+2]), nil
		}
	}
	return 0, fmt.Errorf("rom: type chart has no terminator within %d entries; the table offset is wrong", typeEffectsMax)
}
