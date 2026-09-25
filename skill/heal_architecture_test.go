package skill

import (
	"os"
	"strings"
	"testing"
)

func TestGenericHealRuntimeHasNoConcreteGameDependencies(t *testing.T) {
	src, err := os.ReadFile("heal_runtime.go")
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
			t.Fatalf("heal_runtime.go imports concrete game package %q", forbidden)
		}
	}
}

func TestHealEntryPointDoesNotReadConcreteWorldPosition(t *testing.T) {
	src, err := os.ReadFile("heal.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "func Heal(m *emu.Emu) error {")
	if start < 0 {
		t.Fatal("Heal entry point not found")
	}
	body = body[start:]
	for _, forbidden := range []string{
		"playerXY(",
		"sym.CurMap",
		"sym.XCoord",
		"sym.YCoord",
		"state.Controllable(",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("Heal entry point contains concrete world-state dependency %q", forbidden)
		}
	}
}
