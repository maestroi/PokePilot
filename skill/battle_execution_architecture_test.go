package skill

import (
	"os"
	"strings"
	"testing"
)

func TestBattleExecutionAdapterHasNoConcreteGameDependencies(t *testing.T) {
	for _, path := range []string{"battle_execution.go", "move_learning_state.go"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"github.com/maestroi/pokepilot/red/state",
			"github.com/maestroi/pokepilot/red/sym",
			"github.com/maestroi/pokepilot/blue/",
			"github.com/maestroi/pokepilot/yellow/",
			"github.com/maestroi/pokepilot/gs/",
		} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s imports concrete execution dependency %q", path, forbidden)
			}
		}
	}
}
