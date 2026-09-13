package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSpectatorPresentationPolicyCrossesPublicBoundary(t *testing.T) {
	var source spectatorSourceDashboard
	if err := json.Unmarshal([]byte(`{
		"now":123,
		"runs":[{
			"run_id":"themed-run",
			"status":"running",
			"fps":240,
			"play_style":"adventure",
			"risk_tolerance":"cautious",
			"wild_encounters":"fight",
			"decision":"Explore Route 3",
			"trace":"private trace",
			"detail":"private detail",
			"issue":{"number":99}
		}]
	}`), &source); err != nil {
		t.Fatalf("decode source dashboard: %v", err)
	}
	if len(source.Runs) != 1 {
		t.Fatalf("source runs = %d, want 1", len(source.Runs))
	}

	body, err := json.Marshal(spectatorDashboard{
		Now:  source.Now,
		Runs: []spectatorRun{source.Runs[0].spectatorRun},
	})
	if err != nil {
		t.Fatalf("marshal public dashboard: %v", err)
	}

	for _, want := range []string{
		`"fps":240`,
		`"play_style":"adventure"`,
		`"risk_tolerance":"cautious"`,
		`"wild_encounters":"fight"`,
		`"decision":"Explore Route 3"`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("public dashboard missing %s: %s", want, body)
		}
	}
	for _, forbidden := range []string{"private trace", "private detail", `"issue"`} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Errorf("public dashboard leaked %q: %s", forbidden, body)
		}
	}
}
