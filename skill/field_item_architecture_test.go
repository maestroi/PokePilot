package skill

import (
	"os"
	"strings"
	"testing"
)

func TestOverworldItemDriversDoNotImportConcreteGames(t *testing.T) {
	for _, path := range []string{"field_item.go", "repel.go", "field_item_runtime.go"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, forbidden := range []string{
			"github.com/maestroi/pokepilot/red/",
			"github.com/maestroi/pokepilot/blue/",
			"github.com/maestroi/pokepilot/yellow/",
		} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s imports concrete game package %q", path, forbidden)
			}
		}
	}
}
