package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestAudienceVoteLifecycleDeduplicatesAndPinsWinner(t *testing.T) {
	w := NewWall("")
	programming := challengeProgrammingHTTPHandler(w, w.Handler())
	h := challengeVotingHTTPHandler(w, programming)

	for _, challenge := range []challengeDefinition{
		redChallenge("yellow-race", "Yellow race", "pikachu"),
		redChallenge("mewtwo-run", "Mewtwo run", "mewtwo"),
	} {
		// Keep both fixtures valid Red definitions; the ids/names are editorial.
		if challenge.ID == "yellow-race" {
			challenge.Run.Starter = "squirtle"
		}
		res := challengeRequest(t, h, http.MethodPost, "/v1/challenges", challenge)
		if res.Code != http.StatusCreated {
			t.Fatalf("create %s = %d: %s", challenge.ID, res.Code, res.Body.String())
		}
	}
	if res := challengeRequest(t, h, http.MethodPost, "/v1/programming/pause", map[string]any{}); res.Code != http.StatusOK {
		t.Fatalf("pause = %d: %s", res.Code, res.Body.String())
	}

	create := challengeRequest(t, h, http.MethodPost, "/v1/votes", map[string]any{
		"candidates": []map[string]any{
			{"challenge_id": "yellow-race"},
			{"challenge_id": "mewtwo-run"},
		},
		"tie_policy": "first",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create vote = %d: %s", create.Code, create.Body.String())
	}
	var vote challengeVoteSnapshot
	if err := json.Unmarshal(create.Body.Bytes(), &vote); err != nil {
		t.Fatal(err)
	}
	if len(vote.Candidates) != 2 || vote.Status != voteStatusOpen {
		t.Fatalf("vote = %+v", vote)
	}

	first := challengeRequest(t, h, http.MethodPost, "/v1/votes/"+vote.ID+"/ballots", map[string]any{
		"source": "spectator", "voter_id": "anon-a", "candidate_id": "mewtwo-run",
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first ballot = %d: %s", first.Code, first.Body.String())
	}
	var ballotResponse struct {
		Accepted bool                  `json:"accepted"`
		Vote     challengeVoteSnapshot `json:"vote"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &ballotResponse); err != nil {
		t.Fatal(err)
	}
	if !ballotResponse.Accepted || ballotResponse.Vote.SourceTotals["spectator"] != 1 {
		t.Fatalf("first ballot response = %+v", ballotResponse)
	}

	duplicate := challengeRequest(t, h, http.MethodPost, "/v1/votes/"+vote.ID+"/ballots", map[string]any{
		"source": "spectator", "voter_id": "anon-a", "candidate_id": "mewtwo-run",
	})
	if duplicate.Code != http.StatusOK {
		t.Fatalf("idempotent duplicate = %d: %s", duplicate.Code, duplicate.Body.String())
	}
	if err := json.Unmarshal(duplicate.Body.Bytes(), &ballotResponse); err != nil {
		t.Fatal(err)
	}
	if ballotResponse.Accepted {
		t.Fatal("duplicate ballot unexpectedly accepted")
	}

	changed := challengeRequest(t, h, http.MethodPost, "/v1/votes/"+vote.ID+"/ballots", map[string]any{
		"source": "spectator", "voter_id": "anon-a", "candidate_id": "yellow-race",
	})
	if changed.Code != http.StatusConflict {
		t.Fatalf("changed duplicate = %d, want 409: %s", changed.Code, changed.Body.String())
	}

	closeRes := challengeRequest(t, h, http.MethodPost, "/v1/votes/"+vote.ID+"/close", map[string]any{})
	if closeRes.Code != http.StatusOK {
		t.Fatalf("close = %d: %s", closeRes.Code, closeRes.Body.String())
	}
	var closed challengeVoteSnapshot
	if err := json.Unmarshal(closeRes.Body.Bytes(), &closed); err != nil {
		t.Fatal(err)
	}
	if closed.WinnerID != "mewtwo-run" || closed.QueueEntryID != "slot-"+vote.ID {
		t.Fatalf("closed = %+v", closed)
	}

	// Closing again must return the same result without creating a second slot.
	secondClose := challengeRequest(t, h, http.MethodPost, "/v1/votes/"+vote.ID+"/close", map[string]any{})
	if secondClose.Code != http.StatusOK {
		t.Fatalf("second close = %d: %s", secondClose.Code, secondClose.Body.String())
	}
	snapshotRes := challengeRequest(t, h, http.MethodGet, "/v1/programming", nil)
	var snapshot challengeProgrammingSnapshot
	if err := json.Unmarshal(snapshotRes.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range snapshot.Queue {
		if entry.ID == "slot-"+vote.ID {
			count++
			if entry.ChallengeID != "mewtwo-run" || !entry.Pinned {
				t.Fatalf("winner entry = %+v", entry)
			}
		}
	}
	if count != 1 {
		t.Fatalf("winner queue entries = %d, want 1; queue=%+v", count, snapshot.Queue)
	}
}

func TestAudienceVoteOperatorTieAndSourceLimit(t *testing.T) {
	w := NewWall("")
	h := challengeVotingHTTPHandler(w, challengeProgrammingHTTPHandler(w, w.Handler()))
	for _, challenge := range []challengeDefinition{
		redChallenge("a", "A", "squirtle"),
		redChallenge("b", "B", "charmander"),
	} {
		if res := challengeRequest(t, h, http.MethodPost, "/v1/challenges", challenge); res.Code != http.StatusCreated {
			t.Fatalf("create %s = %d: %s", challenge.ID, res.Code, res.Body.String())
		}
	}
	_ = challengeRequest(t, h, http.MethodPost, "/v1/programming/pause", map[string]any{})

	create := challengeRequest(t, h, http.MethodPost, "/v1/votes", map[string]any{
		"candidates":    []map[string]any{{"challenge_id": "a"}, {"challenge_id": "b"}},
		"tie_policy":    "operator",
		"source_limits": map[string]int{"twitch": 1},
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create vote = %d: %s", create.Code, create.Body.String())
	}
	var vote challengeVoteSnapshot
	if err := json.Unmarshal(create.Body.Bytes(), &vote); err != nil {
		t.Fatal(err)
	}

	if res := challengeRequest(t, h, http.MethodPost, "/v1/votes/"+vote.ID+"/ballots", map[string]any{
		"source": "twitch", "voter_id": "u1", "candidate_id": "a",
	}); res.Code != http.StatusOK {
		t.Fatalf("twitch ballot = %d: %s", res.Code, res.Body.String())
	}
	if res := challengeRequest(t, h, http.MethodPost, "/v1/votes/"+vote.ID+"/ballots", map[string]any{
		"source": "twitch", "voter_id": "u2", "candidate_id": "b",
	}); res.Code != http.StatusTooManyRequests {
		t.Fatalf("source limit = %d, want 429: %s", res.Code, res.Body.String())
	}
	if res := challengeRequest(t, h, http.MethodPost, "/v1/votes/"+vote.ID+"/ballots", map[string]any{
		"source": "spectator", "voter_id": "u2", "candidate_id": "b",
	}); res.Code != http.StatusOK {
		t.Fatalf("spectator ballot = %d: %s", res.Code, res.Body.String())
	}

	tied := challengeRequest(t, h, http.MethodPost, "/v1/votes/"+vote.ID+"/close", map[string]any{})
	if tied.Code != http.StatusBadRequest {
		t.Fatalf("operator tie close = %d, want 400: %s", tied.Code, tied.Body.String())
	}
	resolved := challengeRequest(t, h, http.MethodPost, "/v1/votes/"+vote.ID+"/close", map[string]any{"winner_id": "b"})
	if resolved.Code != http.StatusOK {
		t.Fatalf("resolved tie = %d: %s", resolved.Code, resolved.Body.String())
	}
	var closed challengeVoteSnapshot
	if err := json.Unmarshal(resolved.Body.Bytes(), &closed); err != nil {
		t.Fatal(err)
	}
	if closed.WinnerID != "b" {
		t.Fatalf("winner = %q, want b", closed.WinnerID)
	}
}

func TestAudienceVoteRejectsUnsupportedCandidatesAndPersists(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "wall.json")
	w := NewWall("")
	w.SetStatePath(statePath)
	h := challengeVotingHTTPHandler(w, challengeProgrammingHTTPHandler(w, w.Handler()))

	for _, challenge := range []challengeDefinition{
		redChallenge("one", "One", "squirtle"),
		redChallenge("two", "Two", "charmander"),
	} {
		if res := challengeRequest(t, h, http.MethodPost, "/v1/challenges", challenge); res.Code != http.StatusCreated {
			t.Fatalf("create %s = %d: %s", challenge.ID, res.Code, res.Body.String())
		}
	}
	missing := challengeRequest(t, h, http.MethodPost, "/v1/votes", map[string]any{
		"candidates": []map[string]any{{"challenge_id": "one"}, {"challenge_id": "missing"}},
	})
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing candidate = %d, want 400: %s", missing.Code, missing.Body.String())
	}

	closesAt := time.Now().Add(time.Hour).Unix()
	create := challengeRequest(t, h, http.MethodPost, "/v1/votes", map[string]any{
		"candidates": []map[string]any{{"challenge_id": "one"}, {"challenge_id": "two"}},
		"closes_at":  closesAt,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", create.Code, create.Body.String())
	}
	var created challengeVoteSnapshot
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if res := challengeRequest(t, h, http.MethodPost, "/v1/votes/"+created.ID+"/ballots", map[string]any{
		"source": "youtube", "voter_id": "viewer-1", "candidate_id": "one",
	}); res.Code != http.StatusOK {
		t.Fatalf("ballot = %d: %s", res.Code, res.Body.String())
	}

	w2 := NewWall("")
	w2.SetStatePath(statePath)
	h2 := challengeVotingHTTPHandler(w2, challengeProgrammingHTTPHandler(w2, w2.Handler()))
	active := challengeRequest(t, h2, http.MethodGet, "/v1/votes/active", nil)
	if active.Code != http.StatusOK {
		t.Fatalf("active = %d: %s", active.Code, active.Body.String())
	}
	var response struct {
		Active *challengeVoteSnapshot `json:"active"`
	}
	if err := json.Unmarshal(active.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Active == nil || response.Active.ID != created.ID || response.Active.SourceTotals["youtube"] != 1 {
		t.Fatalf("reloaded active = %+v", response.Active)
	}
}
