package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// failedTile is a finished run that stopped for reason "error" with the
// given detail — the shape the wall's failure groups rank.
func failedTile(runID, detail string) *Tile {
	return &Tile{RunID: runID, Status: statusDone, Reason: "error", Detail: detail, Finished: true}
}

// wallWithFinished builds a wall holding exactly the given tiles, in order.
func wallWithFinished(tiles ...*Tile) *Wall {
	w := NewWall("")
	w.mu.Lock()
	for _, tile := range tiles {
		w.order = append(w.order, tile.RunID)
		w.tiles[tile.RunID] = tile
	}
	w.mu.Unlock()
	return w
}

func TestTriageGroups(t *testing.T) {
	tests := []struct {
		name       string
		tiles      []*Tile
		wantCounts []int
		wantPats   []string
	}{
		{
			name: "coordinates and hex map ids group",
			tiles: []*Tile{
				failedTile("run-a", "still on map 0x0c at (10,35)"),
				failedTile("run-b", "still on map 0x21 at (4,22)"),
			},
			wantCounts: []int{2},
			wantPats:   []string{normalizeDetail("still on map 0x0c at (10,35)")},
		},
		{
			name: "different errors do not group",
			tiles: []*Tile{
				failedTile("run-c", "connection edge 0c->00 via south did not cross within 180 frames"),
				failedTile("run-d", "walk to warp on map 0x0c: text box interrupted movement"),
			},
			wantCounts: []int{1, 1},
			wantPats: []string{
				normalizeDetail("connection edge 0c->00 via south did not cross within 180 frames"),
				normalizeDetail("walk to warp on map 0x0c: text box interrupted movement"),
			},
		},
		{
			name: "ordered by count descending",
			tiles: []*Tile{
				failedTile("run-1", "walk to warp on map 0x0c: text box interrupted movement at (7,9)"),
				failedTile("run-2", "agent: go to pallet town: skill: Travel: blacked out"),
				failedTile("run-3", "walk to warp on map 0x0c: text box interrupted movement at (11,4)"),
				failedTile("run-4", "walk to warp on map 0x21: text box interrupted movement at (2,30)"),
			},
			wantCounts: []int{3, 1},
			wantPats: []string{
				normalizeDetail("walk to warp on map 0x0c: text box interrupted movement at (7,9)"),
				normalizeDetail("agent: go to pallet town: skill: Travel: blacked out"),
			},
		},
		{
			name: "empty detail is in no group",
			tiles: []*Tile{
				failedTile("run-e", ""),
				failedTile("run-f", "agent: go to pallet town: skill: Travel: blacked out"),
			},
			wantCounts: []int{1},
			wantPats:   []string{normalizeDetail("agent: go to pallet town: skill: Travel: blacked out")},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wallWithFinished(tc.tiles...).triage()
			if len(got) != len(tc.wantCounts) {
				t.Fatalf("groups = %d (%+v), want %d", len(got), got, len(tc.wantCounts))
			}
			for i := range tc.wantCounts {
				if got[i].Count != tc.wantCounts[i] {
					t.Errorf("group %d count = %d, want %d (groups %+v)", i, got[i].Count, tc.wantCounts[i], got)
				}
				if got[i].Pattern != tc.wantPats[i] {
					t.Errorf("group %d pattern = %q, want %q", i, got[i].Pattern, tc.wantPats[i])
				}
			}
		})
	}
}

func TestTriageExampleIsVerbatim(t *testing.T) {
	details := []string{
		"still on map 0x0c at (10,35)",
		"still on map 0x21 at (4,22)",
	}
	tiles := []*Tile{failedTile("run-a", details[0]), failedTile("run-b", details[1])}
	got := wallWithFinished(tiles...).triage()
	if len(got) != 1 {
		t.Fatalf("groups = %d, want 1: %+v", len(got), got)
	}
	found := false
	for _, d := range details {
		if got[0].Example == d {
			found = true
		}
	}
	if !found {
		t.Errorf("example %q is not one of the group's verbatim details %v", got[0].Example, details)
	}
	if got[0].Example == normalizeDetail(details[0]) && got[0].Example != details[0] {
		t.Errorf("example %q looks normalised, want unmodified", got[0].Example)
	}
	if len(got[0].RunIDs) != 2 || got[0].RunIDs[0] != "run-b" || got[0].RunIDs[1] != "run-a" {
		t.Errorf("run ids = %v, want newest first [run-b run-a]", got[0].RunIDs)
	}
}

func TestTriageExcludesNonFailures(t *testing.T) {
	w := wallWithFinished(
		failedTile("ran", ""), // failed but no detail: not a cluster of one
		&Tile{RunID: "done-run", Status: statusDone, Reason: "done", Detail: "arrived", Finished: true},
		&Tile{RunID: "live-run", Status: statusRunning, Reason: "error", Detail: "blacked out"},
	)
	got := w.triage()
	if len(got) != 0 {
		t.Fatalf("groups = %+v, want none", got)
	}
}

func TestTriageRunIDsCapped(t *testing.T) {
	var tiles []*Tile
	for i := 0; i < 7; i++ {
		tiles = append(tiles, failedTile("run-x", "still on map 0x0c at (10,35)"))
	}
	got := wallWithFinished(tiles...).triage()
	if len(got) != 1 || got[0].Count != 7 {
		t.Fatalf("groups = %+v, want one group of count 7", got)
	}
	if len(got[0].RunIDs) != triageRunIDCap {
		t.Errorf("run ids = %d, want capped at %d", len(got[0].RunIDs), triageRunIDCap)
	}
}

func TestTriageEndpoint(t *testing.T) {
	w := wallWithFinished(
		failedTile("run-a", "still on map 0x0c at (10,35)"),
		failedTile("run-b", "still on map 0x21 at (4,22)"),
		failedTile("run-c", "agent: go to pallet town: skill: Travel: blacked out"),
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/triage", nil)
	res := httptest.NewRecorder()
	w.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET /v1/triage = %d: %s", res.Code, res.Body.String())
	}
	if ct := res.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got []triageGroup
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode triage: %v\n%s", err, res.Body.String())
	}
	want := w.triage()
	if len(got) != len(want) || got[0].Count != 2 || got[0].Pattern != want[0].Pattern || got[1].Count != 1 {
		t.Fatalf("triage = %+v, want %+v", got, want)
	}
}

func TestTriageOnGridPage(t *testing.T) {
	w := wallWithFinished(
		failedTile("run-a", "still on map 0x0c at (10,35)"),
		failedTile("run-b", "still on map 0x21 at (4,22)"),
	)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	w.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET / = %d: %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, want := range []string{
		"failure groups",
		"still on map 0x0c at (10,35)", // the verbatim example survives rendering
		// html/template escapes the <hex>/<n> placeholders in the pattern.
		"still on map &lt;hex&gt; at (&lt;n&gt;,&lt;n&gt;)",
		"run-a",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("grid page missing %q", want)
		}
	}
}
