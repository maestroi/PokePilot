package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTileRowJSONExposesRunPolicy pins that a run's gameplay policy rides the
// dashboard row as the tile's own fields.
func TestTileRowJSONExposesRunPolicy(t *testing.T) {
	row := tileRow{
		RunID:          "style-row",
		Planner:        "llm",
		PlayStyle:      "adventure",
		RiskTolerance:  "balanced",
		WildEncounters: "fight",
	}
	b, err := json.Marshal(row)
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

// TestTileRowJSONRestoresRunPolicy covers a catalog row read back after the
// run left RAM: the policy must be on the row itself, with no registry needed.
func TestTileRowJSONRestoresRunPolicy(t *testing.T) {
	b, err := json.Marshal(tileRow{
		RunID:          "style-catalog-row",
		Planner:        "llm",
		PlayStyle:      "team_builder",
		RiskTolerance:  "balanced",
		WildEncounters: "avoid",
	})
	if err != nil {
		t.Fatal(err)
	}

	var got tileRow
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.PlayStyle != "team_builder" {
		t.Fatalf("restored style = %q, want team_builder", got.PlayStyle)
	}
	if got.RiskTolerance != "balanced" {
		t.Fatalf("restored risk = %q, want balanced", got.RiskTolerance)
	}
	if got.WildEncounters != "avoid" {
		t.Fatalf("restored wild policy = %q, want avoid", got.WildEncounters)
	}
}

func TestPersistedTileRestoresRunPolicy(t *testing.T) {
	b, err := json.Marshal(persistedTile{
		RunID:          "style-persist",
		Planner:        "llm",
		PlayStyle:      "completionist",
		RiskTolerance:  "cautious",
		WildEncounters: "planner",
	})
	if err != nil {
		t.Fatal(err)
	}

	var got persistedTile
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.PlayStyle != "completionist" {
		t.Fatalf("restored style = %q, want completionist", got.PlayStyle)
	}
	if got.RiskTolerance != "cautious" {
		t.Fatalf("restored risk = %q, want cautious", got.RiskTolerance)
	}
	if got.WildEncounters != "planner" {
		t.Fatalf("restored wild policy = %q, want planner", got.WildEncounters)
	}
}

// TestTileRowLockedCopiesRunPolicy pins that the snapshot the dashboard and
// catalog are built from carries the tile's own policy.
func TestTileRowLockedCopiesRunPolicy(t *testing.T) {
	w := NewWall("")
	row := w.tileRowLocked(&Tile{
		RunID:          "style-snapshot",
		PlayStyle:      "adventure",
		RiskTolerance:  "balanced",
		WildEncounters: "fight",
	})
	if row.PlayStyle != "adventure" || row.RiskTolerance != "balanced" || row.WildEncounters != "fight" {
		t.Fatalf("row policy = %q/%q/%q", row.PlayStyle, row.RiskTolerance, row.WildEncounters)
	}
}

// TestResumedChildInheritsParentRunPolicy covers a child restored from an
// older state file that predates the policy fields on Tile: it inherits the
// parent's policy once, and only for fields it does not already set.
func TestResumedChildInheritsParentRunPolicy(t *testing.T) {
	w := NewWall("")
	w.tiles["style-parent-wall"] = &Tile{
		RunID:          "style-parent-wall",
		PlayStyle:      "team_builder",
		RiskTolerance:  "cautious",
		WildEncounters: "fight",
	}
	child := &Tile{
		RunID:           "style-child-wall",
		ResumeFromRunID: "style-parent-wall",
		PlayStyle:       "completionist",
	}
	w.tiles[child.RunID] = child

	w.inheritRunPolicyLocked(child)
	if child.PlayStyle != "completionist" {
		t.Fatalf("child overridden style = %q, want completionist", child.PlayStyle)
	}
	if child.RiskTolerance != "cautious" {
		t.Fatalf("inherited risk = %q, want cautious", child.RiskTolerance)
	}
	if child.WildEncounters != "fight" {
		t.Fatalf("inherited wild policy = %q, want fight", child.WildEncounters)
	}
}
