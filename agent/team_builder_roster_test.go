package agent

import (
	"strings"
	"testing"
)

func TestTeamBuilderIncompleteRosterPrefersCatchOverMoreCoreTraining(t *testing.T) {
	profile := PlayStyle(PlayStyleTeamBuilder)
	for _, count := range []int{3, 5} {
		t.Run(string(rune('0'+count))+"-members", func(t *testing.T) {
			party := []PartyMon{{Species: "charmeleon", Level: 20, HP: 50, MaxHP: 50}}
			for i := 1; i < count; i++ {
				party = append(party, PartyMon{Species: SpeciesID("member" + string(rune('a'+i))), Level: 15, HP: 35, MaxHP: 35})
			}
			obs := Observation{Map: 0xff, PartyCount: count, Party: party}
			catch := ScoreObjective(obs, Objective{Kind: KindCatch, Species: "pidgey", Place: "route 1", Flee: true}, profile)
			train := ScoreObjective(obs, Objective{Kind: KindTrain, Species: party[1].Species, Slot: 1, Level: 18}, profile)

			if !naturalSignalHasTag(catch.Natural, "fill-roster") {
				t.Fatalf("%d-member catch signal = %+v, want fill-roster", count, catch.Natural)
			}
			if !naturalSignalHasTag(train.Natural, "fill-roster-first") {
				t.Fatalf("%d-member train signal = %+v, want fill-roster-first", count, train.Natural)
			}
			if catch.Total <= train.Total {
				t.Fatalf("%d-member Team Builder should fill roster before more core training: catch %.3f train %.3f", count, catch.Total, train.Total)
			}
		})
	}
}

func TestTeamBuilderRosterPressureStopsAtSix(t *testing.T) {
	obs := Observation{
		PartyCount: 6,
		Party: []PartyMon{
			{Species: "charizard", Level: 30},
			{Species: "pikachu", Level: 20},
			{Species: "nidoking", Level: 28},
			{Species: "lapras", Level: 27},
			{Species: "snorlax", Level: 26},
			{Species: "pidgeot", Level: 25},
		},
	}
	profile := PlayStyle(PlayStyleTeamBuilder)

	catch := ScoreObjective(obs, Objective{Kind: KindCatch, Species: "rattata"}, profile)
	if naturalSignalHasTag(catch.Natural, "fill-roster") {
		t.Fatalf("full roster catch signal = %+v, still has fill-roster", catch.Natural)
	}

	train := ScoreObjective(obs, Objective{Kind: KindTrain, Species: "pikachu", Slot: 1, Level: 24}, profile)
	if !naturalSignalHasTag(train.Natural, "balance-full-roster") {
		t.Fatalf("full roster weak-member training signal = %+v, want balance-full-roster", train.Natural)
	}
	progress := ScoreObjective(obs, Objective{Kind: KindProgress, Progress: "test_progress"}, profile)
	if train.Total <= progress.Total {
		t.Fatalf("weak full-roster member should be developed before generic progress: train %.3f progress %.3f", train.Total, progress.Total)
	}
}

func TestTeamBuilderValuesCaptureSuppliesUntilRosterIsFull(t *testing.T) {
	profile := PlayStyle(PlayStyleTeamBuilder)
	obs := Observation{PartyCount: 4, Party: []PartyMon{{Species: "charmeleon", Level: 20}}}

	mart := ScoreObjective(obs, Objective{Kind: KindGoTo, Place: "viridian mart"}, profile)
	if !naturalSignalHasTag(mart.Natural, "roster-resupply") {
		t.Fatalf("Team Builder Mart signal = %+v, want roster-resupply", mart.Natural)
	}
	buy := ScoreObjective(obs, Objective{Kind: KindBuy, Item: "pokeball", Qty: 5}, profile)
	if !naturalSignalHasTag(buy.Natural, "roster-supplies") {
		t.Fatalf("Team Builder ball purchase signal = %+v, want roster-supplies", buy.Natural)
	}

	obs.PartyCount = 6
	obs.Party = append(obs.Party,
		PartyMon{Species: "pikachu", Level: 18},
		PartyMon{Species: "nidoking", Level: 18},
	)
	mart = ScoreObjective(obs, Objective{Kind: KindGoTo, Place: "viridian mart"}, profile)
	if naturalSignalHasTag(mart.Natural, "roster-resupply") {
		t.Fatalf("full roster Mart signal = %+v, still has roster-resupply", mart.Natural)
	}
}

func TestTeamBuilderRewardsReadyNowAndEvolutionUpside(t *testing.T) {
	obs := Observation{
		PartyCount: 5,
		Party: []PartyMon{
			{Species: "charmeleon", Level: 16},
			{Species: "pikachu", Level: 12},
			{Species: "nidorino", Level: 12},
			{Species: "butterfree", Level: 11},
			{Species: "rattata", Level: 10},
		},
		Dex: DexCatalog{Targets: []DexEntry{
			{Species: "pidgey", Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 1", Level: 10}}},
			{Species: "pidgeotto", Sources: []DexSource{{Kind: AcquireLevelEvo, From: "pidgey", Level: 18}}},
		}},
	}
	got := ScoreObjective(obs, Objective{Kind: KindCatch, Species: "pidgey", Place: "route 1"}, PlayStyle(PlayStyleTeamBuilder))
	if !naturalSignalHasTag(got.Natural, "battle-ready-catch") {
		t.Fatalf("quality catch signal = %+v, want battle-ready-catch", got.Natural)
	}
	if !naturalSignalHasTag(got.Natural, "evolution-upside") {
		t.Fatalf("quality catch signal = %+v, want evolution-upside", got.Natural)
	}
}

func TestTeamBuilderRosterSignalDoesNotLeakToOtherProfiles(t *testing.T) {
	obs := Observation{PartyCount: 3, Party: []PartyMon{{Species: "charmeleon", Level: 18}}}
	o := Objective{Kind: KindCatch, Species: "pidgey"}
	for _, style := range []string{PlayStyleAdventure, PlayStyleCompletionist} {
		got := ScoreObjective(obs, o, PlayStyle(style))
		if naturalSignalHasTag(got.Natural, "fill-roster") {
			t.Fatalf("%s catch signal leaked Team Builder roster policy: %+v", style, got.Natural)
		}
	}
}

func TestTeamBuilderSystemNotePinsFullCapableRoster(t *testing.T) {
	got := PlayStyleSystemNote(PlayStyleTeamBuilder)
	for _, want := range []string{
		"full six-Pokémon party",
		"3, 4, or 5 is still incomplete",
		"strong complementary choices",
		"evolution upside",
		"whole team develops",
		"only the lead or top three",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Team Builder system note = %q, want substring %q", got, want)
		}
	}
}
