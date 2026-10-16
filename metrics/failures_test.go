package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestFailureTypesAreBounded(t *testing.T) {
	tests := map[string]string{
		"route_prerequisite_missing":  "prerequisite",
		"combat_defeat":               "combat",
		"field_roster_catch_failed":   "field_move",
		"stagnation":                  "stagnation",
		"the entire prompt / run=abc": "other",
		"truly_new_cause_982374":      "other",
	}
	for in, want := range tests {
		if got := NormalizeFailureType(in); got != want {
			t.Errorf("NormalizeFailureType(%q) = %q, want %q", in, got, want)
		}
	}
	if got := NormalizeFailureReason("ERROR: run 123"); got != "other" {
		t.Fatalf("unbounded reason = %q", got)
	}
}

func TestFailureCountersAndHistogram(t *testing.T) {
	reg := Must(Options{Service: ServiceWall, Revision: "dev"})
	f, err := NewFailures(reg)
	if err != nil {
		t.Fatal(err)
	}
	f.Objective("pokemon-gold", "route_prerequisite_missing", 5, 2, 1)
	f.Objective("pokemon-gold", "trace_id=secret-payload", 1, 1, 0)
	f.Objective("pokemon-gold", "combat_defeat", 0, 0, 0)
	f.Retry("pokemon-gold", "error")
	f.SupervisorRecovered("pokemon-gold")
	start := time.Unix(1_700_000_000, 0)
	f.TerminalFailure("pokemon-gold", "stuck", start, start.Add(4*time.Hour))
	f.TerminalFailure("pokemon-gold", "stuck", time.Time{}, start)

	body := scrape(t, reg.Handler())
	for _, want := range []string{
		`pokepilot_run_failures_total{failure_type="prerequisite",game="pokemon-gold",outcome="recovered"} 2`,
		`pokepilot_run_failures_total{failure_type="prerequisite",game="pokemon-gold",outcome="terminal"} 1`,
		`pokepilot_run_failures_total{failure_type="prerequisite",game="pokemon-gold",outcome="unresolved"} 2`,
		`pokepilot_failure_recoveries_total{failure_type="prerequisite",game="pokemon-gold"} 2`,
		`pokepilot_failure_recovery_attempts_total{game="pokemon-gold",reason="error"} 1`,
		`pokepilot_failure_supervisor_recoveries_total{game="pokemon-gold"} 1`,
		`pokepilot_failure_time_to_failure_seconds_count{game="pokemon-gold",reason="stuck"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "secret-payload") || strings.Contains(body, "trace_id=") {
		t.Fatalf("unbounded cause leaked into labels:\n%s", body)
	}
}
