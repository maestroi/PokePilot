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
			"decision_engine":{
				"backend":"jev",
				"deployment":"jev9b-local-8077",
				"mode":"active",
				"placements":true,
				"min_confidence":0.65,
				"max_choices":16,
				"inference":{
					"deployment_id":"jev9b-local-8077",
					"label":"JEV 9B · local 8077",
					"model_id":"jev9-local",
					"compute":"private-lan-host",
					"endpoint":"http://192.168.50.80:8077/v1",
					"api_model":"jev9-local",
					"token_env":"PRIVATE_JEV_TOKEN"
				}
			},
			"stats":{
				"decision_calls":12,
				"decision_fallbacks":2,
				"decision_avg_seconds":0.021,
				"decision_backend":"jev",
				"decision_model":"jev9-local",
				"decision_mode":"active",
				"decision_choice":"r1-c5",
				"decision_confidence":0.91,
				"decision_reference":"rotation 0, column 4",
				"decision_reference_agreed":false,
				"decision_reference_agreements":7,
				"decision_reference_disagreements":3,
				"decision_records":[
					{"kind":"tetris_placement","choice":"r1-c5","choice_label":"rotation 1, column 5","confidence":0.91,"duration_seconds":0.021,"backend":"jev","model":"jev9-local"}
				]
			},
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
		`"decision_engine":{"backend":"jev","mode":"active","deployment":"jev9b-local-8077","label":"JEV 9B · local 8077","model":"jev9-local","min_confidence":0.65,"max_choices":16}`,
		`"decision_calls":12`,
		`"decision_fallbacks":2`,
		`"decision_reference_agreements":7`,
		`"decision_reference_disagreements":3`,
		`"choice_label":"rotation 1, column 5"`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("public dashboard missing %s: %s", want, body)
		}
	}
	for _, forbidden := range []string{"private trace", "private detail", `"issue"`, "secret-engine", "private_backend", "hidden-piece", "private_piece_field", "192.168.50.80", "PRIVATE_JEV_TOKEN", "private-lan-host", `"endpoint"`, `"token_env"`, `"compute"`} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Errorf("public dashboard leaked %q: %s", forbidden, body)
		}
	}
}
