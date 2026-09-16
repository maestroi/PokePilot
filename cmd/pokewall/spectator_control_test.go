package main

import (
	"context"
	"strings"
	"testing"
)

func TestSpectatorControlMigrationCreatesDurableState(t *testing.T) {
	for _, table := range []string{"spectator_run_settings", "spectator_state"} {
		if !strings.Contains(controlPlaneMigration003, "TABLE IF NOT EXISTS "+table) {
			t.Fatalf("spectator migration missing %s", table)
		}
	}
}

func TestSpectatorRunControlDefaultsVisibleAndFeatureForcesVisible(t *testing.T) {
	wall := NewWall("")
	wall.tiles["run-1"] = &Tile{RunID: "run-1"}

	hidden := false
	result, err := wall.patchSpectatorRunControl(context.Background(), "run-1", spectatorRunControlPatch{Visible: &hidden})
	if err != nil {
		t.Fatalf("hide run: %v", err)
	}
	if result.Visible || result.Featured {
		t.Fatalf("hidden result = %+v", result)
	}

	featured := true
	result, err = wall.patchSpectatorRunControl(context.Background(), "run-1", spectatorRunControlPatch{Featured: &featured})
	if err != nil {
		t.Fatalf("feature run: %v", err)
	}
	if !result.Visible || !result.Featured || result.FeaturedRunID != "run-1" {
		t.Fatalf("featured result = %+v", result)
	}

	snapshot, err := wall.spectatorControlSnapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.FeaturedRunID != "run-1" || !snapshot.Runs["run-1"].Visible {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestHidingFeaturedRunClearsFeature(t *testing.T) {
	wall := NewWall("")
	wall.tiles["run-1"] = &Tile{RunID: "run-1"}

	featured := true
	if _, err := wall.patchSpectatorRunControl(context.Background(), "run-1", spectatorRunControlPatch{Featured: &featured}); err != nil {
		t.Fatalf("feature run: %v", err)
	}
	hidden := false
	result, err := wall.patchSpectatorRunControl(context.Background(), "run-1", spectatorRunControlPatch{Visible: &hidden})
	if err != nil {
		t.Fatalf("hide featured run: %v", err)
	}
	if result.Featured || result.FeaturedRunID != "" || result.Visible {
		t.Fatalf("result = %+v", result)
	}
}
