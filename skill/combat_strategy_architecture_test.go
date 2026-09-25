package skill

import (
	"os"
	"strings"
	"testing"
)

func TestPortableCombatPolicyHasNoConcreteGen1MechanicsImports(t *testing.T) {
	for _, path := range []string{
		"combat_strategy.go",
		"policy.go",
		"switch_policy.go",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"github.com/maestroi/pokepilot/red/combat",
			"github.com/maestroi/pokepilot/red/rom",
			"github.com/maestroi/pokepilot/red/state",
			"github.com/maestroi/pokepilot/red/sym",
		} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s contains concrete Gen-I mechanics dependency %q", path, forbidden)
			}
		}
	}
}
