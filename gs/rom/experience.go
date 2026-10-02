package rom

import "fmt"

const gen2BaseGrowthOffset = 22

// GrowthRate is BASE_GROWTH_RATE (pokegold data/growth_rates.asm).
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

// SpeciesExperienceData is the experience policy stored in BaseData.
type SpeciesExperienceData struct {
	BaseYield uint8
	Growth    GrowthRate
}

// LookupSpeciesExperience reads growth and base yield from BaseData.
// Gen 2 indexes BaseData by National Dex number.
func LookupSpeciesExperience(rom []byte, species uint8) (SpeciesExperienceData, error) {
	if species == 0 || int(species) > gen2PokemonCount {
		return SpeciesExperienceData{}, fmt.Errorf("gs/rom: species %#02x is outside 1..%d", species, gen2PokemonCount)
	}
	base, err := LocateBaseData(rom)
	if err != nil {
		return SpeciesExperienceData{}, err
	}
	off := base + int(species-1)*gen2BaseDataEntrySize
	if err := mustInROM(rom, off, gen2BaseDataEntrySize, "BaseData"); err != nil {
		return SpeciesExperienceData{}, err
	}
	growth := GrowthRate(rom[off+gen2BaseGrowthOffset])
	if growth >= growthRateCount {
		return SpeciesExperienceData{}, fmt.Errorf("gs/rom: species %#02x has invalid growth rate %d", species, growth)
	}
	return SpeciesExperienceData{BaseYield: rom[off+10], Growth: growth}, nil
}
