package producer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func sampleInput() Input {
	t := farm.MediaTimeline{
		Run: farm.MediaRunSummary{RunID: "run-123", Status: "done", Goal: "Beat the second gym", Planner: "llm"},
		Attempt: 1, EndFrame: 9000, FramesPerSecond: 60,
		Events: []farm.MediaEvent{
			{Type: "gym_battle", Frame: 1000, Summary: "Gym leader defeated", Evidence: "battle:1:won"},
			{Type: "badge_acquired", Frame: 1200, Summary: "Cascade Badge", Evidence: "badge:cascade"},
			{Type: "blackout", Frame: 3000, Summary: "Party wiped", Evidence: "blackout:3"},
			{Type: "planner_recovery", Frame: 4000, Summary: "Returned with a new plan", Evidence: "replan:4"},
			{Type: "run_finished", Frame: 7000, Summary: "Goal completed", Evidence: "finish:7"},
		},
	}.Normalized()
	return Input{RunID:"run-123", Timelines: []farm.MediaTimeline{t}}
}

type fakeModel struct {
	body []byte
	err error
}

func (m fakeModel) Identity() string { return "test-model:revision-1" }
func (m fakeModel) Generate(_ context.Context, _ EvidencePack) ([]byte, error) { return m.body, m.err }

func modelFor(t *testing.T, in Input, publication PublicationClass) fakeModel {
	t.Helper()
	pack, _, err := evidence(in)
	if err != nil { t.Fatal(err) }
	c := Candidate{
		PrimaryStory: "Defeated the gym leader and recovered after a blackout",
		Publication: publication,
		Beats: []Beat{
			{Kind:"hook", Summary:"Gym battle", Evidence:[]EvidenceRef{pack.Events[0].EvidenceRef}},
			{Kind:"crisis", Summary:"Blackout", Evidence:[]EvidenceRef{pack.Events[2].EvidenceRef}},
		},
	}
	data, err := json.Marshal(c)
	if err != nil { t.Fatal(err) }
	return fakeModel{body:data}
}

func TestRulesAssessmentIsDeterministicAndValid(t *testing.T) {
	in := sampleInput()
	a, err := Assess(context.Background(), in, nil, DefaultPolicy())
	if err != nil { t.Fatal(err) }
	b, err := Assess(context.Background(), in, nil, DefaultPolicy())
	if err != nil { t.Fatal(err) }
	if a.Hash != b.Hash || a.InputSHA256 != b.InputSHA256 { t.Fatalf("non-deterministic output: %q != %q", a.Hash, b.Hash) }
	if a.Publication != FullEpisode || len(a.Beats) == 0 { t.Fatalf("expected evidenced episode candidate: %+v", a) }
	if a.Model != "rules-v1" || a.PromptVersion != "none" { t.Fatalf("invalid offline identity: %+v", a) }
	if err := Validate(a, in); err != nil { t.Fatal(err) }
	a.Beats[0].Evidence[0].Frame++
	if err := Validate(a, in); err == nil { t.Fatal("accepted tampered evidence") }
}

func TestModelEvidenceValidationAndCorruptJSON(t *testing.T) {
	in := sampleInput()
	if _, err := Assess(context.Background(), in, fakeModel{body:[]byte("{bad")}, DefaultPolicy()); err == nil {
		t.Fatal("malformed model JSON was accepted")
	}
	if _, err := Assess(context.Background(), in, fakeModel{err:errors.New("offline")}, DefaultPolicy()); err == nil {
		t.Fatal("missing model output was accepted")
	}
	grounded := modelFor(t, in, FullEpisode)
	assessment, err := Assess(context.Background(), in, grounded, DefaultPolicy())
	if err != nil { t.Fatal(err) }
	if assessment.Model != grounded.Identity() || assessment.PromptVersion != PromptVersion { t.Fatal("model version not recorded") }
	if err := Validate(assessment, in); err != nil { t.Fatal(err) }

	pack, _, _ := evidence(in)
	c := Candidate{PrimaryStory:"Claim", Publication:FullEpisode,
		Beats:[]Beat{{Kind:"climax",Summary:"Made up",Evidence:[]EvidenceRef{
			{Attempt:1,EventID:pack.Events[0].EventID,Frame:99999,TimestampMS:pack.Events[0].TimestampMS},
		}}},
	}
	data, _ := json.Marshal(c)
	if _, err := Assess(context.Background(), in, fakeModel{body:data}, DefaultPolicy()); err == nil || !strings.Contains(err.Error(),"mismatched") {
		t.Fatalf("forged source ref accepted: %v", err)
	}
}

func TestMissingTelemetryAndDoNotPublish(t *testing.T) {
	in := Input{RunID:"legacy-run"}
	assessment, err := Assess(context.Background(), in, nil, DefaultPolicy())
	if err != nil { t.Fatal(err) }
	if assessment.Publication != DoNotPublish || len(assessment.Beats) != 0 || assessment.PrimaryStory != "" {
		t.Fatalf("no telemetry should yield no publishable story: %+v", assessment)
	}
	if err := Validate(assessment, in); err != nil { t.Fatal(err) }
	declined, err := Assess(context.Background(), sampleInput(), modelFor(t, sampleInput(), DoNotPublish), DefaultPolicy())
	if err != nil { t.Fatal(err) }
	if declined.Publication != DoNotPublish { t.Fatalf("producer declined publication, got %s", declined.Publication) }
}

func TestRepetitivePublishedRunIsArchived(t *testing.T) {
	in := sampleInput()
	in.History = []HistoricalRun{{
		RunID: "older-run", Source: "farm-run-summary:older-run@v1", Goal:"Beat the second gym",
		DurationMS: 200000, Published:true,
		EventTypes: []string{"gym_battle","badge_acquired","blackout","planner_recovery","run_finished"},
	}}
	out, err := Assess(context.Background(), in, modelFor(t,in,FullEpisode), DefaultPolicy())
	if err != nil { t.Fatal(err) }
	if out.Publication != ArchiveOnly || out.Scores.RecentSimilarity == nil ||
		out.Scores.RecentSimilarity.Value != 100 || len(out.Comparisons) != 2 {
		t.Fatalf("repetition gate or sourced comparison failed: %+v",out)
	}
	if err := Validate(out, in); err != nil { t.Fatal(err) }
	in.History[0].Source = "" // provenance may not be fabricated by a model
	if _, err := Assess(context.Background(), in, nil, DefaultPolicy()); err == nil {
		t.Fatal("accepted unsourced historical data")
	}
}

func TestOlderRunsMissingTerminalStateNeverPromote(t *testing.T) {
	in := sampleInput()
	in.Timelines[0].Run.Status = ""
	out, err := Assess(context.Background(), in, modelFor(t,in,FullEpisode), DefaultPolicy())
	if err != nil { t.Fatal(err) }
	if out.Publication != ArchiveOnly { t.Fatalf("unverified terminal status promoted: %s",out.Publication) }
}
