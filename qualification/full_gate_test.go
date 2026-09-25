package qualification

import (
	"os"
	"strings"
	"testing"
)

func TestFreshHallOfFameWorkflowIsAnActivePrivateGate(t *testing.T) {
	data, err := os.ReadFile("../.github/workflows/qualification.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	if strings.Contains(text, "POKEPILOT_FULL_QUALIFICATION") {
		t.Fatal("weekly full qualification is still hidden behind the pre-#39 repository-variable gate")
	}
	for _, want := range []string{
		"github.event.schedule == '41 3 * * 0'",
		"Run fresh-save Hall of Fame benchmark qualification",
		"go run ./cmd/pokebench red",
		"--from fresh",
		"--until hall-of-fame",
		"gh issue close 39",
		"issues: write",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("qualification workflow missing %q", want)
		}
	}
}
