package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestFailureIdentityStable(t *testing.T) {
	a := normalizeDetail("still on map 0x0c at (10,35)")
	b := normalizeDetail("still on map 0x21 at (4,22)")
	if a != b {
		t.Fatalf("coordinates/map ids must normalize together: %q vs %q", a, b)
	}
	key1, fp1 := failureIdentity(a)
	key2, fp2 := failureIdentity(b)
	if key1 != key2 || fp1 != fp2 {
		t.Fatalf("same pattern must share key/fingerprint: %s %s vs %s %s", key1, fp1, key2, fp2)
	}
	if len(key1) != 16 {
		t.Fatalf("key length = %d, want 16", len(key1))
	}
	sum := sha256.Sum256([]byte(a))
	wantFP := "sha256:" + hex.EncodeToString(sum[:])
	if fp1 != wantFP {
		t.Fatalf("fingerprint = %s, want %s", fp1, wantFP)
	}
	if !strings.HasPrefix(fp1, "sha256:") || len(fp1) != len("sha256:")+64 {
		t.Fatalf("fingerprint shape = %q", fp1)
	}

	other := normalizeDetail("walk to warp on map 0x0c: text box interrupted movement")
	key3, fp3 := failureIdentity(other)
	if key3 == key1 || fp3 == fp1 {
		t.Fatal("different words must remain different groups")
	}
}

func TestTriageNewestRepresentativeFirst(t *testing.T) {
	w := wallWithFinished(
		failedTile("run-old", "still on map 0x0c at (10,35)"),
		failedTile("run-new", "still on map 0x21 at (4,22)"),
	)
	got := w.triage()
	if len(got) != 1 {
		t.Fatalf("groups = %d, want 1", len(got))
	}
	if got[0].Key == "" || got[0].Fingerprint == "" {
		t.Fatalf("group missing identity: %+v", got[0])
	}
	if len(got[0].RunIDs) < 1 || got[0].RunIDs[0] != "run-new" {
		t.Fatalf("run ids = %v, want newest first (run-new)", got[0].RunIDs)
	}
}

func TestFinishRejectsCorruptArtifactsBeforeSettle(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)

	postJSON(t, srv.URL+"/v1/specs", spec("bad-art"))
	postJSON(t, srv.URL+"/v1/lease", struct{}{})
	resp := postJSON(t, srv.URL+"/v1/runs/bad-art/finish", farm.FinishReport{
		RunID:    "bad-art",
		Attempt:  1,
		Reason:   "error",
		Detail:   "stuck",
		SeedBurn: -1,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("finish corrupt = %d, want 400", resp.StatusCode)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			t.Fatalf("dump written despite corrupt evidence: %s", e.Name())
		}
	}
	w.mu.Lock()
	tile := w.tiles["bad-art"]
	finished := tile != nil && tile.Finished
	w.mu.Unlock()
	if finished {
		t.Fatal("corrupt finish settled the run")
	}
}

func TestIssueLinkAndOutboxPersist(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "wall-state.json")
	w1 := NewWall(filepath.Join(dir, "dumps"))
	w1.SetStatePath(stateFile)
	key, fp := failureIdentity(normalizeDetail("still on map 0x0c at (1,2)"))
	w1.mu.Lock()
	w1.issueLinks[key] = IssueLink{
		IssueID:         "11111111-1111-1111-1111-111111111111",
		IssueNumber:     42,
		IssueURL:        "http://192.168.50.81:8081/issues/11111111-1111-1111-1111-111111111111",
		Status:          "open",
		OccurrenceCount: 1,
		LastReportedRun: "run-2f",
		UpdatedAt:       1788080000,
		Fingerprint:     fp,
	}
	w1.outbox["run-2f-attempt-1"] = outboxEntry{
		ExternalID: "run-2f-attempt-1",
		RunID:      "run-2f",
		Attempt:    1,
		Key:        key,
		Status:     outboxPending,
		UpdatedAt:  1788080000,
	}
	w1.outbox["run-1-attempt-1"] = outboxEntry{
		ExternalID: "run-1-attempt-1",
		RunID:      "run-1",
		Attempt:    1,
		Key:        key,
		Status:     outboxComplete,
		UpdatedAt:  1788080001,
	}
	w1.mu.Unlock()
	w1.saveState()

	w2 := NewWall(filepath.Join(dir, "dumps"))
	w2.SetStatePath(stateFile)
	w2.mu.Lock()
	got := w2.issueLinks[key]
	pending := w2.outbox["run-2f-attempt-1"]
	done := w2.outbox["run-1-attempt-1"]
	w2.mu.Unlock()
	if got.IssueID == "" || got.IssueNumber != 42 || got.IssueURL == "" || got.LastReportedRun != "run-2f" || got.UpdatedAt != 1788080000 || got.Status != "open" {
		t.Fatalf("restored issue link = %+v", got)
	}
	if pending.Status != outboxPending || done.Status != outboxComplete {
		t.Fatalf("restored outbox pending=%+v complete=%+v", pending, done)
	}
}

