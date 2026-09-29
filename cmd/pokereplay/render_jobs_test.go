package main

import (
	"context"
	"errors"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
)

func TestMediaRenderJobLeaseLostRecognizesControlCancellation(t *testing.T) {
	for _, message := range []string{
		"media render job is owned by another worker",
		"media render job is not active (state cancelled)",
		"media render job not found",
	} {
		if !mediaRenderJobLeaseLost(errors.New(message)) {
			t.Fatalf("lease error %q was not recognized", message)
		}
	}
	if mediaRenderJobLeaseLost(errors.New("temporary network timeout")) {
		t.Fatal("transient error incorrectly treated as lost lease")
	}
}

func TestRecordingsForAttemptsUsesPersistedAttemptSet(t *testing.T) {
	var mu sync.Mutex
	var requested []string
	latestRequested := false
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/runs/run-1/artifacts" {
			http.NotFound(w, r)
			return
		}
		attempt := r.URL.Query().Get("attempt")
		if attempt == "" {
			mu.Lock()
			latestRequested = true
			mu.Unlock()
			http.Error(w, "latest lookup must not be used for durable recovery", http.StatusInternalServerError)
			return
		}
		mu.Lock()
		requested = append(requested, attempt)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(artifactList{
			RunID: "run-1",
			Artifacts: []artifactRef{{
				Name:       "run.gbrun",
				SHA256:     strings.Repeat(attempt, 64),
				Store:      "s3",
				Bucket:     "pokepilot",
				ObjectKey:  "runs/run-1/attempt-" + attempt + "/run.gbrun",
				Replayable: true,
			}},
		})
	}))
	defer wall.Close()

	replay := newReplayServer(wall.URL, "", "", nil)
	recordings, err := replay.recordingsForAttempts(context.Background(), "run-1", []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(recordings) != 2 {
		t.Fatalf("recordings=%d, want 2", len(recordings))
	}
	if got := []int{recordings[0].Attempt, recordings[1].Attempt}; !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("recording attempts=%v, want [1 2]", got)
	}
	mu.Lock()
	gotRequests := append([]string(nil), requested...)
	gotLatest := latestRequested
	mu.Unlock()
	if gotLatest {
		t.Fatal("durable recovery consulted the run's latest attempt set")
	}
	if !reflect.DeepEqual(gotRequests, []string{"1", "2"}) {
		t.Fatalf("artifact requests=%v, want persisted attempts [1 2]", gotRequests)
	}
}

func TestRunRenderJobRecoveryClaimsPersistedJobOnStartup(t *testing.T) {
	job := farm.MediaRenderJob{
		Version:     farm.MediaRenderJobVersion,
		ID:          "render-recovery-test",
		Identity:    "identity",
		RunID:       "run-1",
		Attempts:    []int{1},
		Mode:        "definitely-not-a-replay-mode",
		ArtifactKey: "artifact",
		State:       farm.MediaRenderJobQueued,
		Stage:       farm.MediaRenderJobQueued,
	}
	finished := make(chan farm.MediaRenderJobFinishRequest, 1)
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/media/render-jobs" && r.URL.Query().Get("claimable") == "1":
			_ = json.NewEncoder(w).Encode(map[string]any{"jobs": []farm.MediaRenderJob{job}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/media/render-jobs/"+job.ID+"/claim":
			var req farm.MediaRenderJobClaimRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode claim: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			claimed := job
			claimed.State = farm.MediaRenderJobPreparing
			claimed.Stage = farm.MediaRenderJobPreparing
			claimed.WorkerID = req.WorkerID
			_ = json.NewEncoder(w).Encode(claimed)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/media/render-jobs/"+job.ID+"/finish":
			var req farm.MediaRenderJobFinishRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode finish: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			finished <- req
			done := job
			done.State = req.State
			done.Stage = req.Stage
			done.LastError = req.LastError
			_ = json.NewEncoder(w).Encode(done)
		default:
			http.NotFound(w, r)
		}
	}))
	defer wall.Close()

	replay := newReplayServer(wall.URL, "", "", &artifactstore.S3{})
	ctx, cancel := context.WithCancel(context.Background())
	recoveryDone := make(chan struct{})
	go func() {
		defer close(recoveryDone)
		replay.runRenderJobRecovery(ctx, time.Hour)
	}()

	select {
	case req := <-finished:
		if req.State != farm.MediaRenderJobFailed || req.LastError == "" {
			t.Fatalf("recovered finish=%+v, want failed with parse error", req)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("startup recovery did not claim the persisted job")
	}
	cancel()
	select {
	case <-recoveryDone:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery loop did not stop")
	}
}

