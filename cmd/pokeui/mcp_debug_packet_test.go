package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
					"trace_tail":     []string{"skill: Travel: text box interrupted movement"},
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

// debugAttemptRecorder captures the wall selectors prepareDebug used, so the
// test can prove the packet was scoped to the failing attempt instead of the
// run's latest one.
type debugAttemptRecorder struct {
	mu        sync.Mutex
	debug     []string
	content   []string
	attemptsQ int
}

func (r *debugAttemptRecorder) recordDebug(attempt string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.debug = append(r.debug, attempt)
}

func (r *debugAttemptRecorder) recordContent(attempt string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.content = append(r.content, attempt)
}

func (r *debugAttemptRecorder) recordAttemptsList() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attemptsQ++
}

func (r *debugAttemptRecorder) snapshot() (debug, content []string, attemptsQ int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.debug...), append([]string(nil), r.content...), r.attemptsQ
}

// The failure evidence is scoped to the attempt that failed, resolved from a
// triage key or an explicit attempt, while omitting both keeps the historical
// latest-attempt behaviour.
func TestPrepareDebugScopesEvidenceToFailingAttempt(t *testing.T) {
	const runID = "run-debug"
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
	const failingAttempt = 23
	checkpoint := "round-066-frame-0001053404-progress-mt-moon-fossil-acquired.state"
	report := farm.FinishReport{
		RunID:         runID,
		Attempt:       failingAttempt,
		RunnerVersion: "deadbeef",
		Artifacts: []farm.Artifact{{
			Name: checkpoint, MediaType: "application/octet-stream",
			SHA256: strings.Repeat("a", 64), Data: []byte("state"),
		}},
	}
	reproArtifact, err := farm.NewFailureReproArtifact(report, farm.ObjectiveFailure{
		Objective: "go to route 9, fleeing wild battles", Error: "Travel: text box interrupted movement",
		Count: 1, FirstRound: 66, LastRound: 66, Key: key, Fingerprint: fingerprint,
		Identity: &identity, Build: "deadbeef", Outcome: identity.Outcome, Cause: identity.Cause,
		Checkpoint: checkpoint,
	})
	if err != nil {
		t.Fatal(err)
	}
	knowledge := strings.TrimSuffix(checkpoint, ".state") + ".knowledge-v6.json"

	recorder := &debugAttemptRecorder{}
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/v1/runs/" + runID + "/attempts":
			recorder.recordAttemptsList()
			_ = json.NewEncoder(res).Encode([]map[string]any{
				{"attempt": 7, "problems": []map[string]any{{"triage_key": "6d93c65304541ee1"}}},
				{"attempt": 10, "problems": []map[string]any{{"triage_key": "a59bfe7d99bbbdef"}}},
				{"attempt": failingAttempt, "problems": []map[string]any{{"triage_key": "b2321b805045a1ea"}}},
			})
		case "/v1/runs/" + runID + "/debug":
			recorder.recordDebug(req.URL.Query().Get("attempt"))
			_ = json.NewEncoder(res).Encode(map[string]any{
				"run": map[string]any{
					"run_id": runID, "status": "done", "game": "pokemon-red", "planner": "llm",
					"goal": "badges:1", "attempts": 56, "error_attempts": failingAttempt,
					"map": 3, "x": 4, "y": 12, "circuit_key": "family-key",
				},
				"finish": map[string]any{
					"attempt": failingAttempt, "reason": "error", "detail": "objective failed",
					"runner_version": "deadbeef",
					"trace_tail":     []string{"skill: Travel: text box interrupted movement"},
				},
				"artifacts": []map[string]any{
					{"name": checkpoint},
					{"name": knowledge},
					{"name": reproArtifact.Name},
					{"name": "run.gbrun"},
				},
				"timeline": []map[string]any{
					{"source": "objective", "kind": "failure", "attempt": failingAttempt, "round": 66, "message": "go to route 9 failed"},
				},
			})
		case "/v1/triage":
			_ = json.NewEncoder(res).Encode([]map[string]any{})
		case "/v1/runs/" + runID + "/artifacts/" + reproArtifact.Name + "/content":
			recorder.recordContent(req.URL.Query().Get("attempt"))
			_, _ = res.Write(reproArtifact.Data)
		default:
			http.NotFound(res, req)
		}
	}))
	t.Cleanup(wall.Close)

	control := &mcpControl{wallBase: wall.URL, artifactBase: wall.URL, http: wall.Client()}

	// A triage key resolves to the attempt that carried that problem key.
	_, packet, err := control.prepareDebug(context.Background(), nil, mcpPrepareDebugInput{RunID: runID, Key: "b2321b805045a1ea"})
	if err != nil {
		t.Fatalf("prepareDebug(key): %v", err)
	}
	if packet.Repro.Attempt != failingAttempt {
		t.Fatalf("repro attempt = %d, want %d", packet.Repro.Attempt, failingAttempt)
	}
	if !packet.Repro.Deterministic || packet.Repro.ContractArtifact == "" || packet.Repro.Checkpoint != checkpoint || packet.Repro.Knowledge != knowledge {
		t.Fatalf("repro = %+v", packet.Repro)
	}
	if debug, content, attemptsQ := recorder.snapshot(); len(debug) != 1 || debug[0] != "23" || len(content) != 1 || content[0] != "23" || attemptsQ != 1 {
		t.Fatalf("key selectors: debug=%v content=%v attemptsQ=%d", debug, content, attemptsQ)
	}

	// An explicit attempt wins over the key and needs no /attempts lookup.
	_, explicit, err := control.prepareDebug(context.Background(), nil, mcpPrepareDebugInput{RunID: runID, Attempt: 7, Key: "b2321b805045a1ea"})
	if err != nil {
		t.Fatalf("prepareDebug(attempt): %v", err)
	}
	if explicit.Repro.Attempt != 7 {
		t.Fatalf("explicit repro attempt = %d, want 7", explicit.Repro.Attempt)
	}
	if debug, _, attemptsQ := recorder.snapshot(); len(debug) != 2 || debug[1] != "7" || attemptsQ != 1 {
		t.Fatalf("explicit selectors: debug=%v attemptsQ=%d", debug, attemptsQ)
	}

	// Without key/attempt the historical latest-attempt behaviour is unchanged.
	_, latest, err := control.prepareDebug(context.Background(), nil, mcpPrepareDebugInput{RunID: runID})
	if err != nil {
		t.Fatalf("prepareDebug(latest): %v", err)
	}
	if latest.Repro.Attempt != 0 {
		t.Fatalf("latest repro attempt = %d, want 0", latest.Repro.Attempt)
	}
	if !latest.Repro.Deterministic || latest.Repro.ContractArtifact == "" {
		t.Fatalf("latest repro = %+v", latest.Repro)
	}
	if debug, content, attemptsQ := recorder.snapshot(); len(debug) != 3 || debug[2] != "" || len(content) != 3 || content[2] != "" || attemptsQ != 1 {
		t.Fatalf("latest selectors: debug=%v content=%v attemptsQ=%d", debug, content, attemptsQ)
	}
}
