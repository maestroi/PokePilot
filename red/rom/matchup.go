package rom

import (
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1rom"
	"github.com/maestroi/pokepilot/red/data"
)

const (
	// oppIDOffset is OPP_ID_OFFSET (constants/trainer_constants.asm): a map
	// object's trainer byte at or above it names a trainer class; below it,
	// the byte is a static encounter's species.
	oppIDOffset = 200
	// trainerClassCount is NUM_TRAINERS, shared by Red/Blue and Yellow.
	trainerClassCount = 0x2F
)

// TrainerParty decodes one trainer class/set party from the cartridge's own
// TrainerDataPointers table.
func TrainerParty(romData []byte, class, set uint8) ([]gen1rom.TrainerMon, error) {
	table := Tables(romData).TrainerDataPointers
	return gen1rom.TrainerParty(romData, class, set, gen1rom.TrainerLayout{
		PointerBank: table.Bank, PointerAddr: table.Addr, TrainerCount: trainerClassCount,
	})
}

// TypeChart is the cartridge's TypeEffects table as a portable game.TypeChart.
type TypeChart struct {
	rom       []byte
	ids       map[game.TypeID]uint8
	attackers []game.TypeID
}

// NewTypeChart reads the attacking types the cartridge's chart names.
func NewTypeChart(romData []byte) (*TypeChart, error) {
	base, err := Tables(romData).TypeEffects.Offset()
	if err != nil {
		return nil, fmt.Errorf("rom: TypeEffects: %w", err)
	}
	c := &TypeChart{rom: romData, ids: map[game.TypeID]uint8{}}
	for i := 0; i < typeEffectsMax; i++ {
		off := base + i*typeEffectEntry
		if off+typeEffectEntry > len(romData) {
			return nil, fmt.Errorf("rom: type chart entry %d exceeds ROM", i)
		}
		if romData[off] == typeEffectsEnd {
			sort.Slice(c.attackers, func(a, b int) bool { return c.attackers[a] < c.attackers[b] })
			return c, nil
		}
		for _, raw := range romData[off : off+2] {
			name, ok := data.TypeName(raw)
			if !ok {
				return nil, fmt.Errorf("rom: type chart names unknown type %#02x", raw)
			}
			c.ids[game.TypeID(name)] = raw
		}
		if attacker := game.TypeID(mustTypeName(romData[off])); !containsTypeID(c.attackers, attacker) {
			c.attackers = append(c.attackers, attacker)
		}
	}
	return nil, fmt.Errorf("rom: type chart has no terminator within %d entries", typeEffectsMax)
}

func (c *TypeChart) AttackTypes() []game.TypeID { return append([]game.TypeID(nil), c.attackers...) }

// Effectiveness applies TypeEffectiveness; a type the chart never names is
// neutral, exactly as the battle engine treats it.
func (c *TypeChart) Effectiveness(attack game.TypeID, defend []game.TypeID) int {
	move, ok := c.ids[attack]
	var ds []uint8
	for _, d := range defend {
		if id, known := c.ids[d]; known {
			ds = append(ds, id)
		}
	}
	if !ok || len(ds) == 0 {
		return NeutralEffect
	}
	d1, d2 := ds[0], ds[0]
	if len(ds) > 1 {
		d2 = ds[1]
	}
	e, err := TypeEffectiveness(c.rom, move, d1, d2)
	if err != nil {
		return NeutralEffect
	}
	return e
}

// SpeciesTypes returns a species' portable types from its base stats.
func SpeciesTypes(romData []byte, species uint8) ([]game.TypeID, error) {
	stats, err := LookupSpeciesBaseStats(romData, species)
	if err != nil {
		return nil, err
	}
	out := []game.TypeID{game.TypeID(mustTypeName(stats.Type1))}
	if stats.Type2 != stats.Type1 {
		out = append(out, game.TypeID(mustTypeName(stats.Type2)))
	}
	return out, nil
}

// MapChallengeOpponents returns the boss party of a challenge map: of every
// trainer object the map header places, the one with the highest-level
// Pokémon (ties: the larger party). Gym leaders, the League rooms and the
// Rocket bosses all outlevel their map's other trainers, so no names are
// needed. ok is false when the map holds no trainer objects.
func MapChallengeOpponents(romData []byte, mapID uint8) ([]game.ChallengeOpponent, bool, error) {
	h, err := ParseMap(romData, mapID)
	if err != nil {
		return nil, false, err
	}
	var best []gen1rom.TrainerMon
	bestLevel := -1
	for _, obj := range h.Objects {
		if obj.TrainerClass < oppIDOffset {
			continue
		}
		party, err := TrainerParty(romData, obj.TrainerClass-oppIDOffset, obj.TrainerSet)
		if err != nil {
			return nil, false, fmt.Errorf("rom: map %#02x trainer %d/%d: %w", mapID, obj.TrainerClass-oppIDOffset, obj.TrainerSet, err)
		}
		top := 0
		for _, mon := range party {
			if int(mon.Level) > top {
				top = int(mon.Level)
			}
		}
		if top > bestLevel || (top == bestLevel && len(party) > len(best)) {
			best, bestLevel = party, top
		}
	}
	if bestLevel < 0 {
		return nil, false, nil
	}
	out := make([]game.ChallengeOpponent, 0, len(best))
	for _, mon := range best {
		name, _ := data.SpeciesName(mon.Species)
		types, err := SpeciesTypes(romData, mon.Species)
		if err != nil {
			return nil, false, fmt.Errorf("rom: map %#02x opponent species %#02x: %w", mapID, mon.Species, err)
		}
		out = append(out, game.ChallengeOpponent{Species: game.SpeciesID(name), Level: int(mon.Level), Types: types})
	}
	return out, true, nil
}

func mustTypeName(raw uint8) string {
	name, _ := data.TypeName(raw)
	return name
}

func containsTypeID(list []game.TypeID, t game.TypeID) bool {
	for _, v := range list {
		if v == t {
			return true
		}
	}
	return false
}
