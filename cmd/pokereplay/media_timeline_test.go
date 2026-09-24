package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestMediaTimelineLoadsInlineArtifact(t *testing.T) {
	artifact, err := farm.NewMediaTimelineArtifact(farm.MediaTimeline{
		Run:       farm.MediaRunSummary{RunID: "run-1", Status: "done"},
		EndFrame:  120,
		Snapshots: []farm.MediaSnapshot{{Frame: 60, Round: 1}},
		Events:    []farm.MediaEvent{{Type: "run_finished", Frame: 120, Evidence: "run:done"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/runs/run-1/artifacts":
			_ = json.NewEncoder(w).Encode(artifactList{
				RunID: "run-1", Attempt: 1,
				Artifacts: []artifactRef{{
					Name: artifact.Name, MediaType: artifact.MediaType, SHA256: artifact.SHA256, Inline: true,
				}},
			})
		case "/v1/runs/run-1/artifacts/media-timeline.json/content":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(artifact.Data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer wall.Close()

	server := newReplayServer(wall.URL, "", "", nil)
	timeline, err := server.mediaTimeline(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if timeline.Run.RunID != "run-1" || len(timeline.Events) != 1 || timeline.Events[0].ID == "" {
		t.Fatalf("timeline = %#v", timeline)
	}
}

func TestMediaTimelineMissingIsNonFatalSignal(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(artifactList{RunID: "old-run", Attempt: 1})
	}))
	defer wall.Close()

	server := newReplayServer(wall.URL, "", "", nil)
	_, err := server.mediaTimeline(context.Background(), "old-run")
	if !errors.Is(err, errMediaTimelineNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func TestMediaTimelineRejectsRunMismatch(t *testing.T) {
	artifact, err := farm.NewMediaTimelineArtifact(farm.MediaTimeline{
		Run: farm.MediaRunSummary{RunID: "other-run"}, EndFrame: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/runs/run-1/artifacts" {
			_ = json.NewEncoder(w).Encode(artifactList{RunID: "run-1", Artifacts: []artifactRef{{Name: artifact.Name, SHA256: artifact.SHA256, Inline: true}}})
			return
		}
		_, _ = w.Write(artifact.Data)
	}))
	defer wall.Close()
	server := newReplayServer(wall.URL, "", "", nil)
	if _, err := server.mediaTimeline(context.Background(), "run-1"); err == nil {
		t.Fatal("expected run mismatch")
	}
}

func TestMediaTimelineAttemptUsesMatchingArtifactGeneration(t *testing.T) {
	artifact, err := farm.NewMediaTimelineArtifact(farm.MediaTimeline{
		Run:     farm.MediaRunSummary{RunID: "run-resume"},
		Attempt: 2,
		EndFrame: 120,
	})
	if err != nil {
		t.Fatal(err)
	}
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("attempt") != "2" {
			t.Fatalf("attempt query=%q, want 2 for %s", r.URL.Query().Get("attempt"), r.URL.Path)
		}
		switch r.URL.Path {
		case "/v1/runs/run-resume/artifacts":
			_ = json.NewEncoder(w).Encode(artifactList{
				RunID: "run-resume", Attempt: 2,
				Artifacts: []artifactRef{{Name: artifact.Name, MediaType: artifact.MediaType, SHA256: artifact.SHA256, Inline: true}},
			})
		case "/v1/runs/run-resume/artifacts/media-timeline.json/content":
			_, _ = w.Write(artifact.Data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer wall.Close()

	server := newReplayServer(wall.URL, "", "", nil)
	timeline, err := server.mediaTimelineAttempt(context.Background(), "run-resume", 2)
	if err != nil {
		t.Fatal(err)
	}
	if timeline.Attempt != 2 {
		t.Fatalf("timeline attempt=%d, want 2", timeline.Attempt)
	}
}
