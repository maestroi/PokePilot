package farm

import (
	"encoding/json"
	"testing"
)

func TestProgressCoverageRoundTripsOnFinishWire(t *testing.T) {
	report := FinishReport{
		RunID: "coverage-run",
		ProgressFinal: &Progress{
			Round: 42,
			Maps:  12,
			Coverage: &Coverage{
				UniqueMapsVisited:     12,
				TrainersDefeated:      18,
				NPCInteractions:       25,
				UniqueItemsAcquired:   14,
				UniqueItemsUsed:       6,
				DexOwned:              33,
				DexSeen:               48,
				OptionalMilestones:    40,
				TMsHMsAcquired:        7,
				TMsHMsUsed:            4,
				Evolutions:            5,
				Catches:               21,
				UniqueSpeciesAcquired: 33,
			},
		},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var got FinishReport
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ProgressFinal == nil || got.ProgressFinal.Coverage == nil {
		t.Fatalf("coverage missing after round-trip: %s", data)
	}
	if got.ProgressFinal.Coverage.TrainersDefeated != 18 || got.ProgressFinal.Coverage.TMsHMsUsed != 4 || got.ProgressFinal.Coverage.DexOwned != 33 {
		t.Fatalf("coverage round-trip = %+v", got.ProgressFinal.Coverage)
	}
}
