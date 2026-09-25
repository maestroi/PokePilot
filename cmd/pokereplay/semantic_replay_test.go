package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
)

func TestSemanticReplayCacheKeyTracksRecordingSet(t *testing.T) {
	recordings := []replayRecording{{
		Attempt: 1,
		Artifact: artifactRef{
			Name: "run.gbrun", SHA256: strings.Repeat("a", 64),
			Store: "s3", ObjectKey: "runs/run-1/attempt-1/run.gbrun", Replayable: true,
		},
	}}
	first := semanticReplayCacheKey("run-1", recordings)
	if !strings.HasPrefix(first, "runs/run-1/attempt-1/semantic-replay-v1-") || !strings.HasSuffix(first, ".json") {
		t.Fatalf("semantic cache key = %q", first)
	}
	recordings[0].Artifact.SHA256 = strings.Repeat("b", 64)
	second := semanticReplayCacheKey("run-1", recordings)
	if first == second {
		t.Fatal("recording identity change did not change semantic cache key")
	}
}

func TestReplayFrameTimingIncludesEncodedInitialFrame(t *testing.T) {
	if got := replayFrameTimeMS(0); got != 0 {
		t.Fatalf("zero frames = %dms", got)
	}
	if got := replayFrameTimeMS(60); got < 1000 || got > 1006 {
		t.Fatalf("60 frames = %dms, want about one second", got)
	}
}

func TestReplaySemanticEndpointServesCachedTimeline(t *testing.T) {
	recording := artifactRef{
		Name: "run.gbrun", SHA256: strings.Repeat("c", 64),
		Store: "s3", Bucket: "pokepilot", ObjectKey: "runs/run-semantic/attempt-1/run.gbrun",
		Replayable: true,
	}
	key := semanticReplayCacheKey("run-semantic", []replayRecording{{Attempt: 1, Artifact: recording}})
	const payload = `{"version":1,"schema_version":1,"frames_per_second":59.7275,"duration_ms":0,"samples":[]}`

	s3srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/pokepilot/"+key {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payload)
	}))
	defer s3srv.Close()

	store, err := artifactstore.NewS3(artifactstore.S3Config{
		Endpoint: s3srv.URL, Bucket: "pokepilot", Region: "us-east-1",
		AccessKey: "test", SecretKey: "secret", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/run-semantic/artifacts" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(artifactList{
			RunID: "run-semantic", Attempt: 1, Artifacts: []artifactRef{recording},
		})
	}))
	defer wall.Close()

	replay := newReplayServer(wall.URL, "", "", store)
	srv := httptest.NewServer(replay.handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/v1/runs/run-semantic/replay/semantic")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("semantic status = %d body=%s", res.StatusCode, body)
	}
	if string(body) != payload {
		t.Fatalf("semantic body = %q", body)
	}
	if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("semantic Content-Type = %q", got)
	}
}
