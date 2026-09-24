package agent

import (
	"testing"

	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestYellowObservationAdapterRegisteredSeparatelyFromRedBlue(t *testing.T) {
	adapter, ok := semanticObservationAdapterFor(yellowprofile.New())
	if !ok {
		t.Fatal("Yellow semantic observation adapter is not registered")
	}
	if adapter.GameID() != yellowprofile.GameID {
		t.Fatalf("adapter id = %q, want %q", adapter.GameID(), yellowprofile.GameID)
	}
}
