package skill

import (
	"os"
	"strings"
	"testing"
)

func TestGenericInteractionRuntimeHasNoConcreteGameDependencies(t *testing.T) {
	src, err := os.ReadFile("interaction_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"github.com/maestroi/pokepilot/red/",
		"github.com/maestroi/pokepilot/blue/",
		"github.com/maestroi/pokepilot/yellow/",
		"github.com/maestroi/pokepilot/gs/",
	} {
		if strings.Contains(string(src), forbidden) {
			t.Fatalf("interaction_runtime.go imports concrete game package %q", forbidden)
		}
	}
}

func TestInteractionPositioningDoesNotReadConcreteWorldCoordinates(t *testing.T) {
	for _, path := range []string{"interact.go", "destination_navigation.go"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"playerXY(",
			"Peek8(sym.CurMap)",
			"Peek8(sym.XCoord)",
			"Peek8(sym.YCoord)",
		} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s contains concrete interaction-position dependency %q", path, forbidden)
			}
		}
	}
}
