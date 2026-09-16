package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestSendFinalHeartbeatPublishesSettledSnapshot(t *testing.T) {
	var got farm.Heartbeat
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/runs/{id}/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode heartbeat: %v", err)
		}
		_ = json.NewEncoder(w).Encode(farm.HeartbeatReply{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := farm.NewClient(srv.URL)
	client.Version = "test-runner"
	hb := farm.Heartbeat{
		RunID:    "badge-finish",
		Frame:    158450,
		Map:      0x36,
		X:        4,
		Y:        2,
		Question: "defeat Brock to earn the Boulder Badge",
		Decision: "defeat Brock to earn the Boulder Badge",
		Stats: &farm.LLMStats{
			GoalSummary:  "badges 1/1",
			GoalCurrent:  1,
			GoalTarget:   1,
			GoalComplete: true,
		},
		Player: &farm.Player{
			Badges: []string{"Boulder"},
			Party:  []farm.PartyMon{{Name: "squirtle", Level: 15, HP: 31, MaxHP: 40}},
		},
	}

	sendFinalHeartbeat(client, hb)

	if got.RunID != hb.RunID || got.Frame != hb.Frame || got.Map != hb.Map || got.X != hb.X || got.Y != hb.Y {
		t.Fatalf("final heartbeat position = %+v, want %+v", got, hb)
	}
	if got.Version != "test-runner" {
		t.Fatalf("runner version = %q, want test-runner", got.Version)
	}
	if got.Stats == nil || !got.Stats.GoalComplete || got.Stats.GoalCurrent != 1 || got.Stats.GoalTarget != 1 {
		t.Fatalf("final goal stats = %+v", got.Stats)
	}
	if got.Player == nil || len(got.Player.Badges) != 1 || got.Player.Badges[0] != "Boulder" {
		t.Fatalf("final player = %+v, want Boulder badge", got.Player)
	}
}

func TestRunOneFlushesSettledHeartbeatBeforeFinish(t *testing.T) {
	src, err := os.ReadFile("farm.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	anchor := strings.Index(text, "// The objective that satisfies a deterministic goal")
	if anchor < 0 {
		t.Fatal("runOne is missing the settled-state heartbeat refresh")
	}
	tail := text[anchor:]
	refresh := strings.Index(tail, "sampleHeartbeat(m, spec.RunID, snap, mem, addrs, trail)")
	join := strings.Index(tail, "<-hbDone")
	flush := strings.Index(tail, "sendFinalHeartbeat(client, snap.load())")
	finish := strings.Index(tail, "finishRunWithRecording(m, client, spec")
	if refresh < 0 || join < 0 || flush < 0 || finish < 0 {
		t.Fatalf("missing final heartbeat sequence: refresh=%d join=%d flush=%d finish=%d", refresh, join, flush, finish)
	}
	if !(refresh < join && join < flush && flush < finish) {
		t.Fatalf("final heartbeat order is wrong: refresh=%d join=%d flush=%d finish=%d", refresh, join, flush, finish)
	}
}
