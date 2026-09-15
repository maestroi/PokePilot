package agent

import (
	"os"
	"testing"
)

// TestObservedTrainersAreReachable pins farm issue #410. Route 12 is one map
// id with traversal components separated by the Snorlax/story geometry. A run
// standing at (9,62) was offered the trainer whose ROM home is (14,31), then
// ChallengeTrainer delegated to TalkAt and failed forever trying to reach the
// trainer's approach tile (13,31). Trainers must obey the same component-aware
// approach filter as ordinary people before they enter Observation.MapObjects.
func TestObservedTrainersAreReachable(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}

	const route12 = 0x17
	trainerX, trainerY := uint8(14), uint8(31)

	if personReachable(romData, route12, 9, 62, trainerX, trainerY) {
		t.Error("Route 12 trainer at (14,31) reported reachable from trapped south component (9,62)")
	}
	if !personReachable(romData, route12, 13, 31, trainerX, trainerY) {
		t.Error("Route 12 trainer at (14,31) reported unreachable from adjacent approach tile (13,31)")
	}
}
