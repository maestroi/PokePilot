package agent

import (
	"sync"

	"github.com/maestroi/pokepilot/red/rom"
)

// LearnableMove is a damaging move a species knows from level Level (1 for
// its starting moves).
type LearnableMove struct {
	Level int
	Type  string
	Power uint8
}

const gen1SpeciesIndexes = 190 // NUM_POKEMON_INDEXES

var gen1LearnsetCache sync.Map // gen1ChallengeKey -> map[SpeciesID][]LearnableMove

// gen1Learnsets reads every species' damaging start and level-up moves from
// the cartridge (BaseStats start moves + EvosMoves), so counter selection
// knows what a catch will be able to do without a hand-written learnset.
func gen1Learnsets(romData []byte) map[SpeciesID][]LearnableMove {
	if len(romData) < 0x150 {
		return nil
	}
	key := gen1ChallengeKey{n: len(romData)}
	copy(key.header[:], romData[0x134:0x150])
	if v, ok := gen1LearnsetCache.Load(key); ok {
		return v.(map[SpeciesID][]LearnableMove)
	}
	out := map[SpeciesID][]LearnableMove{}
	add := func(sp SpeciesID, level int, move uint8) {
		if move == 0 {
			return
		}
		mv, err := rom.LookupMove(romData, move)
		if err != nil || mv.Power == 0 {
			return
		}
		out[sp] = append(out[sp], LearnableMove{Level: level, Type: redTypeName(mv.Type), Power: mv.Power})
	}
	for id := 1; id <= gen1SpeciesIndexes; id++ {
		name, ok := SpeciesName(uint8(id))
		if !ok {
			continue
		}
		sp := SpeciesID(name)
		if stats, err := rom.LookupSpeciesBaseStats(romData, uint8(id)); err == nil {
			for _, move := range stats.Moves {
				add(sp, 1, move)
			}
		}
		if moves, err := rom.LevelUpMoves(romData, uint8(id)); err == nil {
			for _, m := range moves {
				add(sp, int(m.Level), m.Move)
			}
		}
	}
	gen1LearnsetCache.Store(key, out)
	return out
}
