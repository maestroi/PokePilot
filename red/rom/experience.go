package rom

import "fmt"

const (
	baseStatsBaseExpOffset    = 9
	baseStatsGrowthRateOffset = 19
)

// GrowthRate is Red's BASE_GROWTH_RATE value. The numeric order is the
// GrowthRateTable order in pokered/data/growth_rates.asm.
type GrowthRate uint8

const (
	GrowthMediumFast GrowthRate = iota
	GrowthSlightlyFast
	GrowthSlightlySlow
	GrowthMediumSlow
	GrowthFast
	GrowthSlow
	growthRateCount
)

// SpeciesExperienceData is the experience policy stored in one species'
// BaseStats entry. BaseYield feeds battle experience; Growth determines the
// cumulative experience threshold for each level.
type SpeciesExperienceData struct {
	BaseYield uint8
	Growth    GrowthRate
}

// LookupSpeciesExperience reads experience policy from the loaded ROM rather
// than a hand-maintained species chart. Internal Red species IDs are translated
// through PokedexOrder before indexing BaseStats, matching GetMonHeader.
func LookupSpeciesExperience(romData []byte, species uint8) (SpeciesExperienceData, error) {
	dex, err := InternalSpeciesDexNumber(romData, species)
	if err != nil {
		return SpeciesExperienceData{}, err
	}
	entry := baseStatsOffset + (int(dex)-1)*baseStatsEntryLen
	if entry < 0 || entry+baseStatsEntryLen > len(romData) {
		return SpeciesExperienceData{}, fmt.Errorf("rom: base stats for dex %d at offset %#x exceed ROM of %d bytes", dex, entry, len(romData))
	}
	growth := GrowthRate(romData[entry+baseStatsGrowthRateOffset])
	if growth >= growthRateCount {
		return SpeciesExperienceData{}, fmt.Errorf("rom: dex %d has invalid growth rate %d", dex, growth)
	}
	return SpeciesExperienceData{
		BaseYield: romData[entry+baseStatsBaseExpOffset],
		Growth:    growth,
	}, nil
}

// ExperienceAtLevel implements Red's GrowthRateTable equations. The formulas
// are cumulative experience thresholds; integer division mirrors the game's
// fixed-width arithmetic for the cubic coefficient. Medium Slow is negative at
// level 1 mathematically, where Red never needs a negative cumulative value, so
// the public semantic result is clamped to zero.
func ExperienceAtLevel(rate GrowthRate, level int) (uint32, error) {
	if rate >= growthRateCount {
		return 0, fmt.Errorf("rom: invalid growth rate %d", rate)
	}
	if level < 1 || level > 100 {
		return 0, fmt.Errorf("rom: level %d outside 1..100", level)
	}
	n := int64(level)
	n2 := n * n
	n3 := n2 * n
	var xp int64
	switch rate {
	case GrowthMediumFast:
		xp = n3
	case GrowthSlightlyFast:
		xp = 3*n3/4 + 10*n2 - 30
	case GrowthSlightlySlow:
		xp = 3*n3/4 + 20*n2 - 70
	case GrowthMediumSlow:
		xp = 6*n3/5 - 15*n2 + 100*n - 140
	case GrowthFast:
		xp = 4 * n3 / 5
	case GrowthSlow:
		xp = 5 * n3 / 4
	}
	if xp < 0 {
		xp = 0
	}
	return uint32(xp), nil
}

// WildBattleExperience is the unboosted, single-participant Gen I battle XP
// formula used before traded-mon or trainer bonuses: floor(baseExp*level/7).
// Train normally sends the lead into wild battles, so this is the appropriate
// representative yield for deciding whether a local grind is viable.
func WildBattleExperience(baseYield, level uint8) uint32 {
	return uint32(baseYield) * uint32(level) / 7
}
