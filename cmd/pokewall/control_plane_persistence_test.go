package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestControlPlanePersistPayloadIgnoresHeartbeatTelemetry(t *testing.T) {
	w := NewWall("")
	w.mu.Lock()
	w.order = []string{"run-1"}
	w.tiles["run-1"] = &Tile{
		RunID:            "run-1",
		Status:           statusRunning,
		Planner:          "llm",
		Starter:          "bulbasaur",
		Goal:             "earn the Boulder Badge",
		LLMDeployment:    "gpu-9b",
		Seed:             42,
		FPS:              60,
		QueuedAt:         time.Unix(1_700_000_000, 0),
		Attempts:         1,
		RecoveryProfile:  farm.RecoveryProfileResilient,
		RecoveryAttempts: 2,
		RecoveryBadges:   3,
		RecoveryEvents:   20,
		RecoveryMaps:     40,
		Activity: []runActivityEvent{
			{Source: "llm", Kind: "decision", Summary: "heartbeat-activity-marker"},
			{Source: "recovery", Kind: "rollback", Summary: "recovery-activity-marker"},
		},
		Frame:     12345,
		Map:       5,
		X:         11,
		Y:         4,
		Trace:     strings.Repeat("heartbeat-trace-marker", 1024),
		Question:  "heartbeat-question-marker",
		Decision:  "heartbeat-decision-marker",
		StopSoFar: "heartbeat-stop-marker",
		Stats: &farm.LLMStats{
			Round:    7,
			PlanGoal: strings.Repeat("heartbeat-plan-marker", 1024),
		},
		Player: &farm.Player{
			Money: 9999,
			Party: []farm.PartyMon{{Name: "PIKACHU", Level: 42, HP: 100, MaxHP: 100}},
			Bag:   []farm.BagItem{{Name: "POKE BALL", Quantity: 99}},
		},
		workerAddrs: []string{"runner-heartbeat-marker:8099"},
	}
	w.mu.Unlock()

	before, err := captureControlPlanePersistPayload(w)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"heartbeat-trace-marker", "heartbeat-question-marker", "heartbeat-decision-marker",
		"heartbeat-stop-marker", "heartbeat-plan-marker", "PIKACHU", "runner-heartbeat-marker", "heartbeat-activity-marker",
	} {
		if bytes.Contains(before.stateRaw, []byte(marker)) {
			t.Fatalf("control-plane recovery snapshot retained heartbeat telemetry marker %q", marker)
		}
	}
	if !bytes.Contains(before.stateRaw, []byte("recovery-activity-marker")) {
		t.Fatal("control-plane recovery snapshot dropped durable recovery activity")
	}

	w.mu.Lock()
	tile := w.tiles["run-1"]
	tile.Frame = 99999
	tile.Map = 12
	tile.X = 22
	tile.Y = 33
	tile.Trace = "different trace"
	tile.Question = "different question"
	tile.Decision = "different decision"
	tile.StopSoFar = "different stop"
	tile.Stats = &farm.LLMStats{Round: 99, PlanGoal: "different plan"}
	tile.Player = &farm.Player{Money: 1, Party: []farm.PartyMon{{Name: "MEW", Level: 100}}}
	tile.workerAddrs = []string{"different-runner:8099"}
	tile.Activity[0].Summary = "different heartbeat activity"
	tile.Activity = append(tile.Activity, runActivityEvent{Source: "skill", Kind: "execution", Summary: "different skill activity"})
	w.mu.Unlock()

	afterHeartbeat, err := captureControlPlanePersistPayload(w)
	if err != nil {
		t.Fatal(err)
	}
	if before.stateHash != afterHeartbeat.stateHash {
		t.Fatal("heartbeat-only telemetry changed the durable control-plane recovery snapshot")
	}

	w.mu.Lock()
	tile.Attempts++
	w.mu.Unlock()
	afterRecoveryChange, err := captureControlPlanePersistPayload(w)
	if err != nil {
		t.Fatal(err)
	}
	if before.stateHash == afterRecoveryChange.stateHash {
		t.Fatal("attempt/recovery mutation did not change the durable control-plane snapshot")
	}
}

