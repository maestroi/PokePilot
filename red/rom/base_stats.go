package rom

import "fmt"

const (
	baseStatsDexOffset       = 0
	baseStatsHPOffset        = 1
	baseStatsAttackOffset    = 2
	baseStatsDefenseOffset   = 3
	baseStatsSpeedOffset     = 4
	baseStatsSpecialOffset   = 5
	baseStatsType1Offset     = 6
	baseStatsType2Offset     = 7
	baseStatsCatchRateOffset = 8
	baseStatsStartMoves      = 15
)

// SpeciesBaseStats is the subset of Red/Blue BaseStats required to construct a
// structurally valid synthetic party Pokemon for a virtual Cable Club peer.
type SpeciesBaseStats struct {
	Dex       uint8
	HP        uint8
	Attack    uint8
	Defense   uint8
	Speed     uint8
	Special   uint8
	Type1     uint8
	Type2     uint8
	CatchRate uint8
	Moves     [4]uint8
	Growth    GrowthRate
}

// LookupSpeciesBaseStats reads the active ROM instead of a canonical species
// chart, so Red, Blue and compatible patched Gen-I ROMs generate link Pokemon
// from the cartridge they are actually paired with.
func LookupSpeciesBaseStats(romData []byte, species uint8) (SpeciesBaseStats, error) {
	dex, err := InternalSpeciesDexNumber(romData, species)
	if err != nil {
		return SpeciesBaseStats{}, err
	}
	entry, err := baseStatsEntry(romData, dex)
	if err != nil {
		return SpeciesBaseStats{}, err
	}
	if entry < 0 || entry+baseStatsEntryLen > len(romData) {
		return SpeciesBaseStats{}, fmt.Errorf("rom: base stats for dex %d at offset %#x exceed ROM of %d bytes", dex, entry, len(romData))
	}
	e := romData[entry : entry+baseStatsEntryLen]
	if e[baseStatsDexOffset] != dex {
		return SpeciesBaseStats{}, fmt.Errorf("rom: base stats entry for dex %d identifies dex %d", dex, e[baseStatsDexOffset])
	}
	growth := GrowthRate(e[baseStatsGrowthRateOffset])
	if growth >= growthRateCount {
		return SpeciesBaseStats{}, fmt.Errorf("rom: dex %d has invalid growth rate %d", dex, growth)
	}
	stats := SpeciesBaseStats{
		Dex:       dex,
		HP:        e[baseStatsHPOffset],
		Attack:    e[baseStatsAttackOffset],
		Defense:   e[baseStatsDefenseOffset],
		Speed:     e[baseStatsSpeedOffset],
		Special:   e[baseStatsSpecialOffset],
		Type1:     e[baseStatsType1Offset],
		Type2:     e[baseStatsType2Offset],
		CatchRate: e[baseStatsCatchRateOffset],
		Growth:    growth,
	}
	copy(stats.Moves[:], e[baseStatsStartMoves:baseStatsStartMoves+len(stats.Moves)])
	return stats, nil
}