func TestReplayRenderReadyArtifactShortCircuitsAndReconcilesJob(t *testing.T) {
	recordingSHA := strings.Repeat("ab", 32)
	artifact := artifactRef{
		Name:       "run.gbrun",
		MediaType:  "application/octet-stream",
		SHA256:     recordingSHA,
		Store:      "s3",
		Bucket:     "pokepilot",
		ObjectKey:  "runs/run-1/attempt-1/run.gbrun",
		Replayable: true,
	}

	var cacheKey, jobID string
	var mu sync.Mutex
	createCalls := 0
	reconcileCalls := 0

	s3srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/pokepilot/"+cacheKey {
			w.Header().Set("Content-Length", "8")
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer s3srv.Close()
	store, err := artifactstore.NewS3(artifactstore.S3Config{
		Endpoint:  s3srv.URL,
		Bucket:    "pokepilot",
		Region:    "us-east-1",
		AccessKey: "test",
		SecretKey: "secret",
		Timeout:   2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/run-1/artifacts":
			_ = json.NewEncoder(w).Encode(artifactList{
				RunID:     "run-1",
				Attempt:   1,
				Artifacts: []artifactRef{artifact},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/media/render-jobs/"+jobID:
			_ = json.NewEncoder(w).Encode(farm.MediaRenderJob{
				Version:     farm.MediaRenderJobVersion,
				ID:          jobID,
				Identity:    cacheKey,
				RunID:       "run-1",
				Attempts:    []int{1},
				Mode:        string(replayModeRaw),
				ArtifactKey: cacheKey,
				State:       farm.MediaRenderJobUploading,
				Stage:       farm.MediaRenderJobUploading,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/media/render-jobs/"+jobID+"/reconcile-ready":
			_, _ = io.Copy(io.Discard, r.Body)
			mu.Lock()
			reconcileCalls++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(farm.MediaRenderJob{
				Version:     farm.MediaRenderJobVersion,
				ID:          jobID,
				Identity:    cacheKey,
				RunID:       "run-1",
				Attempts:    []int{1},
				Mode:        string(replayModeRaw),
				ArtifactKey: cacheKey,
				State:       farm.MediaRenderJobReady,
				Stage:       farm.MediaRenderJobReady,
				ResultSize:  8,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/media/render-jobs":
			mu.Lock()
			createCalls++
			mu.Unlock()
			http.Error(w, "ready artifacts must not create jobs", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer wall.Close()

	replay := newReplayServer(wall.URL, "", "", store)
	cacheKey = replay.replayCacheKeyForMode("run-1", []replayRecording{{Attempt: 1, Artifact: artifact}}, replayModeRaw)
	jobID = farm.MediaRenderJobID(cacheKey)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/run-1/replay/render?mode=raw", nil)
	res := httptest.NewRecorder()
	replay.handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("render status=%d body=%s, want 200 ready", res.Code, res.Body.String())
	}
	var status replayStatus
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.State != "ready" || status.JobState != farm.MediaRenderJobReady || status.Size != 8 || status.JobID != jobID {
		t.Fatalf("ready status=%+v", status)
	}
	mu.Lock()
	gotCreateCalls := createCalls
	gotReconcileCalls := reconcileCalls
	mu.Unlock()
	if gotCreateCalls != 0 {
		t.Fatalf("ready artifact created %d render jobs, want 0", gotCreateCalls)
	}
	if gotReconcileCalls != 1 {
		t.Fatalf("ready artifact reconciliations=%d, want 1", gotReconcileCalls)
	}
}

func TestReplayStatusFromMediaJobIncludesLastFailure(t *testing.T) {
	status := replayStatusFromMediaJob(farm.MediaRenderJob{
		RunID:       "run-1",
		ArtifactKey: "artifact",
		State:       farm.MediaRenderJobRendering,
		Stage:       "segment-2",
		RetryCount:  1,
		LastError:   "recovered after expired worker lease",
	})
	if status.State != "generating" {
		t.Fatalf("state=%q, want generating", status.State)
	}
	if status.LastError != "recovered after expired worker lease" {
		t.Fatalf("last_error=%q", status.LastError)
	}
	if status.Error != "" {
		t.Fatalf("active job error=%q, want compatibility error field empty", status.Error)
	}
}
