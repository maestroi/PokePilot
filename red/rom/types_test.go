package rom_test

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

// Type ids, constants/type_constants.asm.
const (
	typeNormal   uint8 = 0x00
	typeFlying   uint8 = 0x02
	typeGround   uint8 = 0x04
	typeRock     uint8 = 0x05
	typeFire     uint8 = 0x14
	typeWater    uint8 = 0x15
	typeGrass    uint8 = 0x16
	typeElectric uint8 = 0x17
)

// TestTypeEffectiveness pins the chart against data/types/type_matchups.asm,
// including the case the whole change exists for: Gen 1 applies the
// multiplier for BOTH of the defender's types, so a Rock/Ground opponent
// takes quadruple from Water and a Rock/Flying one is immune to Ground.
func TestTypeEffectiveness(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name             string
		move, def1, def2 uint8
		want             int
	}{
		// Single type, straight from the table.
		{"water into fire", typeWater, typeFire, typeFire, 20},
		{"fire into grass", typeFire, typeGrass, typeGrass, 20},
		{"normal into rock", typeNormal, typeRock, typeRock, 5},
		{"ground into flying", typeGround, typeFlying, typeFlying, 0},
		{"water into water", typeWater, typeWater, typeWater, 5},
		// Silence in the chart means ordinary damage.
		{"normal into water", typeNormal, typeWater, typeWater, rom.NeutralEffect},
		{"normal into normal", typeNormal, typeNormal, typeNormal, rom.NeutralEffect},
		// Both types apply. ONIX is ROCK/GROUND: water is super effective
		// against each, so it lands for four times damage. This is the case
		// a power-only policy gets wrong at Brock's gym.
		{"water into rock/ground", typeWater, typeRock, typeGround, 40},
		{"grass into rock/ground", typeGrass, typeRock, typeGround, 40},
		// An immunity survives the second multiplication: ground does
		// nothing to anything that flies, whatever its other half is.
		{"ground into rock/flying", typeGround, typeRock, typeFlying, 0},
		// One half up, the other half down, cancelling exactly.
		{"electric into water/flying", typeElectric, typeWater, typeFlying, 40},
	}
	for _, tc := range cases {
		got, err := rom.TypeEffectiveness(data, tc.move, tc.def1, tc.def2)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: effectiveness = %d tenths, want %d", tc.name, got, tc.want)
		}
	}
}
