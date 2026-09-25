package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSpectatorDecisionFeedCrossesBoundaryWithoutRawErrors(t *testing.T) {
	var source spectatorSourceDashboard
	if err := json.Unmarshal([]byte(`{
		"runs":[{
			"run_id":"shadow-run",
			"status":"running",
			"stats":{
				"decision_mode":"shadow",
				"decision_battles_paused":true,
				"decision_model":"private-model",
				"decision_summary":{"kinds":{"battle_turn":{"calls":2,"errors":1,"shadow":2,"agreements":1}}},
				"decision_records":[
					{"kind":"battle_turn","choice":"move:0","choice_label":"use cut","confidence":0.8,"shadow":true,"executed":"use cut","agreed":true,"backend":"jev","prompt_tokens":99},
					{"kind":"battle_turn","shadow":true,"executed":"use fly","error":"agent: Jev decision HTTP 500: private body","error_kind":"backend"},
					{"kind":"battle_turn","shadow":true,"executed":"use fly","error":"legacy private error"}
				]
			}
		}]
	}`), &source); err != nil {
		t.Fatalf("decode source dashboard: %v", err)
	}
	body, err := json.Marshal(spectatorDashboard{Runs: []spectatorRun{source.Runs[0].spectatorRun}})
	if err != nil {
		t.Fatalf("marshal public dashboard: %v", err)
	}
	for _, want := range []string{
		`"decision_mode":"shadow"`,
		`"decision_battles_paused":true`,
		`"agreements":1`,
		`"choice_label":"use cut"`,
		`"agreed":true`,
		`"error_kind":"backend"`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("public dashboard missing %s: %s", want, body)
		}
	}
	if got := bytes.Count(body, []byte(`"error_kind":"backend"`)); got != 2 {
		t.Errorf("error_kind backend count = %d, want 2 (legacy error without a kind still reads as failed): %s", got, body)
	}
	for _, forbidden := range []string{"private body", "legacy private error", "private-model", `"error":`, "prompt_tokens", `"backend":"jev"`} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Errorf("public dashboard leaked %q: %s", forbidden, body)
		}
	}
}
