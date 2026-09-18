package skill

import (
	"os"
	"strings"
	"testing"
)

func TestBootDriverHasNoConcreteGameDependencies(t *testing.T) {
	src, err := os.ReadFile("boot.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"github.com/maestroi/pokepilot/red/",
		"github.com/maestroi/pokepilot/blue/",
		"github.com/maestroi/pokepilot/yellow/",
	} {
		if strings.Contains(string(src), forbidden) {
			t.Fatalf("boot.go imports concrete game package %q", forbidden)
		}
	}
}
