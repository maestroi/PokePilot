package rom

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
)

// Gender ratios from pokegold constants/pokemon_data_constants.asm.
// `percent` is `* $ff / 100`, and GENDER_UNKNOWN is -1.
const (
	baseGenderOffset = 13
	genderF0         = 0
	genderF100       = 254
	genderUnknown    = 255
)

// Gender applies GetGender (engine/pokemon/mon_stats.asm) to one species.
// atkSpd is the Attack DV in the high nibble and the Speed DV in the low
// nibble, which is the value the ROM compares with the species ratio.
func Gender(rom []byte, species uint8, atkSpd uint8) (game.Gender, error) {
	if species == 0 || int(species) > gen2PokemonCount {
		return "", fmt.Errorf("gs/rom: species %#02x is outside 1..%d", species, gen2PokemonCount)
	}
	base, err := LocateBaseData(rom)
	if err != nil {
		return "", err
	}
	off := base + (int(species)-1)*gen2BaseDataEntrySize + baseGenderOffset
	if err := mustInROM(rom, off, 1, "gender ratio"); err != nil {
		return "", err
	}
	ratio := rom[off]
	switch ratio {
	case genderUnknown:
		return game.Genderless, nil
	case genderF0:
		return game.GenderMale, nil
	case genderF100:
		return game.GenderFemale, nil
	}
	// cp ratio, DV; carry (ratio < DV) is male. Otherwise female.
	if ratio < atkSpd {
		return game.GenderMale, nil
	}
	return game.GenderFemale, nil
}
