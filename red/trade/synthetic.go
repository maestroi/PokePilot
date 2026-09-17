// Package trade adapts Red/Blue ROM species data to the generation-specific
// Cable Club structures in gen1/trade.
package trade

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/game"
	gen1trade "github.com/maestroi/pokepilot/gen1/trade"
	"github.com/maestroi/pokepilot/red/data"
	"github.com/maestroi/pokepilot/red/rom"
)

const defaultDV = 9

// SyntheticMon constructs a deterministic, legal-looking party Pokemon using
// base stats, types, starting moves, PP and growth policy from the loaded ROM.
// It does not mutate game memory; the resulting bytes only exist on the remote
// side of a normal Cable Club transfer.
func SyntheticMon(romData []byte, species game.SpeciesID, level uint8, trainerID uint16) (gen1trade.Mon, error) {
	var mon gen1trade.Mon
	if level < 1 || level > 100 {
		return mon, fmt.Errorf("red trade: level %d outside 1..100", level)
	}
	rawSpecies, ok := data.SpeciesRaw(species)
	if !ok {
		return mon, fmt.Errorf("red trade: unknown species %q", species)
	}
	base, err := rom.LookupSpeciesBaseStats(romData, rawSpecies)
	if err != nil {
		return mon, err
	}
	xp, err := rom.ExperienceAtLevel(base.Growth, int(level))
	if err != nil {
		return mon, err
	}

	raw := &mon.Raw
	raw[0] = rawSpecies
	raw[3] = level // box level
	raw[4] = 0     // healthy
	raw[5] = base.Type1
	raw[6] = base.Type2
	raw[7] = base.CatchRate
	copy(raw[8:12], base.Moves[:])
	binary.BigEndian.PutUint16(raw[12:14], trainerID)
	raw[14] = byte(xp >> 16)
	raw[15] = byte(xp >> 8)
	raw[16] = byte(xp)
	// Stat experience at 17..26 is deliberately zero. Fixed 9/9/9/9 DVs make
	// generation deterministic; because all four low DV bits are one, HP DV=15.
	raw[27] = defaultDV<<4 | defaultDV
	raw[28] = defaultDV<<4 | defaultDV
	for i, move := range base.Moves {
		if move == 0 {
			continue
		}
		moveData, err := rom.LookupMove(romData, move)
		if err != nil {
			return mon, err
		}
		raw[29+i] = moveData.PP
	}
	raw[33] = level

	hp := hpStat(base.HP, 15, level)
	binary.BigEndian.PutUint16(raw[1:3], hp)
	binary.BigEndian.PutUint16(raw[34:36], hp)
	binary.BigEndian.PutUint16(raw[36:38], normalStat(base.Attack, defaultDV, level))
	binary.BigEndian.PutUint16(raw[38:40], normalStat(base.Defense, defaultDV, level))
	binary.BigEndian.PutUint16(raw[40:42], normalStat(base.Speed, defaultDV, level))
	binary.BigEndian.PutUint16(raw[42:44], normalStat(base.Special, defaultDV, level))

	mon.OT = gen1trade.EncodeText("POKEPILOT")
	mon.Nick = gen1trade.EncodeText(strings.ToUpper(string(species)))
	return mon, nil
}

func hpStat(base uint8, dv int, level uint8) uint16 {
	return uint16(((int(base)+dv)*2*int(level))/100 + int(level) + 10)
}

func normalStat(base uint8, dv int, level uint8) uint16 {
	return uint16(((int(base)+dv)*2*int(level))/100 + 5)
}
