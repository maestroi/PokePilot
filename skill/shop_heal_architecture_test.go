package skill

import (
	"os"
	"strings"
	"testing"
)

func TestShopAndHealTransactionsHaveNoConcreteGameDependencies(t *testing.T) {
	for _, path := range []string{"shop.go", "heal_transaction.go"} {
		src, err := os.ReadFile(path)
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
				t.Fatalf("%s imports concrete transaction dependency %q", path, forbidden)
			}
		}
	}
}
