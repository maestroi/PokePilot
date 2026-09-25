package skill

import (
	"errors"
	"fmt"
	"testing"
)

func TestSafariTimedEjectionInterruptedRequiresMapDisplacementAndDialogue(t *testing.T) {
	wrappedDialogue := fmt.Errorf("walk interrupted: %w", ErrDialogueInterrupted)
	if !safariTimedEjectionInterrupted(safariZoneCenterMap, safariZoneGateMap, wrappedDialogue) {
		t.Fatal("gate displacement with dialogue interruption must be treated as timed ejection")
	}
	if safariTimedEjectionInterrupted(safariZoneCenterMap, safariZoneCenterMap, wrappedDialogue) {
		t.Fatal("same-map dialogue interruption is not a timed ejection")
	}
	if safariTimedEjectionInterrupted(safariZoneCenterMap, safariZoneGateMap, errors.New("other failure")) {
		t.Fatal("map displacement without dialogue interruption must not be silently consumed")
	}
}
