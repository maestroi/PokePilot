package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
	_ "modernc.org/sqlite"
)

// run-s6v9q3t2w5rl: attempt N booted fresh after a failed resume lookup, wrote
// a starter checkpoint, and was lost. The lost-worker retry must still resume
// the campaign's deepest pair, not lock in the fresh attempt's early one.
func TestLostRetryAfterFreshFallbackResumesDeepestLineagePair(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "deep", Planner: "llm", Endless: true, Goal: farm.GoalFrom("beat the game")})

	if l, err := client.Lease(ctx); err != nil || l == nil || l.Attempt != 1 {
		t.Fatalf("lease 1 = %+v, %v", l, err)
	}
	deepState := wallResumeArtifact("round-001-frame-0006644852-progress.state", []byte("deep"), "application/octet-stream")
	deepKnowledge := wallResumeArtifact("round-001-frame-0006644852-progress.knowledge-v6.json", []byte(`{"intent":"deep"}`), "application/json")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{RunID: "deep", Attempt: 1, Artifacts: []farm.Artifact{deepState, deepKnowledge}}); err != nil {
		t.Fatal(err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "deep", Attempt: 1, Reason: "error", Detail: "objective bug"}); err != nil {
		t.Fatal(err)
	}

	if l, err := client.Lease(ctx); err != nil || l == nil || l.Attempt != 2 {
		t.Fatalf("lease 2 = %+v, %v", l, err)
	}
	freshState := wallResumeArtifact("round-001-frame-0000003487-take-starter.state", []byte("fresh"), "application/octet-stream")
	freshKnowledge := wallResumeArtifact("round-001-frame-0000003487-take-starter.knowledge-v6.json", []byte(`{"intent":"fresh"}`), "application/json")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{RunID: "deep", Attempt: 2, Artifacts: []farm.Artifact{freshState, freshKnowledge}}); err != nil {
		t.Fatal(err)
	}
	w.mu.Lock()
	w.tiles["deep"].Status = statusRunning
	w.tiles["deep"].lastUpdate = time.Now().Add(-time.Minute)
	w.mu.Unlock()
	if got := w.reapStale(time.Now()); len(got) != 1 {
		t.Fatalf("reaped = %v", got)
	}

	third, err := client.Lease(ctx)
	if err != nil || third == nil || third.Attempt != 3 {
		t.Fatalf("lease 3 = %+v, %v", third, err)
	}
	cp, err := client.ResumeCheckpoint(ctx, "deep", third.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil || cp.State.Name != deepState.Name {
		t.Fatalf("resume = %+v, want deepest lineage pair %s", cp, deepState.Name)
	}
}

func TestControlPlaneLineageObjectiveRanksByMetadataAndSkipsBrokenCandidates(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE artifacts (
 run_id TEXT NOT NULL, attempt INTEGER NOT NULL, kind TEXT NOT NULL, name TEXT NOT NULL,
 metadata_json BLOB NOT NULL, inline_data BLOB)`); err != nil {
		t.Fatal(err)
	}
	put := func(attempt int, art farm.Artifact, inline bool) {
		t.Helper()
		meta := art
		meta.Data = nil
		if !inline {
			// An S3 reference in a test with no S3 configured: materializing
			// it fails, which is what an unreadable object looks like.
			meta.Store, meta.Bucket, meta.ObjectKey = farm.ArtifactStoreS3, "missing", "missing/"+art.Name
		}
		raw, _ := json.Marshal(meta)
		var data any
		if inline {
			data = art.Data
		}
		if _, err := db.Exec(`INSERT INTO artifacts(run_id,attempt,kind,name,metadata_json,inline_data) VALUES(?,?,'checkpoint',?,?,?)`,
			"cp-run", attempt, art.Name, raw, data); err != nil {
			t.Fatal(err)
		}
	}
	pair := func(frame, payload string) (farm.Artifact, farm.Artifact) {
		return wallResumeArtifact("round-001-frame-"+frame+"-x.state", []byte(payload), "application/octet-stream"),
			wallResumeArtifact("round-001-frame-"+frame+"-x.knowledge-v6.json", []byte(`{"p":"`+payload+`"}`), "application/json")
	}
	deepS, deepK := pair("0000009000", "deep")
	midS, midK := pair("0000005000", "mid")
	freshS, freshK := pair("0000000100", "fresh")
	put(1, midS, true)
	put(1, midK, true)
	put(2, deepS, false) // deepest, but its object cannot be read
	put(2, deepK, true)
	put(3, freshS, true)
	put(3, freshK, true)

	w := NewWall("")
	cp := &controlPlane{db: db}
	wallControlPlanes.Store(w, cp)
	t.Cleanup(func() { wallControlPlanes.Delete(w) })
	w.mu.Lock()
	w.tiles["cp-run"] = &Tile{RunID: "cp-run", Attempts: 3}
	w.mu.Unlock()

	got, err := w.latestStoredLineageObjective("cp-run")
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Name != midS.Name || got.Attempt != 1 {
		t.Fatalf("resume = %s attempt %d, want %s attempt 1", got.State.Name, got.Attempt, midS.Name)
	}
}

func TestControlPlaneResumeLookupFailureIsNotFreshStart(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() }) // no artifacts table: every lookup fails
	w := NewWall("")
	wallControlPlanes.Store(w, &controlPlane{db: db})
	t.Cleanup(func() { wallControlPlanes.Delete(w) })
	w.mu.Lock()
	w.tiles["broken"] = &Tile{RunID: "broken", Planner: "llm", Endless: true, Attempts: 4, Detail: "attempt 4 failed: error"}
	w.mu.Unlock()

	rec := httptest.NewRecorder()
	w.handleControlPlaneCheckpointResume(rec, farm.CheckpointReport{RunID: "broken", Attempt: 5})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (204 would boot a fresh cartridge)", rec.Code)
	}
}
