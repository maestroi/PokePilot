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

func TestCatchRuntimesDoNotDecodeConcreteAcquisitionState(t *testing.T) {
	for _, path := range []string{"catch.go", "water_catch.go", "fishing.go"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"state.DecodeParty(",
			"state.DecodeBox(",
			"state.DecodePokedex(",
			"reddata.WildCaptureBallOrder(",
		} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s contains concrete capture/inventory dependency %q", path, forbidden)
			}
		}
	}
}
