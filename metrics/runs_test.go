package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestRunLabelsStayBounded(t *testing.T) {
	if got := NormalizeGame(""); got != "unknown" {
		t.Fatalf("empty game = %q", got)
	}
	if got := NormalizeGame("Pokemon-Red"); got != "pokemon-red" {
		t.Fatalf("game = %q", got)
	}
	if got := NormalizeGame("run id"); got != "unknown" {
		t.Fatalf("unsafe game = %q", got)
	}
	if got := NormalizeMode("LLM"); got != "llm" {
		t.Fatalf("mode = %q", got)
	}
	if got := NormalizeMode("custom-planner"); got != "other" {
		t.Fatalf("mode = %q", got)
	}
	if id, ok := StoryMilestone("Pokédex"); !ok || id != "pokedex" {
		t.Fatalf("Pokédex = %q %v", id, ok)
	}
	if _, ok := StoryMilestone("route 1 npc"); ok {
		t.Fatal("accepted an unbounded milestone label")
	}
	if id, ok := BadgeMilestone(1); !ok || id != "badge_1" {
		t.Fatalf("badge 1 = %q %v", id, ok)
	}
	if _, ok := BadgeMilestone(MaxBadgeMilestones + 1); ok {
		t.Fatal("accepted a badge past the cap")
	}
	if got := FurthestMilestone(nil); got != "started" {
		t.Fatalf("furthest empty = %q", got)
	}
	if got := FurthestMilestone([]string{"starter", "badge_2", "pokedex"}); got != "pokedex" {
		t.Fatalf("furthest = %q", got)
	}
	if got := RunResult("done", false); got != ResultCompleted {
		t.Fatalf("done = %q", got)
	}
	if got := RunResult("budget", true); got != ResultAborted {
		t.Fatalf("cancelled budget = %q", got)
	}
	if got := RunResult("budget", false); got != ResultFailed {
		t.Fatalf("budget = %q", got)
	}
}

func TestRunSeriesAreScrapable(t *testing.T) {
	reg := Must(Options{Service: ServiceWall, Revision: "abc123"})
	runs := MustRuns(reg)
	runs.Started("pokemon-red", "llm")
	runs.MarkActive("pokemon-red")
	runs.Milestone("pokemon-red", "starter")
	runs.Milestone("pokemon-red", "not-a-stage")
	at := time.Unix(1_700_000_000, 0)
	runs.Finished(RunFinish{
		Game: "pokemon-red", Mode: "llm", Result: ResultCompleted,
		Milestone: "starter", Duration: 2 * time.Hour, HasDuration: true,
		Rounds: 12, Frames: 200_000, At: at,
	})
	runs.ClearActive("pokemon-red")
	runs.NoteSuccess("pokemon-red", float64(at.Add(-time.Hour).Unix()))

	body := scrape(t, reg.Handler())
	for _, want := range []string{
		`pokepilot_runs_started_total{game="pokemon-red",mode="llm"} 1`,
		`pokepilot_runs_completed_total{game="pokemon-red",mode="llm"} 1`,
		`pokepilot_run_milestones_total{game="pokemon-red",milestone="starter"} 1`,
		`pokepilot_run_stopping_milestone_total{game="pokemon-red",milestone="starter",result="completed"} 1`,
		`pokepilot_last_successful_run_timestamp_seconds{game="pokemon-red"} 1.7e+09`,
		`le="7200"`,
		`le="86400"`,
		"pokepilot_run_rounds_count",
		"pokepilot_run_frames_count",
		"pokepilot_run_duration_seconds_count",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("exposition missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "not-a-stage") || strings.Contains(body, "run_id=") {
		t.Fatalf("exposition leaked an unbounded label:\n%s", body)
	}
	if strings.Contains(body, `pokepilot_runs_active{game="pokemon-red"} 1`) {
		t.Fatalf("active gauge was not cleared:\n%s", body)
	}
}

func TestLastSuccessDoesNotMoveBackwards(t *testing.T) {
	reg := Must(Options{Service: ServiceRunner, Revision: "dev"})
	runs := MustRuns(reg)
	runs.NoteSuccess("tetris", 100)
	runs.NoteSuccess("tetris", 50)
	body := scrape(t, reg.Handler())
	if !strings.Contains(body, `pokepilot_last_successful_run_timestamp_seconds{game="tetris"} 100`) {
		t.Fatalf("last success moved backwards:\n%s", body)
	}
}
