package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPRunAttemptsAndCheckpoints(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v1/runs/run-1/attempts":
			res.Write([]byte(`[{"attempt":1,"reason":"stuck","problems":[{"triage_key":"k1"}]},{"attempt":2,"reason":"done","problems":[]}]`)) //nolint:errcheck
		case "/v1/runs/run-1/checkpoints":
			if req.URL.Query().Get("attempt") != "1" {
				t.Errorf("attempt = %q, want 1", req.URL.Query().Get("attempt"))
			}
			res.Write([]byte(`{"run_id":"run-1","attempt":1,"checkpoints":[{"name":"r12","frame":900,"replayable":true,"question":"` + strings.Repeat("q", 2000) + `"}]}`)) //nolint:errcheck
		default:
			http.NotFound(res, req)
		}
	}))
	defer wall.Close()
	control := &mcpControl{wallBase: wall.URL, http: wall.Client()}

	_, attempts, err := control.getRunAttempts(context.Background(), nil, mcpRunInput{RunID: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := attempts["attempts"].([]map[string]any); len(list) != 2 || list[0]["reason"] != "stuck" {
		t.Fatalf("attempts = %#v", attempts["attempts"])
	}

	_, checkpoints, err := control.listCheckpoints(context.Background(), nil, mcpCheckpointsInput{RunID: "run-1", Attempt: 1})
	if err != nil {
		t.Fatal(err)
	}
	cp := checkpoints["checkpoints"].([]any)[0].(map[string]any)
	if cp["name"] != "r12" || len(cp["question"].(string)) > 260 {
		t.Fatalf("checkpoint = name %v, question %d bytes", cp["name"], len(cp["question"].(string)))
	}
}
