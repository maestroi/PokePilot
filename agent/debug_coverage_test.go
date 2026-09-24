package agent

import (
	"strings"
	"testing"
)

func TestNormalizeRunPurpose(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"", RunPurposeNormal},
		{"normal", RunPurposeNormal},
		{"debug_coverage", RunPurposeDebugCoverage},
		{"debug-coverage", RunPurposeDebugCoverage},
		{"coverage", RunPurposeDebugCoverage},
	} {
		if got := NormalizeRunPurpose(tc.in); got != tc.want {
			t.Fatalf("NormalizeRunPurpose(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDebugCoveragePurposeIsOrthogonalToPlayStyle(t *testing.T) {
	obs := Observation{Party: []PartyMon{{Species: "squirtle", HP: 30, MaxHP: 30}}}
	offered := []Objective{
		{Kind: KindTalk, X: 4, Y: 7},
		{Kind: KindProgress, Progress: "test_progress"},
	}

	for _, style := range []string{PlayStyleSpeedrun, PlayStyleAdventure, PlayStyleCompletionist, PlayStyleTeamBuilder} {
		styled := AnnotatePlayStyle(obs, offered, PlayStyle(style))
		got := AnnotateRunPurpose(obs, styled, RunPurposeDebugCoverage)
		if !strings.Contains(got[0].Note, "debug_coverage") || !strings.Contains(got[0].Note, "debug-new-npc") {
			t.Fatalf("style %q debug annotation = %q, want independent debug coverage signal", style, got[0].Note)
		}
		if strings.Contains(got[1].Note, "debug_coverage") {
			t.Fatalf("style %q progression received fake debug novelty: %q", style, got[1].Note)
		}
	}
}

func TestNormalPurposeIsExactNoOp(t *testing.T) {
	offered := []Objective{{Kind: KindTalk, X: 4, Y: 7, Note: "existing"}}
	got := AnnotateRunPurpose(Observation{}, offered, RunPurposeNormal)
	if len(got) != 1 || got[0] != offered[0] {
		t.Fatalf("normal purpose changed objectives: got %#v want %#v", got, offered)
	}
}

func TestRunPurposeSystemNoteSeparatesDebugFromCompletionist(t *testing.T) {
	if note := RunPurposeSystemNote(RunPurposeNormal); note != "" {
		t.Fatalf("normal purpose note = %q, want empty", note)
	}
	debug := RunPurposeSystemNote(RunPurposeDebugCoverage)
	if !strings.Contains(debug, "DEBUG COVERAGE") || !strings.Contains(debug, "little gameplay payoff") {
		t.Fatalf("debug purpose note = %q", debug)
	}
	completionist := PlayStyleSystemNote(PlayStyleCompletionist)
	if strings.Contains(completionist, "interaction-coverage testing") || !strings.Contains(completionist, "thorough human player") {
		t.Fatalf("completionist note still reads as debug mode: %q", completionist)
	}
}
