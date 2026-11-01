package agent_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

// The single strategist captures the request it ACTUALLY sends, rather than
// rebuilding an approximate prompt later from incomplete run telemetry.
func TestTrainingPromptsCaptureActualMessagesOnlyWhenEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected inference path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"teacher","choices":[{"finish_reason":"stop","message":{"content":"{\"choice\":1,\"intent\":\"heal for the next gym\"}"}}]}`)
	}))
	defer srv.Close()

	offered := []agent.Objective{{Kind: agent.KindHeal}}
	obs := agent.Observation{Round: 1, MapName: "PALLET_TOWN"}
	for _, enabled := range []bool{false, true} {
		planner := &agent.LLMPlanner{
			BaseURL: srv.URL, Model: "teacher", Goal: "Earn the first badge.",
			CaptureTrainingPrompts: enabled,
		}
		router := agent.NewFailoverPlanner(planner, nil)
		router.RecordChoiceMenus = enabled
		var calls []agent.LLMCall
		router.OnCall = func(call agent.LLMCall) { calls = append(calls, call) }

		got, err := router.Next(obs, offered)
		if err != nil || got.String() != offered[0].String() {
			t.Fatalf("enabled=%v: Next: got %s, err %v", enabled, got, err)
		}
		if len(calls) != 1 {
			t.Fatalf("enabled=%v: got %d calls", enabled, len(calls))
		}
		call := calls[0]
		if !enabled {
			if call.TrainingSystem != "" || call.TrainingUser != "" || len(call.OfferedObjectives) != 0 {
				t.Fatalf("default run captured private prompts/menus: %+v", call)
			}
			continue
		}
		if !strings.Contains(call.TrainingSystem, "Your goal: Earn the first badge.") ||
			!strings.Contains(call.TrainingUser, "1: heal the party") ||
			!strings.Contains(call.TrainingUser, "PALLET_TOWN") ||
			len(call.OfferedObjectives) != 1 || call.OfferedObjectives[0] != "heal the party" {
			t.Fatalf("lost exact prompt / menu: %+v", call)
		}
		if call.PromptHash == "" || call.Model != "teacher" {
			t.Fatalf("missing model provenance: %+v", call)
		}
	}
}
