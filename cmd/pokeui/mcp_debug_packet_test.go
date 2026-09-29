package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestPrepareDebugBuildsBoundedDeterministicPacket(t *testing.T) {
	identity := farm.FailureIdentity{
		Version: farm.FailureIdentityVersion,
		Game:    "pokemon",
		Adapter: "pokemon-red",
		Objective: farm.FailureObjective{
			Kind:  "go_to",
			Place: "route 9",
			Flee:  true,
		},
		Outcome: "blocked",
		Cause:   "navigation_stalled",
		Initial: farm.FailureState{Location: "cerulean city", X: 20, Y: 8, Controllable: true},
		Final:   farm.FailureState{Location: "route 9", X: 4, Y: 12, Controllable: true},
	}
	key, fingerprint, err := farm.FingerprintFailureIdentity(identity)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := "round-007-frame-0000012345-go-to-route-9.state"
	report := farm.FinishReport{
		RunID:         "run-debug",
		Attempt:       2,
		RunnerVersion: "deadbeef",
		Artifacts: []farm.Artifact{{
			Name: checkpoint, MediaType: "application/octet-stream",
			SHA256: strings.Repeat("a", 64), Data: []byte("state"),
		}},
	}
	reproArtifact, err := farm.NewFailureReproArtifact(report, farm.ObjectiveFailure{
		Objective: "go to route 9, fleeing wild battles", Error: "Travel: text box interrupted movement",
		Count: 1, FirstRound: 7, LastRound: 7, Key: key, Fingerprint: fingerprint,
		Identity: &identity, Build: "deadbeef", Outcome: identity.Outcome, Cause: identity.Cause,
		Checkpoint: checkpoint,
	})
	if err != nil {
		t.Fatal(err)
	}
	knowledge := strings.TrimSuffix(checkpoint, ".state") + ".knowledge-v1.json"

	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/v1/runs/run-debug/debug":
			_ = json.NewEncoder(res).Encode(map[string]any{
				"run": map[string]any{
					"run_id": "run-debug", "status": "done", "game": "pokemon-red", "planner": "llm",
					"goal": "badges:1", "attempts": 2, "error_attempts": 2,
					"map": 3, "x": 4, "y": 12, "circuit_key": "family-key",
				},
				"finish": map[string]any{
					"attempt": 2, "reason": "error", "detail": "objective failed",
					"runner_version": "deadbeef",
					"trace_tail": []string{"skill: Travel: text box interrupted movement"},
				},
				"artifacts": []map[string]any{
					{"name": checkpoint},
					{"name": knowledge},
					{"name": reproArtifact.Name},
					{"name": "run.gbrun"},
				},
				"timeline": []map[string]any{
					{"source": "llm", "kind": "decision", "message": strings.Repeat("ignored", 100)},
					{"source": "objective", "kind": "failure", "attempt": 2, "round": 7, "message": "go to route 9 failed"},
				},
			})
		case "/v1/triage":
			_ = json.NewEncoder(res).Encode([]map[string]any{{
				"key": "family-key", "fingerprint": "sha256:family", "count": 4,
				"run_ids": []string{"run-debug"},
				"issue": map[string]any{
					"issue_number": 42, "issue_url": "https://example.test/issues/42",
					"status": "open", "verification_state": "regressed",
					"solver_attempts": []map[string]any{
						{"backend": "old", "state": "no_pr", "note": "oldest"},
						{"backend": "qwen", "state": "no_pr", "note": "wrong layer"},
						{"backend": "cursor", "state": "pr_opened", "pr_number": 40},
						{"backend": "qwen", "state": "agent_failed", "note": "timeout"},
					},
				},
			}})
		case "/v1/runs/run-debug/artifacts/" + reproArtifact.Name + "/content":
			res.Header().Set("Content-Type", "application/json")
			_, _ = res.Write(reproArtifact.Data)
		default:
			http.NotFound(res, req)
		}
	}))
	t.Cleanup(wall.Close)

	control := &mcpControl{wallBase: wall.URL, artifactBase: wall.URL, http: wall.Client()}
	_, packet, err := control.prepareDebug(context.Background(), nil, mcpPrepareDebugInput{RunID: "run-debug"})
	if err != nil {
		t.Fatalf("prepareDebug: %v", err)
	}
	if packet.Version != farm.DebugPacketVersion || packet.RunID != "run-debug" || packet.Mode != "normal" {
		t.Fatalf("packet identity = %+v", packet)
	}
	if packet.ClassificationHint != "deterministic_defect_candidate" {
		t.Fatalf("classification = %q", packet.ClassificationHint)
	}
	if packet.Repro.ContractArtifact != reproArtifact.Name || packet.Repro.Checkpoint != checkpoint || packet.Repro.Knowledge != knowledge || !packet.Repro.Deterministic {
		t.Fatalf("repro = %+v", packet.Repro)
	}
	if packet.Triage.Key != "family-key" || packet.Triage.IssueNumber != 42 || !packet.Triage.Actionable {
		t.Fatalf("triage = %+v", packet.Triage)
	}
	if len(packet.Triage.SolverAttempts) != 3 || packet.Triage.SolverAttempts[0].Note != "wrong layer" {
		t.Fatalf("solver attempts = %+v", packet.Triage.SolverAttempts)
	}
	if packet.Failure.Cause != "navigation_stalled" || packet.Failure.Objective == "" {
		t.Fatalf("failure = %+v", packet.Failure)
	}
	if len(packet.SearchTerms) == 0 || len(packet.SearchTerms) > 8 {
		t.Fatalf("search terms = %#v", packet.SearchTerms)
	}
	if len(packet.Evidence) != 1 || packet.Evidence[0].Kind != "failure" {
		t.Fatalf("evidence = %+v", packet.Evidence)
	}
}

func TestPrepareDebugRejectsUnknownMode(t *testing.T) {
	control := &mcpControl{}
	if _, _, err := control.prepareDebug(context.Background(), nil, mcpPrepareDebugInput{RunID: "run-x", Mode: "huge"}); err == nil {
		t.Fatal("expected invalid mode error")
	}
}
