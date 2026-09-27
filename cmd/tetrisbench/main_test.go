package main

import (
	"testing"
	"time"

	"github.com/maestroi/pokepilot/tetris"
	"github.com/maestroi/pokepilot/tetris/policy"
	"github.com/maestroi/pokepilot/tetris/session"
)

func TestParseConfigProfiles(t *testing.T) {
	fast, err := parseConfig([]string{"-rom", "tetris.gb"})
	if err != nil {
		t.Fatal(err)
	}
	if fast.Profile != "fast" || fast.Goal != "score:1000" || fast.Runs != 1 || fast.MaxPieces != 120 || fast.Decision != "jev" || fast.DecisionMode != "active" {
		t.Fatalf("fast = %+v", fast)
	}

	full, err := parseConfig([]string{"-rom", "tetris.gb", "-profile", "full"})
	if err != nil {
		t.Fatal(err)
	}
	if full.Goal != "score:10000" || full.Runs != 3 || full.MaxPieces != 600 || full.MaxFrames != 900000 {
		t.Fatalf("full = %+v", full)
	}
}

func TestParseConfigRejectsBadInputs(t *testing.T) {
	for _, args := range [][]string{
		{"-rom", "tetris.gb", "-profile", "unknown"},
		{"-rom", "tetris.gb", "-decision-mode", "maybe"},
		{"-rom", "tetris.gb", "-min-confidence", "1.5"},
		{"-rom", "tetris.gb", "-goal", "score:nope"},
	} {
		if _, err := parseConfig(args); err == nil {
			t.Fatalf("parseConfig(%v) unexpectedly succeeded", args)
		}
	}
}

func TestSummarizeRequiresEveryQualificationRun(t *testing.T) {
	out := report{Runs: []runResult{
		{Passed: true, Score: 1000, Lines: 4, Pieces: 30},
		{Passed: false, Score: 500, Lines: 1, Pieces: 12},
		{Passed: true, Score: 1500, Lines: 6, Pieces: 44},
	}}
	summarize(&out, []time.Duration{20 * time.Millisecond, 10 * time.Millisecond, 40 * time.Millisecond})
	if out.Passed || out.CompletionRate != 2.0/3.0 {
		t.Fatalf("pass summary = passed %v rate %v", out.Passed, out.CompletionRate)
	}
	if out.MedianScore != 1000 || out.MedianLines != 4 || out.MedianPieces != 30 {
		t.Fatalf("medians = score %.0f lines %.0f pieces %.0f", out.MedianScore, out.MedianLines, out.MedianPieces)
	}
	if out.DecisionP50MS != 20 || out.DecisionP95MS != 40 {
		t.Fatalf("decision percentiles = %.1f/%.1f", out.DecisionP50MS, out.DecisionP95MS)
	}
}

func TestQualificationPassedUsesGoalOrSurvivalBudget(t *testing.T) {
	scoreGoal, err := session.ParseGoal("score:1000")
	if err != nil {
		t.Fatal(err)
	}
	done := session.Result{Reason: "done", State: tetris.State{Mode: tetris.ModeA, Score: 1000, ScoreValid: true}}
	if !qualificationPassed(scoreGoal, done, 100) {
		t.Fatal("completed score goal did not qualify")
	}

	survival := session.Goal{Kind: session.GoalSurvival, Objective: policy.ObjectiveSurvival}
	budget := session.Result{Reason: "budget", Pieces: 100, State: tetris.State{}}
	if !qualificationPassed(survival, budget, 100) {
		t.Fatal("survival budget did not qualify")
	}
	budget.Pieces = 99
	if qualificationPassed(survival, budget, 100) {
		t.Fatal("short survival run qualified")
	}
}

func TestSeedBurnIsDeterministicAndBounded(t *testing.T) {
	if got := seedBurn(0); got != 0 {
		t.Fatalf("seed 0 burn = %d, want 0", got)
	}
	a, b := seedBurn(7), seedBurn(7)
	if a != b || a < 0 || a >= 600 {
		t.Fatalf("seed 7 burn = %d/%d", a, b)
	}
}
