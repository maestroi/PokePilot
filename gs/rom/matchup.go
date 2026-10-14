package rom

import (
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/game"
)

// Gen-II native type indices from pokegold/constants/type_constants.asm.
// Only the vocabulary lives here; every effectiveness value comes from the
// cartridge's TypeMatchups table (never a handwritten Gen-I chart).
var gen2TypeIDs = map[game.TypeID]uint8{
	"normal": 0, "fighting": 1, "flying": 2, "poison": 3,
	"ground": 4, "rock": 5, "bug": 7, "ghost": 8, "steel": 9,
	"fire": 20, "water": 21, "grass": 22, "electric": 23,
	"psychic": 24, "ice": 25, "dragon": 26, "dark": 27,
}

var gen2TypeNames = func() map[uint8]game.TypeID {
	names := make(map[uint8]game.TypeID, len(gen2TypeIDs))
	for name, raw := range gen2TypeIDs {
		names[raw] = name
	}
	return names
}()

// TypeChart adapts the Gold/Silver ROM's TypeMatchups to the portable
// game.TypeChart contract used by challenge readiness.
type TypeChart struct {
	pairs     map[[2]uint8]int
	attackers []game.TypeID
}

var _ game.TypeChart = (*TypeChart)(nil)

// NewTypeChart validates the actual cartridge chart and derives the attacking
// types from its rows. Unknown types/unsupported ROMs fail closed.
func NewTypeChart(rom []byte) (*TypeChart, error) {
	pairs, err := typeMatchups(rom)
	if err != nil {
		return nil, err
	}
	c := &TypeChart{pairs: pairs}
	seen := make(map[game.TypeID]bool)
	for native := range pairs {
		attack, ok := gen2TypeNames[native[0]]
		if !ok {
			return nil, fmt.Errorf("gs/rom: unknown attacking type %#02x in TypeMatchups", native[0])
		}
		if _, ok := gen2TypeNames[native[1]]; !ok {
			return nil, fmt.Errorf("gs/rom: unknown defending type %#02x in TypeMatchups", native[1])
		}
		if !seen[attack] {
			seen[attack] = true
			c.attackers = append(c.attackers, attack)
		}
	}
	sort.Slice(c.attackers, func(i, j int) bool { return c.attackers[i] < c.attackers[j] })
	return c, nil
}

func (c *TypeChart) AttackTypes() []game.TypeID {
	if c == nil {
		return nil
	}
	return append([]game.TypeID(nil), c.attackers...)
}

// Effectiveness applies each unique defender type. An unknown type is
// neutral rather than mapped to a wrong generation's type index.
func (c *TypeChart) Effectiveness(attack game.TypeID, defend []game.TypeID) int {
	if c == nil {
		return game.NeutralEffect
	}
	move, ok := gen2TypeIDs[attack]
	if !ok {
		return game.NeutralEffect
	}
	result := game.NeutralEffect
	seen := make(map[uint8]bool, len(defend))
	for _, typ := range defend {
		id, ok := gen2TypeIDs[typ]
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		result = result * pairEffect(c.pairs, move, id) / game.NeutralEffect
	}
	return result
}

// SpeciesTypesAt reads the two type bytes from one Gen-II BaseData entry.
// The supplied base must come from LocateBaseData. Distinct dual types remain
// distinct; an identical second type must not double its effectiveness.
func SpeciesTypesAt(rom []byte, base int, species uint8) ([]game.TypeID, error) {
	if species == 0 || int(species) > gen2PokemonCount {
		return nil, fmt.Errorf("gs/rom: invalid species %#02x", species)
	}
	off := base + (int(species)-1)*gen2BaseDataEntrySize + 7
	if base < 0 || base+gen2PokemonCount*gen2BaseDataEntrySize > len(rom) {
		return nil, fmt.Errorf("gs/rom: BaseData table outside ROM")
	}
	first, ok1 := gen2TypeNames[rom[off]]
	second, ok2 := gen2TypeNames[rom[off+1]]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("gs/rom: species %#02x has unknown types %#02x/%#02x", species, rom[off], rom[off+1])
	}
	if first == second {
		return []game.TypeID{first}, nil
	}
	return []game.TypeID{first, second}, nil
}

func SpeciesTypes(rom []byte, species uint8) ([]game.TypeID, error) {
	base, err := LocateBaseData(rom)
	if err != nil {
		return nil, err
	}
	return SpeciesTypesAt(rom, base, species)
}
