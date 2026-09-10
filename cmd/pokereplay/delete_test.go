package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
)

func TestArtifactDeletePurgesAllAttemptsAndReplayCaches(t *testing.T) {
	const runPrefix = "runs/run-1-abc123/"
	var deleted []string
	s3srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/pokepilot" || r.URL.Query().Get("prefix") != runPrefix {
				t.Fatalf("unexpected list: %s %s", r.Method, r.URL.String())
			}
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprintf(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>%sattempt-1/run.gbrun</Key></Contents><Contents><Key>%sattempt-1/replay-old.mp4</Key></Contents><Contents><Key>%sattempt-2/run.gbrun</Key></Contents><Contents><Key>%sattempt-2/replay-new.mp4</Key></Contents></ListBucketResult>`, runPrefix, runPrefix, runPrefix, runPrefix)
		case http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected S3 request: %s %s", r.Method, r.URL.String())
		}
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
		if r.Method != http.MethodGet || r.URL.Path != "/v1/runs/run-1/artifacts" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(artifactList{
			RunID: "run-1", Attempt: 2,
			Artifacts: []artifactRef{{
				Name: "run.gbrun", Store: "s3", Bucket: "pokepilot",
				ObjectKey: runPrefix + "attempt-2/run.gbrun", Replayable: true,
			}},
		})
	}))
	defer wall.Close()

	replay := newReplayServer(wall.URL, "", "", store)
	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/run-1/artifacts", nil)
	req.SetPathValue("id", "run-1")
	res := httptest.NewRecorder()
	replay.handleArtifactDelete(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("DELETE artifacts = %d, body=%s", res.Code, res.Body.String())
	}
	want := []string{
		"/pokepilot/runs/run-1-abc123/attempt-1/run.gbrun",
		"/pokepilot/runs/run-1-abc123/attempt-1/replay-old.mp4",
		"/pokepilot/runs/run-1-abc123/attempt-2/run.gbrun",
		"/pokepilot/runs/run-1-abc123/attempt-2/replay-new.mp4",
	}
	if !reflect.DeepEqual(deleted, want) {
		t.Fatalf("deleted = %#v, want %#v", deleted, want)
	}
}

func TestArtifactDeleteRequiresConfiguredStoreForRemoteArtifacts(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(artifactList{
			RunID: "run-1", Attempt: 1,
			Artifacts: []artifactRef{{Name: "run.gbrun", Store: "s3", Bucket: "pokepilot", ObjectKey: "runs/run-1/attempt-1/run.gbrun", Replayable: true}},
		})
	}))
	defer wall.Close()

	replay := newReplayServer(wall.URL, "", "", nil)
	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/run-1/artifacts", nil)
	req.SetPathValue("id", "run-1")
	res := httptest.NewRecorder()
	replay.handleArtifactDelete(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("DELETE artifacts = %d, want 503; body=%s", res.Code, res.Body.String())
	}
}

func TestRecordingRunPrefixRejectsBroadOrMalformedKeys(t *testing.T) {
	cases := map[string]bool{
		"runs/run-abc/attempt-1/run.gbrun":    true,
		"runs/run-abc/attempt-12/run.gbrun":   true,
		"runs/run.gbrun":                      false,
		"runs/run-abc/run.gbrun":              false,
		"runs/run-abc/attempt-x/run.gbrun":    false,
		"other/run-abc/attempt-1/run.gbrun":   false,
		"runs/a/b/attempt-1/run.gbrun":        false,
		"runs/run-abc/attempt-1/../run.gbrun": false,
	}
	for key, wantOK := range cases {
		prefix, ok := recordingRunPrefix(key)
		if ok != wantOK {
			t.Errorf("recordingRunPrefix(%q) = %q,%v, want ok=%v", key, prefix, ok, wantOK)
		}
	}
}
