package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestUploadNewObjectivePairsPromotesFirstPostBadgeCheckpoint(t *testing.T) {
	dir := t.TempDir()
	writeMajorTestPair(t, dir, "round-010-frame-0000001000-beat-the-gym-leader-here.state", "pre-gym", 0)
	writeMajorTestPair(t, dir, "round-011-frame-0000001100-heal-the-party.state", "first-post-gym", 1)
	writeMajorTestPair(t, dir, "round-012-frame-0000001200-go-to-route-25.state", "later", 1)

	var uploadedStates []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/endless/checkpoint" {
			http.NotFound(w, r)
			return
		}
		var report farm.CheckpointReport
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			t.Errorf("decode checkpoint: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, a := range report.Artifacts {
			if strings.HasSuffix(a.Name, ".state") {
				uploadedStates = append(uploadedStates, a.Name)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	client := farm.NewClient(srv.URL)
	uploadNewObjectivePairs(client, "endless", 1, dir, map[string]struct{}{})

	var majors []string
	for _, name := range uploadedStates {
		if strings.HasPrefix(name, majorCheckpointStatePrefix) {
			majors = append(majors, name)
		}
	}
	if len(majors) != 1 {
		t.Fatalf("major checkpoint uploads = %v, want exactly one", majors)
	}
	want := "major-badge-01-round-011-frame-0000001100-heal-the-party.state"
	if majors[0] != want {
		t.Fatalf("major checkpoint = %q, want first post-gym boundary %q", majors[0], want)
	}
	data, err := os.ReadFile(filepath.Join(dir, want))
	if err != nil {
		t.Fatalf("read promoted state: %v", err)
	}
	if string(data) != "first-post-gym" {
		t.Fatalf("promoted state = %q, want first post-gym state", data)
	}
	if _, err := os.Stat(majorPromotionMarker(dir, 1)); err != nil {
		t.Fatalf("promotion marker missing: %v", err)
	}
}

func TestSeedMajorPromotionMarkersPreventsResumeDrift(t *testing.T) {
	dir := t.TempDir()
	major := "major-badge-02-round-050-frame-0000005000-go-to-pokemon-center.state"
	if err := os.WriteFile(filepath.Join(dir, major), []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedMajorPromotionMarkers(dir)
	if _, err := os.Stat(majorPromotionMarker(dir, 2)); err != nil {
		t.Fatalf("resume major did not seed marker: %v", err)
	}

	writeMajorTestPair(t, dir, "round-051-frame-0000005100-go-to-route-9.state", "later", 2)
	badge, stateName, _, err := promoteBadgeCheckpointFiles(
		dir,
		"round-051-frame-0000005100-go-to-route-9.state",
		"round-051-frame-0000005100-go-to-route-9.knowledge-v4.json",
	)
	if err != nil {
		t.Fatal(err)
	}
	if badge != 0 || stateName != "" {
		t.Fatalf("resumed badge was re-promoted: badge=%d state=%q", badge, stateName)
	}
}

func writeMajorTestPair(t *testing.T, dir, stateName, state string, gymWins int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, stateName), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(stateName, ".state")
	mem := map[string]any{
		"version": 4,
		"completed": []map[string]any{
			{"Objective": gymCompletionObjective, "Times": gymWins},
		},
	}
	data, err := json.Marshal(mem)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, base+".knowledge-v4.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
