package monitoring

import (
	"os"
	"strings"
	"testing"
)

func TestPrometheusRulesAreBoundedAndActionable(t *testing.T) {
	b, err := os.ReadFile("prometheus-pokepilot-rules.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, name := range []string{
		"pokepilot:run_completion_ratio:1h",
		"pokepilot:run_terminal_rate:1h",
		"pokepilot:run_duration_p95_seconds:1h",
		"pokepilot:failure_recovery_ratio:6h",
		"alert: PokePilotWallDown",
		"alert: PokePilotNoSuccessfulRun",
		"alert: PokePilotCompletionRateLow",
		"alert: PokePilotTerminalFailureSpike",
	} {
		if !strings.Contains(text, name) {
			t.Errorf("rules file missing %s", name)
		}
	}
	// The application alert must stay distinct from "the process is down".
	if !strings.Contains(text, `pokepilot_up{service="wall"}`) || !strings.Contains(text, `up{job="pokepilot-wall"}`) {
		t.Fatal("rules do not distinguish wall scrape failure from a live wall with no successful run")
	}
	if !strings.Contains(text, ">= 4") {
		t.Fatal("completion-rate alert has no minimum sample floor")
	}
	for _, forbidden := range []string{"run_id", "failure_id", "family_id", "trace_id", "goal_id"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("rules file uses identifier %s", forbidden)
		}
	}
}
