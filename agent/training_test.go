package agent

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

const (
	trainingTestPokedexOrderOffset = 0x41024
	trainingTestPokedexOrderLen    = 190
	trainingTestBaseStatsOffset    = 0x383DE
	trainingTestBaseStatsEntryLen  = 28
)

type trainingSpeciesFixture struct {
	internal uint8
	dex      uint8
	baseExp  uint8
	growth   rom.GrowthRate
}

func trainingTestROM(t *testing.T, fixtures ...trainingSpeciesFixture) []byte {
	t.Helper()
	romData := make([]byte, trainingTestPokedexOrderOffset+trainingTestPokedexOrderLen)
	for _, f := range fixtures {
		romData[trainingTestPokedexOrderOffset+int(f.internal)-1] = f.dex
		entry := trainingTestBaseStatsOffset + (int(f.dex)-1)*trainingTestBaseStatsEntryLen
		romData[entry+9] = f.baseExp
		romData[entry+19] = byte(f.growth)
	}
	return romData
}

func trainingFixtureROM(t *testing.T, wildBase uint8) []byte {
	return trainingTestROM(t,
		trainingSpeciesFixture{internal: 1, dex: 1, baseExp: 64, growth: rom.GrowthMediumFast},
		trainingSpeciesFixture{internal: 2, dex: 2, baseExp: wildBase, growth: rom.GrowthMediumFast},
	)
}

func repeatedWildSlots(species, level uint8) []skill.WildEncounterSlot {
	out := make([]skill.WildEncounterSlot, 10)
	for i := range out {
		out[i] = skill.WildEncounterSlot{ID: species, Level: level}
	}
	return out
}

