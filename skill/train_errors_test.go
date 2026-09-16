package skill_test

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestNoEncounterDiagnosticIsTyped(t *testing.T) {
	err := skill.NoEncounterDiagnostic(141, 2, 4, 0x33, 8, 5)
	if !errors.Is(err, skill.ErrNoEncounterPhase) {
		t.Fatalf("NoEncounterDiagnostic() error = %v, want ErrNoEncounterPhase", err)
	}
}
