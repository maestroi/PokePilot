package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestTileRowJSONExposesPlayStyle(t *testing.T) {
	farm.RememberPlayStyle("style-row", "adventure")
	b, err := json.Marshal(tileRow{RunID: "style-row", Planner: "llm"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"play_style":"adventure"`) {
		t.Fatalf("tile row json = %s, want play_style", b)
	}
}

func TestPersistedTileRestoresPlayStyle(t *testing.T) {
	farm.RememberPlayStyle("style-persist", "completionist")
	b, err := json.Marshal(persistedTile{RunID: "style-persist", Planner: "llm"})
	if err != nil {
		t.Fatal(err)
	}
	farm.RememberPlayStyle("style-persist", "")

	var got persistedTile
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if style := farm.PlayStyleForRun("style-persist"); style != "completionist" {
		t.Fatalf("restored style = %q, want completionist", style)
	}
}

func TestResumedChildInheritsParentPlayStyle(t *testing.T) {
	farm.RememberPlayStyle("style-parent-wall", "team_builder")
	row := tileRow{RunID: "style-child-wall", Planner: "llm", ResumeFromRunID: "style-parent-wall"}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"play_style":"team_builder"`) {
		t.Fatalf("child row json = %s, want inherited play_style", b)
	}
	if style := farm.PlayStyleForRun("style-child-wall"); style != "team_builder" {
		t.Fatalf("child registry style = %q, want team_builder", style)
	}
}
