package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func challengeRequest(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func redChallenge(id, name, starter string) challengeDefinition {
	return challengeDefinition{
		ID: id, Name: name,
		Run: farm.Spec{
			Game: "pokemon-red", Planner: "llm", Starter: starter,
			PlayStyle: "speedrun", RecoveryProfile: farm.RecoveryProfileStrict,
		},
	}
}

func TestChallengeCatalogValidatesCapabilitiesAndVersions(t *testing.T) {
	w := NewWall("")
	h := challengeProgrammingHTTPHandler(w, w.Handler())

	yellow := challengeDefinition{
		ID: "yellow-speedrun", Name: "Yellow speedrun",
		Run: farm.Spec{
			Game: "pokemon-yellow", Planner: "llm", Starter: "pikachu",
			PlayStyle: "speedrun", RecoveryProfile: farm.RecoveryProfileStrict,
		},
		Requirements: []string{"pokemon.story"},
	}
	res := challengeRequest(t, h, http.MethodPost, "/v1/challenges", yellow)
	if res.Code != http.StatusCreated {
		t.Fatalf("create yellow = %d: %s", res.Code, res.Body.String())
	}
	var saved challengeDefinition
	if err := json.Unmarshal(res.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Version != 1 || saved.Run.Starter != "pikachu" {
		t.Fatalf("saved = %+v", saved)
	}

	yellow.Description = "second revision"
	res = challengeRequest(t, h, http.MethodPost, "/v1/challenges", yellow)
	if res.Code != http.StatusCreated {
		t.Fatalf("create v2 = %d: %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Version != 2 {
		t.Fatalf("version = %d, want 2", saved.Version)
	}

	badStarter := yellow
	badStarter.ID = "yellow-mewtwo"
	badStarter.Version = 0
	badStarter.Run.Starter = "mewtwo"
	res = challengeRequest(t, h, http.MethodPost, "/v1/challenges", badStarter)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("invalid yellow starter = %d, want 400: %s", res.Code, res.Body.String())
	}

	missingCapability := redChallenge("missing-capability", "Missing capability", "mewtwo")
	missingCapability.Requirements = []string{"pokemon.trading"}
	res = challengeRequest(t, h, http.MethodPost, "/v1/challenges", missingCapability)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("missing capability = %d, want 400: %s", res.Code, res.Body.String())
	}

	mewtwo := redChallenge("mewtwo-starter", "Mewtwo starter", "mewtwo")
	res = challengeRequest(t, h, http.MethodPost, "/v1/challenges", mewtwo)
	if res.Code != http.StatusCreated {
		t.Fatalf("mewtwo starter = %d: %s", res.Code, res.Body.String())
	}
}

func TestProgrammingQueuePromotesOneChallengeAndHandsOffAfterFinish(t *testing.T) {
	w := NewWall("")
	h := challengeProgrammingHTTPHandler(w, w.Handler())

	for _, challenge := range []challengeDefinition{
		redChallenge("first", "First programmed run", "mewtwo"),
		redChallenge("second", "Second programmed run", "squirtle"),
	} {
		if res := challengeRequest(t, h, http.MethodPost, "/v1/challenges", challenge); res.Code != http.StatusCreated {
			t.Fatalf("create %s = %d: %s", challenge.ID, res.Code, res.Body.String())
		}
	}

	if res := challengeRequest(t, h, http.MethodPost, "/v1/challenges/first/queue", enqueueChallengeRequest{}); res.Code != http.StatusCreated {
		t.Fatalf("queue first = %d: %s", res.Code, res.Body.String())
	}
	if res := challengeRequest(t, h, http.MethodPost, "/v1/challenges/second/queue", enqueueChallengeRequest{}); res.Code != http.StatusCreated {
		t.Fatalf("queue second = %d: %s", res.Code, res.Body.String())
	}

	snapshotRes := challengeRequest(t, h, http.MethodGet, "/v1/programming", nil)
	var snapshot challengeProgrammingSnapshot
	if err := json.Unmarshal(snapshotRes.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.LiveNow == nil || snapshot.LiveNow.ChallengeID != "first" {
		t.Fatalf("live now = %+v, want first", snapshot.LiveNow)
	}
	if snapshot.UpNext == nil || snapshot.UpNext.ChallengeID != "second" {
		t.Fatalf("up next = %+v, want second", snapshot.UpNext)
	}
	if len(snapshot.LiveNow.RunIDs) != 1 {
		t.Fatalf("first run ids = %v", snapshot.LiveNow.RunIDs)
	}
	firstRunID := snapshot.LiveNow.RunIDs[0]

	lease := challengeRequest(t, h, http.MethodPost, "/v1/lease", nil)
	if lease.Code != http.StatusOK {
		t.Fatalf("lease first = %d: %s", lease.Code, lease.Body.String())
	}
	var leased farm.Spec
	if err := json.Unmarshal(lease.Body.Bytes(), &leased); err != nil {
		t.Fatal(err)
	}
	if leased.RunID != firstRunID {
		t.Fatalf("leased %q, want %q", leased.RunID, firstRunID)
	}

	finish := farm.FinishReport{RunID: firstRunID, Attempt: 1, Reason: "done", Detail: "challenge complete"}
	if res := challengeRequest(t, h, http.MethodPost, "/v1/runs/"+firstRunID+"/finish", finish); res.Code != http.StatusOK {
		t.Fatalf("finish first = %d: %s", res.Code, res.Body.String())
	}

	snapshotRes = challengeRequest(t, h, http.MethodGet, "/v1/programming", nil)
	if err := json.Unmarshal(snapshotRes.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.LiveNow == nil || snapshot.LiveNow.ChallengeID != "second" {
		t.Fatalf("live after handoff = %+v, want second", snapshot.LiveNow)
	}
	if snapshot.UpNext != nil {
		t.Fatalf("up next after handoff = %+v, want nil", snapshot.UpNext)
	}
	var firstState string
	for _, entry := range snapshot.Queue {
		if entry.ChallengeID == "first" {
			firstState = entry.State
		}
	}
	if firstState != programStateCompleted {
		t.Fatalf("first state = %q, want completed", firstState)
	}

	linkRes := challengeRequest(t, h, http.MethodGet, "/v1/programming/runs/"+firstRunID, nil)
	if linkRes.Code != http.StatusOK {
		t.Fatalf("run link = %d: %s", linkRes.Code, linkRes.Body.String())
	}
	var link challengeRunLink
	if err := json.Unmarshal(linkRes.Body.Bytes(), &link); err != nil {
		t.Fatal(err)
	}
	if link.ChallengeID != "first" || link.ChallengeVersion != 1 {
		t.Fatalf("run link = %+v", link)
	}
}

func TestProgrammingQueuePersistsCatalogAndScheduledEntry(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "wall.json")
	w := NewWall("")
	w.SetStatePath(statePath)
	h := challengeProgrammingHTTPHandler(w, w.Handler())

	challenge := redChallenge("persisted", "Persisted challenge", "bulbasaur")
	if res := challengeRequest(t, h, http.MethodPost, "/v1/challenges", challenge); res.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", res.Code, res.Body.String())
	}
	future := time.Now().Add(time.Hour).Unix()
	if res := challengeRequest(t, h, http.MethodPost, "/v1/challenges/persisted/queue", enqueueChallengeRequest{ScheduledAt: future}); res.Code != http.StatusCreated {
		t.Fatalf("queue = %d: %s", res.Code, res.Body.String())
	}

	w2 := NewWall("")
	w2.SetStatePath(statePath)
	h2 := challengeProgrammingHTTPHandler(w2, w2.Handler())
	catalog := challengeRequest(t, h2, http.MethodGet, "/v1/challenges", nil)
	if catalog.Code != http.StatusOK {
		t.Fatalf("catalog = %d: %s", catalog.Code, catalog.Body.String())
	}
	var catalogBody struct {
		Challenges []challengeDefinition `json:"challenges"`
	}
	if err := json.Unmarshal(catalog.Body.Bytes(), &catalogBody); err != nil {
		t.Fatal(err)
	}
	if len(catalogBody.Challenges) != 1 || catalogBody.Challenges[0].ID != "persisted" {
		t.Fatalf("catalog after restart = %+v", catalogBody.Challenges)
	}

	programming := challengeRequest(t, h2, http.MethodGet, "/v1/programming", nil)
	var snapshot challengeProgrammingSnapshot
	if err := json.Unmarshal(programming.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.UpNext == nil || snapshot.UpNext.ChallengeID != "persisted" || snapshot.UpNext.State != programStateScheduled {
		t.Fatalf("programming after restart = %+v", snapshot)
	}
}

func TestProgrammingBlockedEntryCanRetryOrSkip(t *testing.T) {
	w := NewWall("")
	h := challengeProgrammingHTTPHandler(w, w.Handler())
	controller := challengeProgrammingFor(w)

	challenge := redChallenge("retryable", "Retryable", "squirtle")
	if res := challengeRequest(t, h, http.MethodPost, "/v1/challenges", challenge); res.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", res.Code, res.Body.String())
	}
	if res := challengeRequest(t, h, http.MethodPost, "/v1/programming/pause", map[string]any{}); res.Code != http.StatusOK {
		t.Fatalf("pause = %d: %s", res.Code, res.Body.String())
	}
	res := challengeRequest(t, h, http.MethodPost, "/v1/challenges/retryable/queue", enqueueChallengeRequest{})
	if res.Code != http.StatusCreated {
		t.Fatalf("queue = %d: %s", res.Code, res.Body.String())
	}
	var entry challengeQueueEntry
	if err := json.Unmarshal(res.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	controller.mu.Lock()
	for i := range controller.state.Entries {
		if controller.state.Entries[i].ID == entry.ID {
			controller.state.Entries[i].State = programStateBlocked
			controller.state.Entries[i].Error = "synthetic start failure"
		}
	}
	controller.mu.Unlock()

	retry := challengeRequest(t, h, http.MethodPost, "/v1/programming/"+entry.ID+"/retry", map[string]any{})
	if retry.Code != http.StatusOK {
		t.Fatalf("retry = %d: %s", retry.Code, retry.Body.String())
	}
	skip := challengeRequest(t, h, http.MethodPost, "/v1/programming/"+entry.ID+"/skip", map[string]any{})
	if skip.Code != http.StatusOK {
		t.Fatalf("skip = %d: %s", skip.Code, skip.Body.String())
	}
}
