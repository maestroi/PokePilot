package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/media/publication"
)

func TestProductionStepAdvancesWithoutRepeatingAssessment(t *testing.T) {
	goalComplete := true
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"run": map[string]any{
			"status": "done", "reason": "done", "stats": map[string]any{"goal_complete": goalComplete},
		}})
	}))
	defer wall.Close()
	s := newReplayServer(wall.URL, "", "", nil)
	var health map[string]any
	if _, err := selfJSON(context.Background(), s.handler(), http.MethodGet, "/healthz", nil, &health); err != nil || health["status"] == nil {
		t.Fatalf("in-process call through the real mux: %v %v", health, err)
	}

	h := func(c byte) string { return strings.Repeat(string(c), 64) }
	broadcast, edits := "generating", "generating"
	calls := map[string]int{}
	var formats []string
	mux := http.NewServeMux()
	route := func(pattern string, reply func(r *http.Request) any) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			calls[pattern]++
			writeJSON(w, http.StatusOK, reply(r))
		})
	}
	route("POST /v1/runs/{id}/replay/render", func(*http.Request) any { return map[string]string{"state": broadcast} })
	route("POST /v1/runs/{id}/producer/assess", func(*http.Request) any {
		return map[string]any{"assessment": map[string]string{"hash": h('a'), "publication_class": "full_episode"}}
	})
	route("POST /v1/runs/{id}/studio/edits", func(*http.Request) any { return map[string]any{"plan": map[string]string{"hash": h('b')}} })
	route("POST /v1/runs/{id}/studio/edits/{hash}/render", func(*http.Request) any { return map[string]string{"state": edits} })
	route("POST /v1/runs/{id}/studio/packages", func(*http.Request) any {
		return map[string]any{"package": map[string]any{"hash": h('c'), "titles": []any{map[string]string{"id": "t"}}, "shorts": []any{map[string]string{"id": "s"}}}}
	})
	route("POST /v1/runs/{id}/studio/packages/{hash}/thumbnails", func(*http.Request) any {
		return map[string]any{"manifest": map[string]any{"candidates": []any{map[string]string{"id": h('d')}}}}
	})
	route("POST /v1/runs/{id}/studio/packages/{hash}/thumbnails/{candidate}/select", func(*http.Request) any { return map[string]string{} })
	route("POST /v1/runs/{id}/studio/publications", func(r *http.Request) any {
		var body struct {
			Format  string `json:"format"`
			Privacy string `json:"privacy"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Privacy != "private" {
			t.Errorf("candidate %s privacy = %q, want private", body.Format, body.Privacy)
		}
		formats = append(formats, body.Format)
		return map[string]string{"id": h('e'), "state": "queued"}
	})

	st := productionState{Version: 1, RunID: "run-e2e", Stage: "pending"}
	step := func() {
		t.Helper()
		if err := s.productionStep(context.Background(), mux, &st); err != nil {
			t.Fatalf("step at %s: %v", st.Stage, err)
		}
	}

	step() // broadcast still rendering
	if st.Stage != "broadcast_render" || st.Done || calls["POST /v1/runs/{id}/producer/assess"] != 0 {
		t.Fatalf("after broadcast wait: %+v calls=%v", st, calls)
	}
	broadcast = "ready"
	step() // episode edit still rendering
	if st.Stage != "episode_render" || st.Done {
		t.Fatalf("after edit wait: %+v", st)
	}
	edits = "ready"
	step()
	if !st.Done || st.Stage != "ready_for_review" || len(st.Candidates) != 2 {
		t.Fatalf("final: %+v", st)
	}
	if n := calls["POST /v1/runs/{id}/producer/assess"]; n != 1 {
		t.Fatalf("producer assessed %d times, want exactly 1 (it is LLM-backed and non-deterministic)", n)
	}
	if strings.Join(formats, ",") != "episode,short" {
		t.Fatalf("candidates = %v", formats)
	}

	goalComplete = false
	st = productionState{Version: 1, RunID: "run-lost", Stage: "pending"}
	step()
	if !st.Done || st.Stage != "goal_not_complete" {
		t.Fatalf("run without the goal flag must stop, got %+v", st)
	}
}

func TestProductionRetryRearmsFailedStageKeepingAssessment(t *testing.T) {
	_, store := newMemS3(t)
	s := newReplayServer("http://wall.invalid", "", "", store)
	ctx := context.Background()
	h := s.handler()
	assessment := strings.Repeat("a", 64)

	if code, _ := selfJSON(ctx, h, http.MethodPost, "/v1/runs/run-e2e/production/retry", nil, nil); code != http.StatusConflict {
		t.Fatalf("retry of pending production = %d, want 409", code)
	}
	if err := s.saveProduction(ctx, productionState{Version: 1, RunID: "run-e2e", Stage: "failed_episode_render", Done: true,
		AssessmentHash: assessment, Verdict: "full_episode", Error: "422", Failures: productionMaxFailures}); err != nil {
		t.Fatal(err)
	}
	var st productionState
	if _, err := selfJSON(ctx, h, http.MethodPost, "/v1/runs/run-e2e/production/retry", nil, &st); err != nil {
		t.Fatal(err)
	}
	if st.Stage != "episode_render" || st.Done || st.Failures != 0 || st.Error != "" || st.AssessmentHash != assessment {
		t.Fatalf("retry state = %+v", st)
	}
	if saved, err := s.loadProduction(ctx, "run-e2e"); err != nil || saved.Done || saved.Stage != "episode_render" {
		t.Fatalf("retry not persisted: %+v %v", saved, err)
	}
}

func TestProductionUploadsOptedInPrivateCandidatesInTurn(t *testing.T) {
	h := func(c byte) string { return strings.Repeat(string(c), 64) }
	states := map[string]publication.State{h('e'): publication.Queued, h('f'): publication.Queued}
	pacing := false
	var actions []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/runs/{id}/studio/publications", func(w http.ResponseWriter, r *http.Request) {
		var out []publication.Candidate
		for id, state := range states {
			c := publication.Candidate{ID: id, State: state, Approved: state != publication.Queued, Proposal: publication.Proposal{Privacy: "private"}}
			if state == publication.Published {
				c.ProviderID, c.ProviderURL = "yt-"+id[:1], "https://youtu.be/"+id[:1]
			}
			out = append(out, c)
		}
		writeJSON(w, http.StatusOK, map[string]any{"candidates": out})
	})
	mux.HandleFunc("POST /v1/runs/{id}/studio/publications/{candidate}/{action}", func(w http.ResponseWriter, r *http.Request) {
		id, action := r.PathValue("candidate"), r.PathValue("action")
		actions = append(actions, action+":"+id[:1])
		switch {
		case action == "approve":
			states[id] = publication.Approved
		case pacing:
			writeJSON(w, http.StatusConflict, map[string]string{"error": "publication: channel minimum upload interval not elapsed"})
			return
		default:
			states[id] = publication.Publishing
		}
		writeJSON(w, http.StatusOK, map[string]string{})
	})

	s := newReplayServer("http://wall.invalid", "", "", nil)
	st := productionState{Version: 1, RunID: "run-e2e", Stage: "ready_for_review", AssessmentHash: h('a'), Verdict: "full_episode",
		Plans: map[string]string{"episode": h('b'), "short": h('b')}, Candidates: map[string]string{"episode": h('e'), "short": h('f')}, UploadPrivate: true}
	step := func() {
		t.Helper()
		if err := s.productionStep(context.Background(), mux, &st); err != nil {
			t.Fatalf("step at %s: %v", st.Stage, err)
		}
	}

	step() // approve episode
	step() // publish episode
	if st.Stage != "episode_upload" || st.Done || strings.Join(actions, ",") != "approve:e,publish:e" {
		t.Fatalf("episode dispatch: %+v %v", st, actions)
	}
	states[h('e')] = publication.Published
	step() // short approved
	pacing = true
	step() // channel pacing is a wait, not a failure
	if st.Stage != "short_upload" || st.Done || st.Videos["episode"] != "https://youtu.be/e" {
		t.Fatalf("pacing wait: %+v", st)
	}
	pacing = false
	step()
	states[h('f')] = publication.Published
	step()
	if !st.Done || st.Stage != "published_private" || len(st.Videos) != 2 {
		t.Fatalf("final: %+v", st)
	}

	// A gate rejection is terminal, never retried for hours.
	states = map[string]publication.State{h('e'): publication.Blocked}
	st = productionState{Version: 1, RunID: "run-e2e", AssessmentHash: h('a'), Verdict: "full_episode",
		Plans: map[string]string{"episode": h('b'), "short": h('b')}, Candidates: map[string]string{"episode": h('e'), "short": h('f')}, UploadPrivate: true}
	if err := s.productionStep(context.Background(), mux, &st); err == nil || !st.Done || st.Stage != "failed_episode_upload" {
		t.Fatalf("blocked candidate: %+v %v", st, err)
	}
}

func TestProductionRequestOptsInOnlyWithConfiguredUploader(t *testing.T) {
	_, store := newMemS3(t)
	s := newReplayServer("http://wall.invalid", "", "", store)
	ctx := context.Background()
	body := map[string]bool{"upload_private": true}
	if code, _ := selfJSON(ctx, s.handler(), http.MethodPost, "/v1/runs/run-e2e/production", body, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("opt-in without uploader = %d, want 503", code)
	}
	s.youtube = youtubeConfig{ClientID: "id", ClientSecret: "secret", RefreshToken: "token"}
	if err := s.saveProduction(ctx, productionState{Version: 1, RunID: "run-e2e", Stage: "ready_for_review", Done: true}); err != nil {
		t.Fatal(err)
	}
	var st productionState
	if _, err := selfJSON(ctx, s.handler(), http.MethodPost, "/v1/runs/run-e2e/production", body, &st); err != nil {
		t.Fatal(err)
	}
	if !st.UploadPrivate || st.Done {
		t.Fatalf("opt-in on a reviewed run must resume it for upload: %+v", st)
	}
}
