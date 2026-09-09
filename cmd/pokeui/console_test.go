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

func TestUIRunConsoleHasWatchingFirstWorkspace(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		`href="/console.css"`,
		`role="tablist"`,
		`data-view="live"`,
		`data-view="runs"`,
		`data-view="failures"`,
		`data-view="operations"`,
		`data-view="tools"`,
		`id="system-summary"`,
		`id="run-rail"`,
		`id="selected-run"`,
		`id="visual-stage"`,
		`id="run-timeline"`,
		`id="run-story"`,
		`id="evidence-drawer"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("operator console missing %q", want)
		}
	}
}

func TestUIRunConsoleUsesContextualActions(t *testing.T) {
	js := string(inspectorJS)
	for _, want := range []string{
		"Generate replay",
		"Return to live",
		"Start a new run from here",
		"Investigate with AI",
		"Recorded events",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("inspector.js missing %q", want)
		}
	}
}

func TestUIInspectorCannotMissInitialRunSelection(t *testing.T) {
	ui := string(uiJS)
	inspector := string(inspectorJS)
	if !strings.Contains(ui, `dataset.selectedRun = selected`) {
		t.Error("dashboard renderer must publish the current selection for late-loaded console modules")
	}
	if !strings.Contains(inspector, `dataset.selectedRun`) {
		t.Error("inspector must recover a selection dispatched before its script loaded")
	}
}

func TestUIReplayTimelineIsSeekable(t *testing.T) {
	js := string(inspectorJS)
	for _, want := range []string{`id="pp-scrubber"`, `type="range"`, `video.currentTime`, `video.duration`} {
		if !strings.Contains(js, want) {
			t.Errorf("replay transport missing %q", want)
		}
	}
}

func TestUIFollowsWatchingFirstReference(t *testing.T) {
	html := string(indexHTML)
	js := string(uiJS)
	inspector := string(inspectorJS)
	if strings.Contains(html, "brand-mark") {
		t.Error("header must use the restrained PokeFarm wordmark without an invented logo")
	}
	for _, want := range []string{`Run state`, `id="detail-location"`, `id="detail-objective"`, `id="detail-decision"`, `id="detail-frame"`, `id="detail-round"`} {
		if !strings.Contains(html, want) {
			t.Errorf("watching-first state strip missing %q", want)
		}
	}
	for _, want := range []string{`detail-location`, `detail-objective`, `detail-decision`, `detail-frame`, `detail-round`} {
		if !strings.Contains(js, want) {
			t.Errorf("dashboard renderer missing %q", want)
		}
	}
	if !strings.Contains(inspector, `id="pp-story-actions"`) {
		t.Error("Run Story must keep contextual actions in its header")
	}
}

func TestUIDetailDeckRetainsDebuggingInformation(t *testing.T) {
	html := string(indexHTML)
	js := string(uiJS)
	if strings.Contains(html, `id="detail-body" hidden`) {
		t.Error("current run detail deck must remain visible")
	}
	for _, want := range []string{`<h3>Settings</h3>`, `<h3>Outcome</h3>`, `planHTML(run)`, `playHTML(run)`, `partyHTML(run)`, `lastEventHTML(run)`} {
		if !strings.Contains(js, want) {
			t.Errorf("current run detail deck missing %q", want)
		}
	}
}

func TestUIReplayActionStaysVisibleWhenRecordingIsMissing(t *testing.T) {
	js := string(inspectorJS)
	for _, want := range []string{`replayButton.hidden=false`, `replayButton.disabled=true`, `Generate replay`, `Available after this run finishes`, `run.gbrun`} {
		if !strings.Contains(js, want) {
			t.Errorf("missing-recording replay state missing %q", want)
		}
	}
}

func TestUIGameFrameFitsInsideFixedStage(t *testing.T) {
	css := string(consoleCSS)
	rule := regexp.MustCompile(`\.game-monitor \.lcd img\{[^}]+\}`).FindString(css)
	if rule == "" {
		t.Fatal("console.css missing a game-frame sizing rule")
	}
	for _, want := range []string{"position:absolute", "inset:0", "object-fit:contain", "object-position:center"} {
		if !strings.Contains(rule, want) {
			t.Errorf("game-frame rule %q missing %q", rule, want)
		}
	}
}

func TestUIConsoleStylesheetIsServed(t *testing.T) {
	h := handler("http://wall.invalid")
	req := httptest.NewRequest(http.MethodGet, "/console.css", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET /console.css = %d, want 200", res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/css; charset=utf-8", got)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
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
	for _, want := range []string{`settingChips`, `statusChip`, `detail-objective`, `histFilter`, `ended_at`, `random_seed`} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
}

// TestUIShowsLatestPlan is the watch-pane half of the heartbeat question
// and decision: a one-line emulator trace is not the plan.
func TestUIShowsLatestPlan(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{`run.question`, `run.decision`, `detail-decision`} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
}

func TestUIInspectorLivesWithSelectedRun(t *testing.T) {
	html := string(indexHTML)
	if !strings.Contains(html, `id="run-inspector"`) {
		t.Error("selected run must host the run inspector")
	}
	if regexp.MustCompile(`(?s)id="selected-run".*id="run-inspector"`).FindString(html) == "" {
		t.Error("run inspector must be inside the selected-run workspace")
	}
	js := string(inspectorJS)
	for _, want := range []string{`id="run-timeline"`, `id="run-story"`, `id="evidence-drawer"`, `pokefarm-select-run`} {
		if !strings.Contains(js, want) {
			t.Errorf("inspector.js missing %q", want)
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

func TestUIHistoryShowsReplayAvailability(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{
		"replayChip",
		"replay_available",
		`chip("replay", "replay")`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
	css := string(consoleCSS)
	if !strings.Contains(css, `.chip.replay`) {
		t.Error("console.css missing .chip.replay so the history badge cannot stand out")
	}
	inspector := string(inspectorJS)
	if !strings.Contains(inspector, "replay/status") {
		t.Error("selected-run transport must load replay availability")
	}
}

// TestUIRendersLLMStats: the console shows the same planner tally the
// runner's watch page renders — a line on each live llm card and a Play
// block in the detail pane — so a wandering run is visible without opening
// port 8099.
func TestUIRendersLLMStats(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{
		"statsLine", "playHTML", "llmProfileLabel", "llm_profile",
		`r.stats`,
		`repeat picks`,
		`row("model"`,
		`round ${s.round}`,
		`${s.avg_offered.toFixed(1)} avg`,
		`pbar`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
	html := string(consoleCSS)
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
	html := string(consoleCSS)
	if !strings.Contains(string(indexHTML), `id="hist-pager"`) {
		t.Error("index.html missing history pager")
	}
	for _, want := range []string{`.pager`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html missing %s", want)
		}
	}
	// The row grid must be content-independent. Fixed length minima (for
	// example minmax(8rem, 1fr)) are safe; content-sized tracks such as auto,
	// min-content, or max-content let individual row contents move boundaries.
	if m := regexp.MustCompile(`\.hist\{[^}]*grid-template-columns:([^;]*);`).FindStringSubmatch(html); m == nil {
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

func TestUIWatchUsesPairedVisualStageAndCompactStateDeck(t *testing.T) {
	css := string(consoleCSS)
	body := regexp.MustCompile(`#detail-body\{[^}]+\}`).FindString(css)
	if body == "" {
		t.Fatal("console.css missing #detail-body rule")
	}
	if !strings.Contains(body, "repeat(4,minmax(0,1fr))") {
		t.Errorf("#detail-body %q must use a compact four-column state deck", body)
	}
	scroll := regexp.MustCompile(`\.block\.scroll\{[^}]+\}`).FindString(css)
	if !strings.Contains(scroll, "max-height:") || !strings.Contains(scroll, "overflow:auto") {
		t.Errorf(".block.scroll %q must cap height and scroll", scroll)
	}
	js := string(uiJS)
	for _, want := range []string{`class="block compact"`, `class="block scroll"`, `fpsLabel`, `updateFpsLive`, `paintHTML`, `holding`} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
	grid := regexp.MustCompile(`\.visual-stage\{[^}]+\}`).FindString(css)
	if !strings.Contains(grid, "grid-template-columns") {
		t.Errorf(".visual-stage %q must pair the game and semantic map", grid)
	}
	if !strings.Contains(string(indexHTML), `id="detail-party"`) {
		t.Error("party must sit outside #detail-body so it does not take a 1fr track")
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
		`Investigate with AI`,
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

func TestUIRendersPlayerRoster(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{
		"partyHTML",
		`r.player`,
		`<h3>Party</h3>`,
		"no Pokémon yet",
		"no badges",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
	start := strings.Index(js, "function renderLive")
	end := strings.Index(js, "function renderWorkers")
	if start < 0 || end <= start {
		t.Fatal("renderLive bounds")
	}
	if strings.Contains(js[start:end], "partyHTML") {
		t.Error("live cards must not render the party roster")
	}
	css := string(consoleCSS)
	for _, want := range []string{`.party-row`, `.party-hp`, `.party-sum`, `.party-grid`, `id="detail-party"`} {
		if want == `id="detail-party"` {
			if !strings.Contains(string(indexHTML), want) {
				t.Errorf("index.html missing %s", want)
			}
		} else if !strings.Contains(css, want) {
			t.Errorf("console.css missing %s", want)
		}
	}
	if !strings.Contains(css, `grid-template-columns:repeat(6,minmax(0,1fr))`) {
		t.Error("desktop party strip must show six slots in one compact row")
	}
	if !strings.Contains(js, `$("detail-party")`) && !strings.Contains(js, `$("detail-party").innerHTML`) {
		t.Error("ui.js must render the roster into #detail-party, not a 1fr grid cell")
	}
}
