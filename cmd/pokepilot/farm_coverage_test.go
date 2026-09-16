package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestFarmProgressCarriesCoverage(t *testing.T) {
	got := farmProgress(&agent.Progress{
		Round: 9,
		Maps:  5,
		Coverage: agent.Coverage{
			UniqueMapsVisited:     5,
			TrainersDefeated:      7,
			NPCInteractions:       11,
			UniqueItemsAcquired:   6,
			UniqueItemsUsed:       3,
			DexOwned:              14,
			DexSeen:               20,
			OptionalMilestones:    19,
			TMsHMsAcquired:        2,
			TMsHMsUsed:            1,
			Evolutions:            2,
			Catches:               8,
			UniqueSpeciesAcquired: 14,
		},
	})
	if got == nil || got.Coverage == nil {
		t.Fatal("farm progress omitted coverage")
	}
	if got.Coverage.TrainersDefeated != 7 || got.Coverage.NPCInteractions != 11 || got.Coverage.DexOwned != 14 {
		t.Fatalf("farm coverage = %+v", got.Coverage)
	}
}