func TestTrainingEstimateWeakBandIsOutsideBudget(t *testing.T) {
	romData := trainingFixtureROM(t, 50)
	currentXP, _ := rom.ExperienceAtLevel(rom.GrowthMediumFast, 16)
	got, err := estimateTraining(romData, 1, currentXP, 16, repeatedWildSlots(2, 4), 18, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Viability != TrainingOutsideBudget {
		t.Fatalf("weak-band viability = %+v, want outside_budget", got)
	}
	if got.EstimatedEncounters <= got.SessionBudget {
		t.Fatalf("weak-band estimate = %+v, want encounters above session budget", got)
	}
	if !strings.Contains(got.Diagnostic(), "outside current session budget") {
		t.Fatalf("diagnostic = %q, want explicit outside-budget reason", got.Diagnostic())
	}
}

func TestTrainingEstimateStrongerBandIsViable(t *testing.T) {
	romData := trainingFixtureROM(t, 100)
	currentXP, _ := rom.ExperienceAtLevel(rom.GrowthMediumFast, 16)
	got, err := estimateTraining(romData, 1, currentXP, 16, repeatedWildSlots(2, 14), 18, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Viability != TrainingViable {
		t.Fatalf("strong-band viability = %+v, want viable", got)
	}
	if got.EstimatedEncounters >= 20 {
		t.Fatalf("strong-band estimate = %+v, want comfortably inside budget", got)
	}
}

func TestTrainingEstimateExpensiveNearBudget(t *testing.T) {
	romData := trainingFixtureROM(t, 60)
	currentXP, _ := rom.ExperienceAtLevel(rom.GrowthMediumFast, 16)
	got, err := estimateTraining(romData, 1, currentXP, 16, repeatedWildSlots(2, 13), 18, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Viability != TrainingExpensive {
		t.Fatalf("near-budget viability = %+v, want expensive", got)
	}
}

func TestTrainingEstimateWeightsRareHighLevelSlots(t *testing.T) {
	romData := trainingFixtureROM(t, 100)
	currentXP, _ := rom.ExperienceAtLevel(rom.GrowthMediumFast, 16)
	chances := [...]uint16{51, 51, 39, 25, 25, 25, 13, 13, 11, 3}
	slots := make([]skill.WildEncounterSlot, len(chances))
	for i, chance := range chances {
		slots[i] = skill.WildEncounterSlot{ID: 2, Level: 4, Chance: chance}
	}
	// Slot 9 is only 3/256 of encounters. An equal-slot arithmetic mean
	// overweights this outlier by more than 8x and incorrectly puts the L16
	// -> L18 target inside a 20-battle session.
	slots[9].Level = 50

	got, err := estimateTraining(romData, 1, currentXP, 16, slots, 18, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Viability != TrainingOutsideBudget {
		t.Fatalf("weighted rare-slot estimate = %+v, want outside_budget", got)
	}
	if got.EstimatedEncounters <= got.SessionBudget {
		t.Fatalf("weighted rare-slot estimate = %+v, want encounters above session budget", got)
	}
}

func TestTrainingEstimateNearTargetUsesCurrentProgress(t *testing.T) {
	romData := trainingFixtureROM(t, 50)
	targetXP, _ := rom.ExperienceAtLevel(rom.GrowthMediumFast, 18)
	got, err := estimateTraining(romData, 1, targetXP-50, 17, repeatedWildSlots(2, 4), 18, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Viability != TrainingViable || got.XPRemaining != 50 || got.EstimatedEncounters > 2 {
		t.Fatalf("near-target estimate = %+v, want viable with 50 XP remaining in <=2 encounters", got)
	}
}

func TestTrainingEstimateAlreadySatisfied(t *testing.T) {
	romData := trainingFixtureROM(t, 50)
	targetXP, _ := rom.ExperienceAtLevel(rom.GrowthMediumFast, 18)
	got, err := estimateTraining(romData, 1, targetXP, 18, repeatedWildSlots(2, 4), 18, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Viability != TrainingSatisfied || got.XPRemaining != 0 || got.EstimatedEncounters != 0 {
		t.Fatalf("satisfied estimate = %+v", got)
	}
}

func TestTrainingInefficientErrorIsTyped(t *testing.T) {
	err := &TrainingInefficientError{Estimate: TrainingEstimate{Viability: TrainingOutsideBudget, SessionBudget: 20}}
	if !errors.Is(err, ErrTrainingInefficient) {
		t.Fatal("TrainingInefficientError does not unwrap to ErrTrainingInefficient")
	}
	cause, _ := failureCauseFor(err)
	if cause != "training_inefficient_area" {
		t.Fatalf("failure cause = %q, want training_inefficient_area", cause)
	}
}

func TestFailureStateTracksCumulativeExperience(t *testing.T) {
	a := FailureStateFor(Observation{Party: []PartyMon{{Species: "pidgey", Level: 16, Experience: 4100}}})
	b := FailureStateFor(Observation{Party: []PartyMon{{Species: "pidgey", Level: 16, Experience: 4200}}})
	if len(a.Party) != 1 || len(b.Party) != 1 || a.Party[0].Experience == b.Party[0].Experience {
		t.Fatalf("failure state did not preserve XP progress: a=%+v b=%+v", a.Party, b.Party)
	}
}

func TestTrainingOfferWithholdsOutsideBudgetLeadTraining(t *testing.T) {
	obs := Observation{
		Map:        0,
		PartyCount: 1,
		Party: []PartyMon{{
			Species: "pidgey", Level: 16, HP: 20, MaxHP: 20,
		}},
		HasGrass:  true,
		WildGrass: []WildSpecies{{Name: "pidgey", MinLevel: 2, MaxLevel: 5, Slots: 10}},
		Training: &TrainingEstimate{
			CurrentLevel: 16, TargetLevel: 18, XPRemaining: 1700,
			XPPerEncounter: 28, EstimatedEncounters: 61,
			SessionBudget: 20, Viability: TrainingOutsideBudget,
		},
	}
	for _, offer := range Offer(obs, NewKnowledge(map[uint8][]uint8{})) {
		if offer.Kind == KindTrain {
			t.Fatalf("outside-budget training was offered: %+v", offer)
		}
	}
}

func TestTrainingOfferKeepsViableLeadTraining(t *testing.T) {
	obs := Observation{
		Map:        0,
		PartyCount: 1,
		Party: []PartyMon{{
			Species: "pidgey", Level: 16, HP: 20, MaxHP: 20,
		}},
		HasGrass:  true,
		WildGrass: []WildSpecies{{Name: "pidgey", MinLevel: 14, MaxLevel: 16, Slots: 10}},
		Training: &TrainingEstimate{
			CurrentLevel: 16, TargetLevel: 18, XPRemaining: 500,
			XPPerEncounter: 100, EstimatedEncounters: 5,
			SessionBudget: 20, Viability: TrainingViable,
		},
	}
	for _, offer := range Offer(obs, NewKnowledge(map[uint8][]uint8{})) {
		if offer.Kind == KindTrain && offer.Level == 18 {
			if !strings.Contains(offer.Note, "training viable") {
				t.Fatalf("training note = %q, want viability evidence", offer.Note)
			}
			return
		}
	}
	t.Fatal("viable lead training objective was not offered")
}

func TestTrainingInefficiencyClassifiesAsRecoverableBlockage(t *testing.T) {
	err := &TrainingInefficientError{Estimate: TrainingEstimate{Viability: TrainingOutsideBudget, SessionBudget: 20}}
	if got := classifyObjectiveOutcome(Objective{Kind: KindTrain}, err, Observation{}); got != OutcomeBlocked {
		t.Fatalf("classifyObjectiveOutcome(training inefficiency) = %q, want %q", got, OutcomeBlocked)
	}
	if got := actionFor(OutcomeBlocked); got != actionReplan {
		t.Fatalf("actionFor(blocked) = %v, want replan", got)
	}
}

func TestApplyPartyTrainingMethodUsesSwitchTrainingForUnsafeTarget(t *testing.T) {
	party := state.PartyState{Count: 2, Mons: []state.Mon{
		{Species: 1, Level: 12, HP: 30, MaxHP: 30, PP: [4]uint8{20}},
		{Species: 2, Level: 45, HP: 120, MaxHP: 120, PP: [4]uint8{20}},
	}}
	slots := repeatedWildSlots(2, 40)
	estimate := TrainingEstimate{
		CurrentLevel: 12, TargetLevel: 14, XPRemaining: 400,
		XPPerEncounter: 200, EstimatedEncounters: 2,
		SessionBudget: 20, Viability: TrainingViable, Method: TrainingDirect,
	}

	got := applyPartyTrainingMethod(party, 0, slots, estimate)
	if got.Method != TrainingSwitch {
		t.Fatalf("method = %q, want switch training", got.Method)
	}
	if got.CarryLevel != 45 || got.WildMaxLevel != 40 || got.MinCarryLevel != 38 {
		t.Fatalf("switch metadata = %+v, want carry L45, wild max L40, minimum carry L38", got)
	}
	if got.XPPerEncounter != 100 || got.EstimatedEncounters != 4 || got.Viability != TrainingViable {
		t.Fatalf("switch XP estimate = %+v, want 100 XP/encounter and 4 encounters", got)
	}
	if !strings.Contains(got.Diagnostic(), "switch training") || !strings.Contains(got.Diagnostic(), "shared XP") {
		t.Fatalf("diagnostic = %q, want explicit switch/shared-XP evidence", got.Diagnostic())
	}
}

func TestApplyPartyTrainingMethodRejectsUnsafeAreaWithoutCarry(t *testing.T) {
	party := state.PartyState{Count: 2, Mons: []state.Mon{
		{Species: 1, Level: 12, HP: 30, MaxHP: 30, PP: [4]uint8{20}},
		{Species: 2, Level: 20, HP: 60, MaxHP: 60, PP: [4]uint8{20}},
	}}
	estimate := TrainingEstimate{
		CurrentLevel: 12, TargetLevel: 14, XPRemaining: 400,
		XPPerEncounter: 200, EstimatedEncounters: 2,
		SessionBudget: 20, Viability: TrainingViable, Method: TrainingDirect,
	}

	got := applyPartyTrainingMethod(party, 0, repeatedWildSlots(2, 40), estimate)
	if got.Viability != TrainingOutsideBudget || got.XPPerEncounter != 0 {
		t.Fatalf("unsafe no-carry estimate = %+v, want outside-budget safety block", got)
	}
	if !strings.Contains(got.Diagnostic(), "no healthy carry") {
		t.Fatalf("diagnostic = %q, want no-carry safety reason", got.Diagnostic())
	}
}

func TestApplyPartyTrainingMethodKeepsDirectTrainingForManageableWilds(t *testing.T) {
	party := state.PartyState{Count: 2, Mons: []state.Mon{
		{Species: 1, Level: 20, HP: 60, MaxHP: 60, PP: [4]uint8{20}},
		{Species: 2, Level: 45, HP: 120, MaxHP: 120, PP: [4]uint8{20}},
	}}
	estimate := TrainingEstimate{
		CurrentLevel: 20, TargetLevel: 22, XPRemaining: 400,
		XPPerEncounter: 200, EstimatedEncounters: 2,
		SessionBudget: 20, Viability: TrainingViable, Method: TrainingDirect,
	}

	got := applyPartyTrainingMethod(party, 0, repeatedWildSlots(2, 23), estimate)
	if got.Method != TrainingDirect || got.XPPerEncounter != 200 || got.EstimatedEncounters != 2 {
		t.Fatalf("manageable-wild estimate = %+v, want unchanged direct training", got)
	}
}
