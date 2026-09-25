package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// A spectator started without -replay has no replay catalog, and
// publicSpectatorRuns then drops every finished run before it can look for a
// cached video. The public page therefore showed only live runs and the replay
// archive looked empty rather than unconfigured, which is how a missing
// -replay flag on the Swarm spectator went unnoticed while finishes with ready
// replays existed. The snapshot now states the archive's state outright.
func TestSpectatorSnapshotReportsReplayArchiveState(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(res, `{"now":1,"runs":[]}`)
	}))
	defer wall.Close()

	for _, tc := range []struct {
		name       string
		replayBase string
		want       bool
	}{
		{name: "disabled", replayBase: "", want: false},
		{name: "enabled", replayBase: "http://replay:8080", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := spectatorSnapshotWithReplay(wall.URL, newSpectatorReplayCatalog(tc.replayBase))
			rec := httptest.NewRecorder()
			handler(rec, httptest.NewRequest(http.MethodGet, "/v1/watch", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
			}
			var got spectatorDashboard
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode snapshot: %v", err)
			}
			if got.ReplayArchive != tc.want {
				t.Errorf("replay_archive = %t, want %t", got.ReplayArchive, tc.want)
			}
		})
	}
}

// The startup warning is the only thing that tells an operator the public
// archive is switched off, so lock its presence and its cause.
func TestSpectatorWithoutReplayWarnsAtStartup(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"replayBase == \"\"",
		"spectator mode without -replay",
		"finished runs and their cached replays are omitted from the public archive",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("pokeui main.go missing spectator replay warning fragment %q", want)
		}
	}
}
