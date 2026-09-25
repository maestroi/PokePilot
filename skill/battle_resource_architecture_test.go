package skill

import (
	"os"
	"strings"
	"testing"
)

func TestBattleResourcePolicyHasNoRedRAMDependencies(t *testing.T) {
	for _, path := range []string{"battle_resource_policy.go", "switch_policy.go", "battle_resource_state.go"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"github.com/maestroi/pokepilot/red/state",
			"github.com/maestroi/pokepilot/red/sym",
		} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s imports Red RAM package %q", path, forbidden)
			}
		}
	}
}