func TestCheckpointRetainsBoundedWindow(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)
	client := farm.NewClient(srv.URL)

	postJSON(t, srv.URL+"/v1/specs", spec("cp-1"))
	postJSON(t, srv.URL+"/v1/lease", struct{}{})

	for i := 1; i <= 5; i++ {
		name := periodicStateNameForTest(uint64(i) * 18000)
		art := hashedArtifact(name, []byte("p"+strings.Repeat("x", i)))
		if err := client.Checkpoint(t.Context(), farm.CheckpointReport{
			RunID: "cp-1", Attempt: 1, Artifacts: []farm.Artifact{art, hashedArtifact(strings.TrimSuffix(name, ".state")+".json", []byte(`{}`))},
		}); err != nil {
			t.Fatalf("checkpoint periodic %d: %v", i, err)
		}
	}
	objState := hashedArtifact("round-001-frame-0000000100-goto.state", []byte("obj-state"))
	objKnow := hashedArtifact("round-001-frame-0000000100-goto.knowledge-v4.json", []byte(`{"k":1}`))
	if err := client.Checkpoint(t.Context(), farm.CheckpointReport{
		RunID: "cp-1", Attempt: 1, Artifacts: []farm.Artifact{objState, objKnow},
	}); err != nil {
		t.Fatalf("checkpoint objective: %v", err)
	}
	obj2 := hashedArtifact("round-002-frame-0000000200-goto.state", []byte("obj-state-2"))
	know2 := hashedArtifact("round-002-frame-0000000200-goto.knowledge-v4.json", []byte(`{"k":2}`))
	if err := client.Checkpoint(t.Context(), farm.CheckpointReport{
		RunID: "cp-1", Attempt: 1, Artifacts: []farm.Artifact{obj2, know2},
	}); err != nil {
		t.Fatalf("checkpoint objective 2: %v", err)
	}

	files := listCheckpointFiles(t, dir)
	var periodic, objective int
	for _, n := range files {
		switch {
		case strings.HasPrefix(n, "periodic-") && strings.HasSuffix(n, ".state"):
			periodic++
		case strings.HasPrefix(n, "round-") && strings.HasSuffix(n, ".state"):
			objective++
		}
	}
	if periodic != 3 {
		t.Fatalf("periodic states retained = %d, want 3: %v", periodic, files)
	}
	if objective != 1 {
		t.Fatalf("objective states retained = %d, want 1 (latest pair): %v", objective, files)
	}
	if !containsName(files, "round-002-frame-0000000200-goto.state") {
		t.Fatalf("latest objective pair missing: %v", files)
	}
	if containsName(files, "round-001-frame-0000000100-goto.state") {
		t.Fatalf("older objective pair was kept: %v", files)
	}

	if resp := postJSON(t, srv.URL+"/v1/runs/cp-1/finish", farm.FinishReport{RunID: "cp-1", Attempt: 1, Reason: "done"}); resp.StatusCode != http.StatusOK {
		t.Fatalf("finish = %d", resp.StatusCode)
	}
	err := client.Checkpoint(t.Context(), farm.CheckpointReport{
		RunID: "cp-1", Attempt: 1, Artifacts: []farm.Artifact{hashedArtifact("periodic-00000099999.state", []byte("late"))},
	})
	if err == nil {
		t.Fatal("checkpoint after Finish must be rejected")
	}
}

func TestDashboardAndTriageExposeIssueLinkWithoutArtifacts(t *testing.T) {
	w := wallWithFinished(failedTile("run-a", "still on map 0x0c at (10,35)"))
	key, _ := failureIdentity(normalizeDetail("still on map 0x0c at (10,35)"))
	w.mu.Lock()
	w.issueLinks[key] = IssueLink{
		IssueID:     "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		IssueNumber: 42,
		IssueURL:    "http://192.168.50.81:8081/issues/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Status:      "open",
	}
	w.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/v1/triage", nil)
	res := httptest.NewRecorder()
	w.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("triage = %d: %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, `"issue_number":42`) || !strings.Contains(body, `"issue_url":`) {
		t.Fatalf("triage missing issue link: %s", body)
	}
	if strings.Contains(body, `"data"`) || strings.Contains(body, "artifacts") {
		t.Fatalf("triage leaked artifact bytes: %s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil)
	res = httptest.NewRecorder()
	w.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("dashboard = %d: %s", res.Code, res.Body.String())
	}
	raw := res.Body.Bytes()
	if !bytes.Contains(raw, []byte(`"issue_number":42`)) {
		t.Fatalf("dashboard missing issue link: %s", raw)
	}
	if bytes.Contains(raw, []byte(`"artifacts"`)) {
		t.Fatalf("dashboard leaked artifacts: %s", raw)
	}
}

func hashedArtifact(name string, data []byte) farm.Artifact {
	sum := sha256.Sum256(data)
	media := "application/octet-stream"
	if strings.HasSuffix(name, ".json") {
		media = "application/json"
	}
	return farm.Artifact{Name: name, MediaType: media, SHA256: hex.EncodeToString(sum[:]), Data: data}
}

func periodicStateNameForTest(frame uint64) string {
	return fmt.Sprintf("periodic-%010d.state", frame)
}

func listCheckpointFiles(t *testing.T, dumps string) []string {
	t.Helper()
	var names []string
	filepath.Walk(dumps, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		names = append(names, info.Name())
		return nil
	})
	return names
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
