package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func rememberTestRunPolicy(runID, style, risk, wild string) {
	farm.RememberPlayStyle(runID, style)
	farm.RememberRiskTolerance(runID, risk)
	farm.RememberWildEncounters(runID, wild)
}

func TestTileRowJSONExposesRunPolicy(t *testing.T) {
	rememberTestRunPolicy("style-row", "adventure", "balanced", "fight")
	b, err := json.Marshal(tileRow{RunID: "style-row", Planner: "llm"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"play_style":"adventure"`,
		`"risk_tolerance":"balanced"`,
		`"wild_encounters":"fight"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("tile row json = %s, want %s", b, want)
		}
	}
}

func TestPersistedTileRestoresRunPolicy(t *testing.T) {
	rememberTestRunPolicy("style-persist", "completionist", "cautious", "planner")
	b, err := json.Marshal(persistedTile{RunID: "style-persist", Planner: "llm"})
	if err != nil {
		t.Fatal(err)
	}
	rememberTestRunPolicy("style-persist", "", "", "")

	var got persistedTile
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if style := farm.PlayStyleForRun("style-persist"); style != "completionist" {
		t.Fatalf("restored style = %q, want completionist", style)
	}
	if risk := farm.RiskToleranceForRun("style-persist"); risk != "cautious" {
		t.Fatalf("restored risk = %q, want cautious", risk)
	}
	if wild := farm.WildEncountersForRun("style-persist"); wild != "planner" {
		t.Fatalf("restored wild policy = %q, want planner", wild)
	}
}

func TestResumedChildInheritsParentRunPolicy(t *testing.T) {
	rememberTestRunPolicy("style-parent-wall", "team_builder", "cautious", "fight")
	row := tileRow{RunID: "style-child-wall", Planner: "llm", ResumeFromRunID: "style-parent-wall"}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"play_style":"team_builder"`,
		`"risk_tolerance":"cautious"`,
		`"wild_encounters":"fight"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("child row json = %s, want %s", b, want)
		}
	}
	if style := farm.PlayStyleForRun("style-child-wall"); style != "team_builder" {
		t.Fatalf("child registry style = %q, want team_builder", style)
	}
	if risk := farm.RiskToleranceForRun("style-child-wall"); risk != "cautious" {
		t.Fatalf("child registry risk = %q, want cautious", risk)
	}
	if wild := farm.WildEncountersForRun("style-child-wall"); wild != "fight" {
		t.Fatalf("child registry wild = %q, want fight", wild)
	}
}
