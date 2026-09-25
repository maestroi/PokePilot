package skill

import (
	"os"
	"strings"
	"testing"
)

func TestPortableBattleRuntimeFilesHaveNoRedRAMDependencies(t *testing.T) {
	for _, path := range []string{
		"battle.go",
		"battle_settlement.go",
		"battle_medicine_execution.go",
		"battle_runtime.go",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"github.com/maestroi/pokepilot/red/state",
			"github.com/maestroi/pokepilot/red/sym",
			"state.Snapshot(",
			"Peek8(sym.",
		} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s contains concrete Red RAM dependency %q", path, forbidden)
			}
		}
	}
}
