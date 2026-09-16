package main

import (
	"encoding/json"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestRunFinishViewPreservesCoverage(t *testing.T) {
	report := farm.FinishReport{
		RunID: "completionist-coverage",
		ProgressFinal: &farm.Progress{
			Round: 17,
			Coverage: &farm.Coverage{
				UniqueMapsVisited: 9,
				TrainersDefeated:  12,
				NPCInteractions:   15,
				DexOwned:          22,
				TMsHMsUsed:        3,
			},
		},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded farm.FinishReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	view := runFinishView{
		ProgressEarly: decoded.ProgressEarly,
		ProgressFinal: decoded.ProgressFinal,
	}
	if view.ProgressFinal == nil || view.ProgressFinal.Coverage == nil {
		t.Fatal("run inspection lost coverage")
	}
	if view.ProgressFinal.Coverage.TrainersDefeated != 12 || view.ProgressFinal.Coverage.DexOwned != 22 {
		t.Fatalf("inspection coverage = %+v", view.ProgressFinal.Coverage)
	}
}

func TestRunSummaryTreatsCoverageOnlyDeltaAsProgress(t *testing.T) {
	report := &farm.FinishReport{
		ProgressEarly: &farm.Progress{
			Map:  10,
			Maps: 4,
			Coverage: &farm.Coverage{
				UniqueMapsVisited: 4,
				DexOwned:          8,
			},
		},
		ProgressFinal: &farm.Progress{
			Map:  10,
			Maps: 4,
			Coverage: &farm.Coverage{
				UniqueMapsVisited: 4,
				TrainersDefeated:  3,
				NPCInteractions:   5,
				DexOwned:          10,
				TMsHMsUsed:        1,
			},
		},
	}
	summary := summarizeRun(report, nil)
	if !summary.ProgressKnown || !summary.Progressed {
		t.Fatalf("coverage-only progress not recognized: %+v", summary)
	}
	if summary.CoverageDelta == nil {
		t.Fatal("coverage delta missing")
	}
	if summary.CoverageDelta.TrainersDefeated != 3 || summary.CoverageDelta.NPCInteractions != 5 || summary.CoverageDelta.DexOwned != 2 || summary.CoverageDelta.TMsHMsUsed != 1 {
		t.Fatalf("coverage delta = %+v", summary.CoverageDelta)
	}
}