func TestControlPlanePersistedStateRestoresRetryStateWithoutTelemetry(t *testing.T) {
	w := NewWall("")
	w.mu.Lock()
	w.order = []string{"retry-run"}
	w.queue = []string{"retry-run"}
	w.tiles["retry-run"] = &Tile{
		RunID:            "retry-run",
		Status:           statusQueued,
		Planner:          "llm",
		Starter:          "squirtle",
		Goal:             "earn the Cascade Badge",
		LLMDeployment:    "gpu-9b",
		Seed:             1234,
		FPS:              60,
		MaxRounds:        50,
		MaxFrames:        1_000_000,
		Endless:          true,
		RandomSeed:       true,
		QueuedAt:         time.Unix(1_700_000_123, 0),
		Attempts:         4,
		ErrorAttempts:    2,
		LossRecoveries:   1,
		RecoveryProfile:  farm.RecoveryProfileResilient,
		RecoveryAttempts: 3,
		RecoveryBadges:   4,
		RecoveryEvents:   31,
		RecoveryMaps:     74,
		Activity: []runActivityEvent{
			{Source: "recovery", Kind: "rollback", Summary: "rollback to badge 3"},
			{Source: "system", Kind: "leased", Summary: "attempt 5 leased"},
			{Source: "llm", Kind: "decision", Summary: "this live breadcrumb should be filtered"},
		},
		Detail:          "attempt 4 failed: no heartbeat for 31s",
		ResumeFromRunID: "parent-run",
		Frame:           888,
		Trace:           "must not survive postgres restart",
		Stats:           &farm.LLMStats{Round: 8, PlanGoal: "must not survive"},
		Player:          &farm.Player{Money: 500},
		workerAddrs:     []string{"old-runner:8099"},
	}
	w.mu.Unlock()

	payload, err := captureControlPlanePersistPayload(w)
	if err != nil {
		t.Fatal(err)
	}
	var persisted persistedState
	if err := json.Unmarshal(payload.stateRaw, &persisted); err != nil {
		t.Fatal(err)
	}

	restored := NewWall("")
	restorePersistedState(restored, persisted)
	restored.mu.Lock()
	defer restored.mu.Unlock()
	if len(restored.queue) != 1 || restored.queue[0] != "retry-run" {
		t.Fatalf("restored queue = %v, want [retry-run]", restored.queue)
	}
	tile := restored.tiles["retry-run"]
	if tile == nil {
		t.Fatal("retry-run missing after restore")
	}
	if tile.Status != statusQueued || tile.Attempts != 4 || tile.ErrorAttempts != 2 || tile.LossRecoveries != 1 {
		t.Fatalf("restored recovery state = status %q attempts %d error_attempts %d loss_recoveries %d", tile.Status, tile.Attempts, tile.ErrorAttempts, tile.LossRecoveries)
	}
	if tile.RecoveryProfile != farm.RecoveryProfileResilient || tile.RecoveryAttempts != 3 || tile.RecoveryBadges != 4 || tile.RecoveryEvents != 31 || tile.RecoveryMaps != 74 {
		t.Fatalf("restored resilient state = profile %q attempts %d frontier %d/%d/%d", tile.RecoveryProfile, tile.RecoveryAttempts, tile.RecoveryBadges, tile.RecoveryEvents, tile.RecoveryMaps)
	}
	if len(tile.Activity) != 2 || tile.Activity[0].Source != "recovery" || tile.Activity[1].Source != "system" {
		t.Fatalf("restored durable activity = %+v", tile.Activity)
	}
	if tile.Seed != 1234 || tile.LLMDeployment != "gpu-9b" || tile.ResumeFromRunID != "parent-run" || tile.Detail == "" {
		t.Fatalf("restored scheduler fields incomplete: %+v", tile)
	}
	if tile.Frame != 0 || tile.Trace != "" || tile.Stats != nil || tile.Player != nil || len(tile.workerAddrs) != 0 {
		t.Fatalf("heartbeat telemetry unexpectedly survived control-plane restore: %+v", tile)
	}
}
