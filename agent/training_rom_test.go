package agent

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func TestROMTrainingViabilityMovesBeyondObsoleteRoute1Band(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Red map ids are canonical ROM structure here: Route 1 is 0x0c and
	// Route 15 is 0x1a. The runtime never hard-codes either as a recommendation;
	// this ROM-backed regression only proves that encounter evidence changes the
	// deterministic estimate when the same lead moves to a stronger band.
	route1, err := skill.WildGrassSlots(romData, 0x0c)
	if err != nil {
		t.Fatal(err)
	}
	route15, err := skill.WildGrassSlots(romData, 0x1a)
	if err != nil {
		t.Fatal(err)
	}
	lead := uint8(0x24) // PIDGEY internal species id in Red.
	policy, err := rom.LookupSpeciesExperience(romData, lead)
	if err != nil {
		t.Fatal(err)
	}
	currentXP, err := rom.ExperienceAtLevel(policy.Growth, 25)
	if err != nil {
		t.Fatal(err)
	}

	weak, err := estimateTraining(romData, lead, currentXP, 25, route1, 27, trainSessionBattleBudget)
	if err != nil {
		t.Fatal(err)
	}
	strong, err := estimateTraining(romData, lead, currentXP, 25, route15, 27, trainSessionBattleBudget)
	if err != nil {
		t.Fatal(err)
	}
	if weak.Viability != TrainingOutsideBudget {
		t.Fatalf("Route 1 at lead L25 = %+v, want outside_budget", weak)
	}
	if strong.Viability == TrainingOutsideBudget {
		t.Fatalf("Route 15 at lead L25 = %+v, want materially better than Route 1", strong)
	}
	if strong.EstimatedEncounters >= weak.EstimatedEncounters {
		t.Fatalf("Route 15 encounters=%d, Route 1=%d; stronger band did not improve effort", strong.EstimatedEncounters, weak.EstimatedEncounters)
	}
}

// A Lick-only trainee cannot damage Route 3's Normal-type wilds (Ghost has
// no effect on Normal in Gen 1). Direct training would flee every encounter,
// so the habitat must be outside budget rather than offered and executed
// (run-2qrxgybx4ic51xfxtib9eon64, triage:48e41d4e4c318585).
func TestROMTrainingRejectsHabitatTheTraineeCannotDamage(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	route3, err := skill.WildGrassSlots(romData, 0x0e)
	if err != nil {
		t.Fatal(err)
	}
	const ghost, poison, lick, tackle = 0x08, 0x03, 122, 33
	gastly := state.Mon{Level: 21, HP: 50, MaxHP: 50, Type1: ghost, Type2: poison, Attack: 25, Defense: 22, Speed: 40, Special: 50}
	gastly.Moves[0], gastly.PP[0] = lick, 30
	if trainingWildsDamageable(romData, gastly, route3) {
		t.Fatal("Lick-only Ghost reported able to damage Route 3's Normal-type wilds")
	}
	gastly.Moves[1], gastly.PP[1] = tackle, 35
	if !trainingWildsDamageable(romData, gastly, route3) {
		t.Fatal("Tackle should damage Route 3's wilds")
	}
}
