package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The operator page used to fetch /frame in a tight loop with no delay
// on success. Chrome then spent a third of a core decoding 160x144 PNGs
// as fast as the overlay could answer — one loop per running card.
func TestUIFramePumpIsCapped(t *testing.T) {
	src := string(uiJS)
	m := regexp.MustCompile(`const frameMs = (\d+)`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("ui.js frame pump has no frameMs; unbounded fetch+decode burns the tab")
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("frameMs: %v", err)
	}
	if n < 50 {
		t.Fatalf("frameMs = %d, want >= 50 (20 fps cap; 0 is a tight loop)", n)
	}
}

func TestUIDoesNotKeepPumpingFinishedRuns(t *testing.T) {
	src := string(uiJS)
	// The live pump set is only running cards. A selected done run used
	// to stay in that set and re-decode the same last PNG forever.
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, `sel.status === "done"`) && strings.Contains(line, "want.add") {
			t.Fatal("selected done runs are in the live pump set")
		}
	}
}

func TestUISeparatesSettingsFromState(t *testing.T) {
	html := string(indexHTML)
	js := string(uiJS)
	if !strings.Contains(html, `id="hist-filters"`) {
		t.Error("index missing history filters")
	}
	if !strings.Contains(html, `id="watch"`) {
		t.Error("index missing watch theater")
	}
	if !strings.Contains(html, `name="goal"`) || !strings.Contains(html, `name="endless"`) {
		t.Error("index missing goal or endless fields")
	}
	if !strings.Contains(html, `id="detail-chips"`) {
		t.Error("index missing detail badge row")
	}
	for _, want := range []string{`settingChips`, `statusChip`, `<h3>Settings</h3>`, `Outcome`, `histFilter`, `ended_at`, `random_seed`} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
}

// TestUIShowsLatestPlan is the watch-pane half of the heartbeat question
// and decision: a one-line emulator trace is not the plan.
func TestUIShowsLatestPlan(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{`run.question`, `run.decision`, `<h3>Plan</h3>`} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
}

func TestUIHistoryCanBeDeletedAndNewestFirst(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{`data-delete`, `method: "DELETE"`} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
}

// TestUIRendersLLMStats: the console shows the same planner tally the
// runner's watch page renders — a line on each live llm card and a Play
// block in the detail pane — so a wandering run is visible without opening
// port 8099.
func TestUIRendersLLMStats(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{
		"statsLine", "playHTML",
		`r.stats`,
		`repeat picks`,
		`round ${s.round}`,
		`${s.avg_offered.toFixed(1)} avg`,
		`pbar`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
	html := string(indexHTML)
	for _, want := range []string{`.pnums`, `.pchoice`, `.pwarn`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html missing %s", want)
		}
	}
}

// TestUIHistoryPaginatedAndAligned: the history table lines its columns up
// across rows (no content-sized tracks) and pages the list so a long farm
// history does not render hundreds of rows at once.
func TestUIHistoryPaginatedAndAligned(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{
		"const HIST_PAGE = 25",
		"let histPage = 0",
		"hist-pager",
		`data-page="prev"`,
		`data-page="next"`,
		"filteredHistory()",
		".slice(",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
	// A filter change starts on the first page again.
	if !strings.Contains(js, "histPage = 0") {
		t.Error("ui.js does not reset the history page on filter change")
	}
	html := string(indexHTML)
	for _, want := range []string{`id="hist-pager"`, `.pager`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html missing %s", want)
		}
	}
	// The row grid must be content-independent. Fixed length minima (for
	// example minmax(8rem, 1fr)) are safe; content-sized tracks such as auto,
	// min-content, or max-content let individual row contents move boundaries.
	if m := regexp.MustCompile(`\.hist \{[^}]*grid-template-columns:([^;]*);`).FindStringSubmatch(html); m == nil {
		t.Fatal("index.html .hist has no grid-template-columns")
	} else {
		grid := strings.ToLower(m[1])
		for _, bad := range []string{"auto", "min-content", "max-content"} {
			if strings.Contains(grid, bad) {
				t.Errorf(".hist grid %q contains content-sized track %q; row columns may not align", m[1], bad)
			}
		}
	}
}

// TestUIWatchCardsShareGridTracks: Settings/Outcome/Plan/Play must share
// equal tracks instead of sizing to content, or a long Play list drops the
// row edge below the map.
func TestUIWatchCardsShareGridTracks(t *testing.T) {
	html := string(indexHTML)
	body := regexp.MustCompile(`#detail-body\{[^}]+\}`).FindString(html)
	if body == "" {
		t.Fatal("index.html missing #detail-body rule")
	}
	for _, want := range []string{"grid-auto-rows:minmax(0,1fr)", "align-items:stretch"} {
		if !strings.Contains(body, want) {
			t.Errorf("#detail-body %q missing %q", body, want)
		}
	}
	if strings.Contains(body, "align-items:start") {
		t.Error("#detail-body sizes cards to content")
	}
	block := regexp.MustCompile(`\.block\{[^}]+\}`).FindString(html)
	if !strings.Contains(block, "overflow:auto") || !strings.Contains(block, "min-height:0") {
		t.Errorf(".block %q must scroll inside a fixed cell", block)
	}
	grid := regexp.MustCompile(`\.watch-grid\{[^}]+\}`).FindString(html)
	if !strings.Contains(grid, "align-items:stretch") {
		t.Errorf(".watch-grid %q must stretch so the card grid matches the screens", grid)
	}
}

func TestUIFailuresAndIssueLinks(t *testing.T) {
	html := string(indexHTML)
	js := string(uiJS)
	if !strings.Contains(html, `id="failures"`) {
		t.Error("index missing Failures section")
	}
	for _, want := range []string{
		`/v1/triage`,
		`data-investigate`,
		`Investigate now`,
		`pending report`,
		`issueHref`,
		`new URL`,
		`investigating`,
		`issueBadge`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
	if strings.Contains(js, "/api/issues/") && strings.Contains(js, "issue_number") && strings.Contains(js, "`/issues/${") {
		t.Error("ui.js must not build Agent Orchestrator URLs from issue numbers")
	}
}

func TestVersionEndpoint(t *testing.T) {
	h := handler("http://wall.invalid")
	req := httptest.NewRequest(http.MethodGet, "/v1/version", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET /v1/version = %d, want 200: %s", res.Code, res.Body.String())
	}
	var got map[string]string
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["version"] == "" {
		t.Fatal("version missing from /v1/version")
	}
}
