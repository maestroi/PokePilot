package skill

import (
	"os"
	"strings"
	"testing"
)

func TestTravelHasNoConcreteGen1RuntimeDependencies(t *testing.T) {
	src, err := os.ReadFile("travel.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"github.com/maestroi/pokepilot/red/state",
		"github.com/maestroi/pokepilot/red/sym",
		"github.com/maestroi/pokepilot/red/rom",
		"github.com/maestroi/pokepilot/red/combat",
		"Peek8(",
		"state.Snapshot(",
	} {
		if strings.Contains(string(src), forbidden) {
			t.Fatalf("travel.go contains concrete Gen-I runtime dependency %q", forbidden)
		}
	}
}
