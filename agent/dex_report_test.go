package agent

import "testing"

func TestBuildDexCompletionReportClassifiesFinalCatalog(t *testing.T) {
	obs := Observation{Dex: DexCatalog{
		Owned: []DexEntry{
			{Species: "mewtwo", Dex: 150, Owned: true, Sources: []DexSource{{Kind: AcquireStatic}}},
			{Species: "bulbasaur", Dex: 1, Owned: true, Sources: []DexSource{{Kind: AcquireStarter}}},
		},
		Targets:     []DexEntry{{Species: "pidgey", Dex: 16, Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 1"}}}},
		Unavailable: []DexEntry{{Species: "alakazam", Dex: 65, Unavailable: UnavailableTradeEvolution, Sources: []DexSource{{Kind: AcquireTradeEvo, From: "kadabra"}}}},
	}}
	outcomes := []ObjectiveResult{{
		Objective: Objective{Kind: KindCatch, Species: "mewtwo", Intent: dexStaticIntent},
		Outcome:   OutcomeCompleted,
	}}

	report := BuildDexCompletionReport(obs, outcomes)
	if report.Complete || report.Owned != 2 || report.Obtainable != 3 || report.Remaining != 1 || report.Unavailable != 1 {
		t.Fatalf("report counts = %+v", report)
	}
	if report.AcquisitionKnown != 1 {
		t.Fatalf("AcquisitionKnown = %d, want 1", report.AcquisitionKnown)
	}
	if len(report.Species) != 4 {
		t.Fatalf("species rows = %d, want 4", len(report.Species))
	}
	// Rows are stable National-Dex order, independent of catalog slice order.
	if report.Species[0].Species != "bulbasaur" || report.Species[1].Species != "pidgey" || report.Species[2].Species != "alakazam" || report.Species[3].Species != "mewtwo" {
		t.Fatalf("species order = %+v", report.Species)
	}
	if report.Species[2].Status != DexReportUnavailable || report.Species[2].Reason != UnavailableTradeEvolution {
		t.Fatalf("Alakazam row = %+v", report.Species[2])
	}
	if report.Species[3].Method != AcquireStatic {
		t.Fatalf("Mewtwo method = %q, want %q", report.Species[3].Method, AcquireStatic)
	}
}

func TestBuildDexCompletionReportRecordsVerifiedItemEvolution(t *testing.T) {
	owned := DexEntry{
		Species: "wigglytuff",
		Dex:     40,
		Owned:   true,
		Sources: []DexSource{{Kind: AcquireItemEvo, From: "jigglypuff", Item: "moon stone"}},
	}
	obs := Observation{
		Party: []PartyMon{{Species: "squirtle"}, {Species: "wigglytuff"}},
		Dex:   DexCatalog{Owned: []DexEntry{owned}},
	}
	outcomes := []ObjectiveResult{{
		Objective: Objective{Kind: KindUseItem, Item: "moon stone", Slot: 1, Intent: "dex-evolution"},
		Outcome:   OutcomeCompleted,
		Final:     obs,
	}}
	report := BuildDexCompletionReport(obs, outcomes)
	if !report.Complete || report.Owned != 1 || report.Remaining != 0 {
		t.Fatalf("complete report = %+v", report)
	}
	if len(report.Species) != 1 || report.Species[0].Method != AcquireItemEvo {
		t.Fatalf("Wigglytuff report = %+v", report.Species)
	}
}

func TestBuildDexCompletionReportEmptyCatalogNeverCompletes(t *testing.T) {
	report := BuildDexCompletionReport(Observation{}, nil)
	if report.Complete {
		t.Fatalf("empty catalog reported complete: %+v", report)
	}
	if report.Obtainable != 0 || report.Summary == "" {
		t.Fatalf("empty catalog report = %+v", report)
	}
}

func TestResultDexReportUsesFinalObservationAndOutcomes(t *testing.T) {
	result := Result{
		Final: Observation{Dex: DexCatalog{Owned: []DexEntry{{Species: "zapdos", Dex: 145, Owned: true}}}},
		Outcomes: []ObjectiveResult{{
			Objective: Objective{Kind: KindCatch, Species: "zapdos", Intent: dexStaticIntent},
			Outcome:   OutcomeCompleted,
		}},
	}
	report := result.DexReport()
	if !report.Complete || len(report.Species) != 1 || report.Species[0].Method != AcquireStatic {
		t.Fatalf("Result.DexReport() = %+v", report)
	}
}
