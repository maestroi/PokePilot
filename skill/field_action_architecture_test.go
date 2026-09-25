package skill

import (
	"os"
	"strings"
	"testing"
)

func TestGenericFieldActionRuntimeHasNoConcreteGameDependencies(t *testing.T) {
	src, err := os.ReadFile("field_action_runtime.go")
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
			t.Fatalf("field_action_runtime.go imports concrete game package %q", forbidden)
		}
	}
}

func TestFieldActionExecutionDoesNotDecodeConcreteEffectFlags(t *testing.T) {
	src, err := os.ReadFile("field_action.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"mem.U8(sym.TileInFrontOfPlayer)",
		"mem.U8(sym.WalkBikeSurfState)",
		"mem.U8(sym.StatusFlags1)",
		"mem.U8(sym.MapPalOffset)",
		"mem.U8(sym.ActionResult)",
	} {
		if strings.Contains(string(src), forbidden) {
			t.Fatalf("field_action.go contains concrete runtime-effect read %q", forbidden)
		}
	}
}
