package skill

import (
	"os"
	"strings"
	"testing"
)

func TestGenericOverworldDriversHaveNoConcreteGameDependencies(t *testing.T) {
	for _, path := range []string{"overworld.go", "navigation_overworld.go"} {
		src, err := os.ReadFile(path)
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
				t.Fatalf("%s imports concrete game package %q", path, forbidden)
			}
		}
	}
}

func TestGoToDoesNotReadConcretePositionSymbols(t *testing.T) {
	src, err := os.ReadFile("goto.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"github.com/maestroi/pokepilot/red/sym",
		"Peek8(sym.CurMap)",
		"playerXY(",
	} {
		if strings.Contains(string(src), forbidden) {
			t.Fatalf("goto.go contains concrete live-position dependency %q", forbidden)
		}
	}
}
