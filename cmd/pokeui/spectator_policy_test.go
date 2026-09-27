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
			"game":"tetris",
			"game_state":{
				"kind":"tetris",
				"mode":"type-a",
				"screen":"playing",
				"score":573,
				"lines_cleared":7,
				"level":0,
				"board":["..........","####..####"],
				"active":{"piece":"T","rotation":1,"x":3,"y":4,"private_piece_field":"hidden-piece"},
				"next":{"piece":"L","rotation":0},
				"private_backend":"secret-engine"
			},
			"fps":240,
			"play_style":"adventure",
			"purpose":"debug_coverage",
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
		`"game":"tetris"`,
		`"game_state":{"kind":"tetris"`,
		`"score":573`,
		`"lines_cleared":7`,
		`"piece":"T"`,
		`"fps":240`,
		`"play_style":"adventure"`,
		`"purpose":"debug_coverage"`,
		`"risk_tolerance":"cautious"`,
		`"wild_encounters":"fight"`,
		`"decision":"Explore Route 3"`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("public dashboard missing %s: %s", want, body)
		}
	}
	for _, forbidden := range []string{"private trace", "private detail", `"issue"`, "secret-engine", "private_backend", "hidden-piece", "private_piece_field"} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Errorf("public dashboard leaked %q: %s", forbidden, body)
		}
	}
}
