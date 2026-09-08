package agent

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
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
