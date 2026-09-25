package skill

import (
	"os"
	"strings"
	"testing"
)

func TestGenericCaptureRuntimeHasNoConcreteGameDependencies(t *testing.T) {
	src, err := os.ReadFile("capture_runtime.go")
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
			t.Fatalf("capture_runtime.go imports concrete game package %q", forbidden)
		}
	}
}
