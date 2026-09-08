package rom

import "testing"

func TestExperienceAtLevelGrowthRates(t *testing.T) {
	for _, tc := range []struct {
		name string
		rate GrowthRate
		want uint32
	}{
		{"medium fast", GrowthMediumFast, 1000},
		{"slightly fast", GrowthSlightlyFast, 1720},
		{"slightly slow", GrowthSlightlySlow, 2680},
		{"medium slow", GrowthMediumSlow, 560},
		{"fast", GrowthFast, 800},
		{"slow", GrowthSlow, 1250},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExperienceAtLevel(tc.rate, 10)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("ExperienceAtLevel(%d,10) = %d, want %d", tc.rate, got, tc.want)
			}
		})
	}
}

func TestExperienceAtLevelRejectsInvalidInputAndClampsLevelOne(t *testing.T) {
	if got, err := ExperienceAtLevel(GrowthMediumSlow, 1); err != nil || got != 0 {
		t.Fatalf("medium-slow level 1 = %d,%v, want 0,nil", got, err)
	}
	if _, err := ExperienceAtLevel(GrowthMediumFast, 0); err == nil {
		t.Fatal("level 0 accepted")
	}
	if _, err := ExperienceAtLevel(growthRateCount, 10); err == nil {
		t.Fatal("invalid growth rate accepted")
	}
}

func TestLookupSpeciesExperienceUsesPokedexOrderAndBaseStats(t *testing.T) {
	romData := make([]byte, pokedexOrderOffset+pokedexOrderLen)
	internal := uint8(0x24)
	dex := uint8(16)
	romData[pokedexOrderOffset+int(internal)-1] = dex
	entry := baseStatsOffset + (int(dex)-1)*baseStatsEntryLen
	romData[entry+baseStatsBaseExpOffset] = 55
	romData[entry+baseStatsGrowthRateOffset] = byte(GrowthMediumSlow)

	got, err := LookupSpeciesExperience(romData, internal)
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseYield != 55 || got.Growth != GrowthMediumSlow {
		t.Fatalf("LookupSpeciesExperience = %+v, want base=55 growth=%d", got, GrowthMediumSlow)
	}
}

func TestWildBattleExperience(t *testing.T) {
	if got := WildBattleExperience(55, 5); got != 39 {
		t.Fatalf("WildBattleExperience(55,5) = %d, want 39", got)
	}
}
