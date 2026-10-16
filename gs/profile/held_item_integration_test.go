package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

// fixtureGoldHeldItemROM contains just the two cartridge tables used by combat:
// a neutral TypeMatchups chart and ItemAttributes. This exercises the public
// profile scorer and the ROM attribute lookup without requiring a private ROM.
func fixtureGoldHeldItemROM() []byte {
	rom := make([]byte, 0x800)

	// TypeMatchups' real leading fingerprint, followed by enough valid rows
	// for the cartridge table locator and its terminating marker.
	const chart = 0x200
	copy(rom[chart:], []byte{0, 5, 5, 0, 9, 5})
	for i := 2; i < 66; i++ {
		copy(rom[chart+i*3:], []byte{1, 1, 5})
	}
	rom[chart+66*3] = 0xff

	// ItemAttributes' identifying prices: MASTER_BALL = 0,
	// ULTRA_BALL = 1200, POKE_BALL = 200.
	const items = 0x400
	const record = 7
	rom[items+record] = 0xb0
	rom[items+record+1] = 0x04
	rom[items+4*record] = 200
	setHeldEffect := func(id, effect, param uint8) {
		offset := items + (int(id)-1)*record
		rom[offset+2] = effect
		rom[offset+3] = param
	}
	setHeldEffect(0x75, gsHeldEffectBoostFirst+10, 10) // MIRACLE_SEED, Grass +10%
	setHeldEffect(0x92, gsHeldEffectLeftovers, 0)      // LEFTOVERS
	setHeldEffect(0x90, 77, 99)                        // Unknown effect: neutral
	return rom
}

func TestGoldHeldItemModifiersAffectCombatEvaluationWithoutROMFile(t *testing.T) {
	rom := fixtureGoldHeldItemROM()
	profile := NewGold()
	attacker := game.BattleCombatant{
		Type1: uint16(gsTypeGrass), Type2: uint16(gsTypeGrass),
		HP: 50, MaxHP: 100,
	}
	defender := game.BattleCombatant{
		Type1: uint16(gsTypeNormal), Type2: uint16(gsTypeNormal),
	}
	evaluate := func(item uint16, move uint16) game.BattleMoveEvaluation {
		t.Helper()
		attacker.HeldItem = item
		eval, role, err := profile.EvaluateCombatMove(rom, attacker, defender, move, 20)
		if err != nil {
			t.Fatal(err)
		}
		if role != game.BattleMoveRoleDirectDamage {
			t.Fatalf("move %d with item %#x role = %v", move, item, role)
		}
		return eval
	}

	const razorLeaf uint16 = 75
	baseline := evaluate(0, razorLeaf)
	if baseline.ExpectedScore <= 0 || baseline.ItemModifier != (game.BattleItemModifier{}) {
		t.Fatalf("no-item grass move must have an unmodified positive score: %+v", baseline)
	}

	boosted := evaluate(0x75, razorLeaf)
	wantBoosted := baseline.ExpectedScore + baseline.ExpectedScore/10
	if boosted.ExpectedScore != wantBoosted ||
		boosted.ItemModifier.DamageBonusPercent != 10 ||
		boosted.ItemModifier.RecoveryPerTurn != 0 {
		t.Fatalf("grass-boosting item: got %+v, want score %d and 10%% boost", boosted, wantBoosted)
	}

	// A type boost may not raise a move of another type.
	offType := evaluate(0x75, 33) // TACKLE, Normal
	noItemNormal := evaluate(0, 33)
	if offType.ExpectedScore != noItemNormal.ExpectedScore ||
		offType.ItemModifier != (game.BattleItemModifier{}) {
		t.Fatalf("off-type item must be neutral: item=%+v no-item=%+v", offType, noItemNormal)
	}

	leftovers := evaluate(0x92, razorLeaf)
	if leftovers.ExpectedScore != baseline.ExpectedScore ||
		leftovers.ItemModifier.RecoveryPerTurn != 6 ||
		leftovers.ItemModifier.DamageBonusPercent != 0 {
		t.Fatalf("Leftovers must report recovery without altering damage: %+v", leftovers)
	}

	for _, item := range []uint16{0x90, 0x100, 0xff} {
		unknown := evaluate(item, razorLeaf)
		if unknown.ExpectedScore != baseline.ExpectedScore ||
			unknown.ItemModifier != (game.BattleItemModifier{}) {
			t.Fatalf("unknown/invalid item %#x must fail closed: %+v", item, unknown)
		}
	}
}
