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
		`data-view="analytics"`,
		`data-view="operations"`,
		`data-view="tools"`,
		`data-console-view="analytics"`,
		`id="system-summary"`,
		`id="run-rail"`,
		`id="selected-run"`,
		`id="visual-stage"`,
		`id="run-timeline"`,
		`id="run-story"`,
		`id="evidence-drawer"`,
		`id="analytics-outcomes"`,
		`id="operations-health"`,
		`id="operations-workers"`,
		`id="operations-active"`,
		`id="operations-queue"`,
		`id="operations-recent"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("operator console missing %q", want)
		}
	}
}

func TestUIGameMediaKeepsStableFrameBox(t *testing.T) {
	css := string(consoleCSS)
	host := regexp.MustCompile(`\.game-media\{[^}]+\}`).FindString(css)
	for _, want := range []string{"position:relative", "flex:1", "min-height:0"} {
		if !strings.Contains(host, want) {
			t.Errorf(".game-media rule %q missing %q", host, want)
		}
	}
	lcd := regexp.MustCompile(`\.game-media>\.lcd\{[^}]+\}`).FindString(css)
	for _, want := range []string{"position:relative", "flex:1", "width:100%", "height:100%"} {
		if !strings.Contains(lcd, want) {
			t.Errorf(".game-media>.lcd rule %q missing %q", lcd, want)
		}
	}
}

func TestUITabsUseAccessibleKeyboardNavigation(t *testing.T) {
	html := string(indexHTML)
	for i, view := range []string{"live", "runs", "failures", "analytics", "operations", "tools"} {
		tabIndex := `-1`
		if i == 0 {
			tabIndex = `0`
		}
		tab := `id="tab-` + view + `" aria-controls="view-` + view + `" tabindex="` + tabIndex + `"`
		if !strings.Contains(html, tab) {
			t.Errorf("%s tab missing %q", view, tab)
		}
		panel := `id="view-` + view + `" class="console-view" role="tabpanel" aria-labelledby="tab-` + view + `"`
		if !strings.Contains(html, panel) {
			t.Errorf("%s panel missing %q", view, panel)
		}
	}
	js := string(uiJS)
	for _, want := range []string{"applyTabView", "wireTabNavigation(tabs, setView)"} {
		if !strings.Contains(js, want) {
			t.Errorf("tab keyboard navigation missing %q", want)
		}
	}
	behavior := string(behaviorJS)
	for _, want := range []string{`"ArrowLeft"`, `"ArrowRight"`, `"Home"`, `"End"`, "tab.tabIndex", "tabs[next].focus()", "activate(tabs[next].dataset.view)"} {
		if !strings.Contains(behavior, want) {
			t.Errorf("shared tab keyboard navigation missing %q", want)
		}
	}
}

func TestUIReplayLivesInGameBay(t *testing.T) {
	html := string(indexHTML)
	inspector := string(inspectorJS)
	if !strings.Contains(html, `id="detail-game-media"`) {
		t.Fatal("Game bay must expose one live/replay media host")
	}
	if !strings.Contains(inspector, `detail-game-media`) {
		t.Error("inspector must mount replay into the Game bay")
	}
	if strings.Contains(inspector, `id="pp-video"`) {
		t.Error("inspector must not create a second detached replay video")
	}
	if !strings.Contains(inspector, `id="pp-game-video"`) {
		t.Error("inspector must mount the replay video in the stable Game media host")
	}
	for _, stale := range []string{`id="pp-scrubber"`, `type="range"`, `replay-scrubber`} {
		if strings.Contains(inspector, stale) {
			t.Errorf("inspector must not create detached replay transport %q", stale)
		}
	}
}

func TestUIStatsBelongToAnalytics(t *testing.T) {
	js := string(statsJS)
	if !strings.Contains(js, `analytics-outcomes`) {
		t.Error("stats.js must render campaign statistics in Analytics")
	}
	if strings.Contains(js, `run-outcomes`) {
		t.Error("stats.js must not render campaign statistics in Operations")
	}
}

func TestUIOperationsAreOwnedByDashboardRender(t *testing.T) {
	js := string(uiJS)
	start := strings.Index(js, "function renderOperations")
	if start < 0 {
		t.Fatal("ui.js must render Operations from its dashboard snapshot")
	}
	end := strings.Index(js[start:], "\n  function ")
	if end < 0 {
		t.Fatal("renderOperations bounds")
	}
	operations := js[start : start+end]
	for _, want := range []string{
		`operations-health`,
		`operations-workers`,
		`operations-active`,
		`operations-queue`,
		`operations-recent`,
		`snap.runs`,
		`snap.workers`,
		`status === "leased"`,
		`partitionOperations`,
		`operationRuns.active`,
		`operationRuns.waiting`,
		`operationRuns.recent`,
		`data-run=`,
		`RECENT_OPERATIONS_LIMIT`,
		`ended_at`,
		`queued_at`,
		`run.goal || run.dest`,
		`run.reason`,
		`run.detail`,
	} {
		if !strings.Contains(operations, want) {
			t.Errorf("renderOperations missing %q", want)
		}
	}
	render := strings.Index(js, "function render()")
	refresh := strings.Index(js, "async function refresh()")
	if render < 0 || refresh <= render || !strings.Contains(js[render:refresh], "renderOperations()") {
		t.Error("main dashboard render pass must call renderOperations")
	}
}

func TestUIOperationsUseTruthfulRowsAndDeepLinks(t *testing.T) {
	js := string(uiJS)
	start := strings.Index(js, "function renderOperations")
	if start < 0 {
		t.Fatal("renderOperations start")
	}
	end := strings.Index(js[start:], "\n  function ")
	if end < 0 {
		t.Fatal("renderOperations bounds")
	}
	operations := js[start : start+end]
	for _, want := range []string{
		`wallDown`,
		`lastFreshAt`,
		`consoleVersion`,
		`snap.wall_version`,
		`worker.run_id`,
		`worker.seen_ago`,
		`class="operation-row`,
	} {
		if !strings.Contains(operations, want) {
			t.Errorf("operations truth/deep-link contract missing %q", want)
		}
	}
	for _, forbidden := range []string{"Badge distribution", "Objective wins", "Endless experiments", "service healthy"} {
		if strings.Contains(operations, forbidden) {
			t.Errorf("Operations contains campaign or invented health content %q", forbidden)
		}
	}
}

func TestUIChipClassesAreAllowlisted(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{
		`const CHIP_KINDS = new Set`,
		`function safeChipKind`,
		`CHIP_KINDS.has(kind)`,
		`chip${safeChipKind(kind)}`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("safe chip class contract missing %q", want)
		}
	}
	for _, unsafe := range []string{`class="chip ${kind}`, `class="chip ${esc(kind)}`} {
		if strings.Contains(js, unsafe) {
			t.Errorf("chip class still accepts arbitrary token through %q", unsafe)
		}
	}
	// The payload documents the stored-XSS shape this contract must reject:
	// reason = `x" onmouseover="alert(1)` may be text, never class markup.
	malicious := `x" onmouseover="alert(1)`
	if strings.Contains(js, `CHIP_KINDS.add(`+malicious) {
		t.Fatal("malicious reason unexpectedly allowlisted")
	}
}

func TestUIOperationsPaintOnceAndPreserveUnchangedRows(t *testing.T) {
	js := string(uiJS)
	start := strings.Index(js, "function renderOperations")
	if start < 0 {
		t.Fatal("renderOperations start")
	}
	end := strings.Index(js[start:], "\n  function ")
	if end < 0 {
		t.Fatal("renderOperations bounds")
	}
	operations := js[start : start+end]
	for _, target := range []string{`health`, `$(` + `"workers"` + `)`, `active`, `queue`, `recent`} {
		if !strings.Contains(operations, `paintHTML(`+target) {
			t.Errorf("renderOperations must paint-guard %s", target)
		}
	}
	if strings.Contains(operations, ".innerHTML =") {
		t.Error("renderOperations bypasses paintHTML and replaces focused rows")
	}
	if strings.Contains(js, "function renderWorkers") {
		t.Error("worker region has a second renderer outside renderOperations")
	}
	if strings.Count(js, `renderOperations();`) != 2 {
		t.Error("Operations should render only for active dashboard refreshes and newly-opened tab")
	}
}

func TestUIPaintHTMLRestoresFocusedRunAfterChangedSnapshot(t *testing.T) {
	js := string(uiJS)
	start := strings.Index(js, "function paintHTML")
	end := strings.Index(js[start:], "\n  function ")
	if start < 0 || end < 0 {
		t.Fatal("paintHTML bounds")
	}
	paint := js[start : start+end]
	for _, want := range []string{`document.activeElement`, `focusRun`, `CSS.escape(focusRun)`, `.focus()`} {
		if !strings.Contains(paint, want) {
			t.Errorf("paintHTML focus restoration missing %q", want)
		}
	}
}

func TestUIOperationsCollapseBeforeRailCanClipThem(t *testing.T) {
	css := string(consoleCSS)
	if !strings.Contains(css, `@media(max-width:1100px){.operations-grid{grid-template-columns:1fr}`) {
		t.Error("Operations must collapse while the persistent desktop rail still constrains content")
	}
	for _, want := range []string{
		`.operation-row{grid-template-columns:minmax(0,.8fr) minmax(0,1.2fr)`,
		`overflow-wrap:anywhere`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("responsive Operations styles missing %q", want)
		}
	}
}

func TestUIStatsOwnOnlyAggregateAnalytics(t *testing.T) {
	js := string(statsJS)
	if strings.Contains(js, `createElement("style")`) {
		t.Error("stats.js must not inject styles; console.css owns presentation")
	}
	if strings.Contains(js, `/v1/dashboard`) {
		t.Error("stats.js must not poll the dashboard; ui.js owns the snapshot")
	}
	if strings.Contains(js, `live-goal-progress`) {
		t.Error("stats.js must not render selected-run goal progress")
	}
	for _, want := range []string{
		"Completed attempts",
		"Objective wins",
		"Badge distribution",
		"Terminal outcomes",
		"Retry failures",
		"No progress data",
		"Endless experiments",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Analytics lost aggregate campaign statistic %q", want)
		}
	}
	css := string(consoleCSS)
	for _, want := range []string{`.outcome-stats`, `.outcome-summary`, `.outcome-grid`, `.endless-table`} {
		if !strings.Contains(css, want) {
			t.Errorf("console.css missing Analytics style %q", want)
		}
	}
}

func TestUIStatsPauseWhileAnalyticsIsHidden(t *testing.T) {
	stats := string(statsJS)
	for _, want := range []string{
		`function analyticsVisible`,
		`if(!analyticsVisible())return`,
		`pokefarm-console-view`,
		`detail.view==="analytics"`,
	} {
		if !strings.Contains(stats, want) {
			t.Errorf("hidden Analytics polling contract missing %q", want)
		}
	}
	if !strings.Contains(string(uiJS), `pokefarm-console-view`) {
		t.Error("view owner must notify Analytics when its tab opens")
	}
}

func TestUISelectedGoalProgressUsesExistingSnapshot(t *testing.T) {
	ui := string(uiJS)
	for _, want := range []string{`goalProgressHTML`, `goal_summary`, `goal_current`, `goal_target`, `goal_complete`, `detail-goal`} {
		if !strings.Contains(ui, want) {
			t.Errorf("ui.js selected-run goal progress missing %q", want)
		}
	}
	if strings.Count(ui, `fetch("/v1/dashboard"`) != 1 {
		t.Error("ui.js must keep one dashboard polling owner")
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
	for _, want := range []string{`id="pp-game-video"`, `video.currentTime`, `video.duration`, `nearestEventIndex`, `Semantic state from nearest persisted event`} {
		if !strings.Contains(js, want) {
			t.Errorf("replay transport missing %q", want)
		}
	}
	if strings.Contains(js, `type="range"`) {
		t.Error("finished replay must use native video controls without a duplicate range input")
	}
}

func TestUIFollowsWatchingFirstReference(t *testing.T) {
	html := string(indexHTML)
	js := string(uiJS)
	inspector := string(inspectorJS)
	for _, want := range []string{`class="brand-mark"`, `aria-hidden="true"`, `<strong>PokéFarm</strong>`, `<span>Operator console</span>`} {
		if !strings.Contains(html, want) {
			t.Errorf("header must restore the established PokéFarm mark and wordmark, missing %q", want)
		}
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
	for _, want := range []string{`"missing"`, `"generating"`, `"ready"`, `"error"`, `"disabled"`, `renderReplay({state:"missing"`, `replayButton.hidden=false`, `replayButton.disabled=true`, `Generate replay`, `Retry replay`, `Available after this run finishes`, `run.gbrun`} {
		if !strings.Contains(js, want) {
			t.Errorf("replay state contracts missing %q", want)
		}
	}
	for _, want := range []string{`detail-lcd`, `removeAttribute("src")`, `stopReplayPoll()`, `id!==runID`} {
		if !strings.Contains(js, want) {
			t.Errorf("replay cleanup contract missing %q", want)
		}
	}
}

func TestUISemanticReplayContextIsTruthful(t *testing.T) {
	inspector := string(inspectorJS)
	ui := string(uiJS)
	for _, want := range []string{`pokefarm-semantic-event`, `nearest persisted event`, `followingLive`} {
		if !strings.Contains(inspector, want) {
			t.Errorf("inspector replay semantics missing %q", want)
		}
	}
	for _, want := range []string{`pokefarm-semantic-event`, `Semantic state from nearest persisted event`, `renderMap`} {
		if !strings.Contains(ui, want) {
			t.Errorf("dashboard semantic context missing %q", want)
		}
	}
	for _, want := range []string{`No map snapshot for this checkpoint`, `showing last known state`} {
		if !strings.Contains(ui, want) {
			t.Errorf("dashboard map fallback missing %q", want)
		}
	}
	if strings.Contains(ui, `renderMap(context && !hasEventPosition ? null : run`) {
		t.Error("selecting a locationless checkpoint must not hide the semantic map")
	}
}

func TestUITimelineCommunicatesScaleAndCheckpointRestartability(t *testing.T) {
	inspector := string(inspectorJS)
	for _, want := range []string{
		`id="pp-timeline-action"`,
		`id="pp-timeline-selection"`,
		`timelineLayout(events, timelineFrameTotal())`,
		`Start a new run from here`,
		`Evidence only`,
		`Restartable checkpoint`,
		`--timeline-lanes`,
	} {
		if !strings.Contains(inspector, want) {
			t.Errorf("timeline clarity contract missing %q", want)
		}
	}
}

func TestUIReloadsInspectorWhenSelectedRunFinishes(t *testing.T) {
	ui := string(uiJS)
	inspector := string(inspectorJS)
	for _, want := range []string{`pokefarm-run-lifecycle`, `runId: run.run_id`, `status: run.status`, `frame: run.frame`, `replayAvailable: Boolean(run.replay_available)`, `completionSignature: completionSignature(run)`} {
		if !strings.Contains(ui, want) {
			t.Errorf("dashboard lifecycle publication missing %q", want)
		}
	}
	for _, want := range []string{`pokefarm-run-lifecycle`, `previousStatus`, `signatureChanged`, `signature!==completionSignature`, `runLoadInFlight`, `finishedReloadPending`} {
		if !strings.Contains(inspector, want) {
			t.Errorf("inspector lifecycle reload missing %q", want)
		}
	}
	if !strings.Contains(inspector, `if(changed){stopFinalizeRetry();lifecycleStatus="";completionSignature="";finishedReloadPending=false;dashboardReplayAvailable=false;runTotalFrame=0}`) {
		t.Error("switching runs must discard a pending completion reload owned by the previous run")
	}
}

func TestUICompletionSignatureTracksReportEnrichment(t *testing.T) {
	ui := string(uiJS)
	start := strings.Index(ui, "function completionSignature")
	if start < 0 {
		t.Fatal("ui.js missing completionSignature")
	}
	end := strings.Index(ui[start:], "\n  }")
	if end < 0 {
		t.Fatal("completionSignature bounds")
	}
	signature := ui[start : start+end]
	for _, want := range []string{`run.status`, `run.ended_at`, `run.reason`, `run.detail`, `run.replay_available`, `run.attempts`} {
		if !strings.Contains(signature, want) {
			t.Errorf("completion signature missing report-backed fact %q", want)
		}
	}
	for _, unrelated := range []string{`run.issue`, `issue.issue_id`, `issue.status`, `issue.occurrence_count`, `issue.resolution`, `issue.fixed_revision`} {
		if strings.Contains(signature, unrelated) {
			t.Errorf("completion signature contains unrelated triage fact %q", unrelated)
		}
	}
	if strings.Contains(signature, `run.frame`) {
		t.Error("completion signature must not change with ordinary frame/dashboard refreshes")
	}

	inspector := string(inspectorJS)
	for _, want := range []string{`if(status!=="done")`, `completionSignature=signature`, `previousStatus===""`, `selectRun(runID,true)`} {
		if !strings.Contains(inspector, want) {
			t.Errorf("inspector completion enrichment handling missing %q", want)
		}
	}
}

func TestUIFinalizedReplayEvidenceSettlesWithBoundedRetry(t *testing.T) {
	inspector := string(inspectorJS)
	for _, want := range []string{
		`const FINALIZE_RETRY_LIMIT=5`,
		`const FINALIZE_RETRY_MS=750`,
		`dashboardReplayAvailable`,
		`finishEvidence`,
		`!replayable||!finishEvidence`,
		`scheduleFinalizeRetry(id)`,
		`finalizeRetryCount>=FINALIZE_RETRY_LIMIT`,
		`stopFinalizeRetry()`,
		`clearTimeout(finalizeRetryTimer)`,
		`if(!dashboardReplayAvailable)return`,
	} {
		if !strings.Contains(inspector, want) {
			t.Errorf("bounded finalized replay settle missing %q", want)
		}
	}
	if !strings.Contains(inspector, `if(changed)resetGameMedia();else stopReplayPoll()`) {
		t.Error("same-run evidence refresh must preserve already-ready replay media")
	}
}

func TestUIReplayUsesRunTotalFrameForVideoMapping(t *testing.T) {
	inspector := string(inspectorJS)
	for _, want := range []string{`runTotalFrame`, `timelineFrameTotal()`, `debugView.run`, `detail.frame`} {
		if !strings.Contains(inspector, want) {
			t.Errorf("replay total-frame mapping missing %q", want)
		}
	}
}

func TestUISemanticContextSurvivesMapResize(t *testing.T) {
	ui := string(uiJS)
	start := strings.Index(ui, "function watchMapSize")
	if start < 0 {
		t.Fatal("map resize handler start")
	}
	end := strings.Index(ui[start:], `$("detail-map").addEventListener`)
	if end < 0 {
		t.Fatal("map resize handler bounds")
	}
	resize := ui[start : start+end]
	if !strings.Contains(resize, "renderDetail()") {
		t.Error("map resize must repaint through renderDetail so semanticContext is preserved")
	}
	if strings.Contains(resize, "renderMap(run)") {
		t.Error("map resize must not bypass active semanticContext")
	}
}

func TestUIInvestigateCapturesSelectedRun(t *testing.T) {
	inspector := string(inspectorJS)
	for _, want := range []string{`const targetRunID=runID`, `includes(targetRunID)`, `targetRunID!==runID`} {
		if !strings.Contains(inspector, want) {
			t.Errorf("investigation stale-response guard missing %q", want)
		}
	}
	start := strings.Index(inspector, "async function investigateRun")
	if start < 0 {
		t.Fatal("investigateRun start")
	}
	end := strings.Index(inspector[start:], "root.addEventListener")
	if end < 0 {
		t.Fatal("investigateRun bounds")
	}
	if strings.Contains(inspector[start:start+end], `includes(runID)`) {
		t.Error("investigateRun must not read mutable runID after awaiting triage")
	}
}

func TestUITimelineMarkersHaveAccessibleHitTargets(t *testing.T) {
	css := string(consoleCSS)
	rule := regexp.MustCompile(`\.timeline-track \.timeline-marker\{[^}]+\}`).FindString(css)
	for _, want := range []string{"width:24px", "height:24px"} {
		if !strings.Contains(rule, want) {
			t.Errorf("timeline marker hit-target rule %q missing %q", rule, want)
		}
	}
	if !strings.Contains(css, `.timeline-track .timeline-marker:before`) {
		t.Error("timeline marker must use a pseudo-element for the thin visible tick")
	}
}

func TestUIRevokesFrameObjectURLs(t *testing.T) {
	ui := string(uiJS)
	for _, want := range []string{`lastFrameURLs`, `revokeLastFrameURL`, `URL.revokeObjectURL(blobUrl)`, `cleanupFrameURLs`, `beforeunload`, `if (stop) { URL.revokeObjectURL(url); break; }`, `displayed.has(id)`} {
		if !strings.Contains(ui, want) {
			t.Errorf("frame object URL cleanup missing %q", want)
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

func TestUIVisualStageCollapsesBeforePersistentRailClipsIt(t *testing.T) {
	css := string(consoleCSS)
	start := strings.Index(css, `@media(max-width:1140px) and (min-width:761px)`)
	end := strings.Index(css, `@media(max-width:760px)`)
	if start < 0 || end <= start {
		t.Fatal("console must collapse the three-bay stage before the persistent rail clips it")
	}
	responsive := css[start:end]
	for _, want := range []string{
		`.visual-stage{height:auto;grid-template-columns:minmax(0,1fr) minmax(0,1.12fr)}`,
		`.game-state-monitor{grid-column:1/-1}`,
	} {
		if !strings.Contains(responsive, want) {
			t.Errorf("wide-tablet stage contract missing %q", want)
		}
	}
}

func TestUINarrowHeaderKeepsPrimaryActionVisible(t *testing.T) {
	css := string(consoleCSS)
	start := strings.Index(css, `@media(max-width:760px)`)
	if start < 0 {
		t.Fatal("console missing narrow layout")
	}
	narrow := css[start:]
	for _, want := range []string{
		`.system-summary{display:none}`,
		`.console-tabs{width:100%;min-width:0`,
	} {
		if !strings.Contains(narrow, want) {
			t.Errorf("narrow header contract missing %q", want)
		}
	}
	if !regexp.MustCompile(`\.console-bar\{[^}]*grid-template-columns:minmax\(0,1fr\) auto`).MatchString(narrow) {
		t.Error("narrow header must reserve a shrink-safe brand track beside the primary action")
	}
}

func TestUIFirstViewportIncludesRunStory(t *testing.T) {
	css := string(consoleCSS)
	stage := regexp.MustCompile(`\.visual-stage\{[^}]*height:clamp\((\d+)px,(\d+)vh,(\d+)px\)`).FindStringSubmatch(css)
	if stage == nil {
		t.Fatal("visual stage must use a bounded viewport-aware height")
	}
	vh, _ := strconv.Atoi(stage[2])
	max, _ := strconv.Atoi(stage[3])
	if vh > 42 || max > 380 {
		t.Fatalf("visual stage clamp %q leaves no room for timeline and Run Story at 1440x900", stage[0])
	}
	deck := regexp.MustCompile(`\.state-deck\{[^}]+\}`).FindString(css)
	if strings.Contains(deck, "overflow:auto") || strings.Contains(deck, "max-height:") {
		t.Errorf("secondary state deck %q must not create a shallow outer scroll trap", deck)
	}
	body := regexp.MustCompile(`#detail-body\{[^}]+\}`).FindString(css)
	if !strings.Contains(body, "height:260px") {
		t.Errorf("secondary detail band %q must reserve enough height for useful values", body)
	}
	html := string(indexHTML)
	header := regexp.MustCompile(`(?s)<header class="run-header".*?</header>`).FindString(html)
	if !strings.Contains(header, `id="detail-goal"`) {
		t.Error("goal progress must live in the state strip instead of consuming a detail-deck row")
	}
}

func TestUIStackedGameMediaHasARatioAwareHeightCap(t *testing.T) {
	css := string(consoleCSS)
	start := strings.Index(css, `@media(max-width:760px)`)
	if start < 0 {
		t.Fatal("console missing stacked narrow layout")
	}
	narrow := css[start:]
	game := regexp.MustCompile(`\.game-monitor\{[^}]+\}`).FindString(narrow)
	for _, want := range []string{"height:min(", "52vh", "calc(90vw + 21px)", "max-height:430px"} {
		if !strings.Contains(game, want) {
			t.Errorf("stacked Game bay %q missing %q", game, want)
		}
	}
}

func TestUIDetailGridHeightFollowsResponsiveRows(t *testing.T) {
	css := string(consoleCSS)
	tabletStart := strings.Index(css, `@media(max-width:1140px) and (min-width:761px)`)
	narrowStart := strings.Index(css, `@media(max-width:760px)`)
	if tabletStart < 0 || narrowStart <= tabletStart {
		t.Fatal("responsive detail-grid breakpoints missing")
	}
	tablet := css[tabletStart:narrowStart]
	for _, want := range []string{
		`#detail-body{height:auto;grid-template-columns:repeat(2,minmax(0,1fr));grid-auto-rows:260px}`,
	} {
		if !strings.Contains(tablet, want) {
			t.Errorf("tablet detail grid missing %q", want)
		}
	}
	narrow := css[narrowStart:]
	for _, want := range []string{
		`#detail-body{height:auto;grid-template-columns:1fr;grid-auto-rows:auto}`,
		`.compact{height:auto;min-height:112px;max-height:180px;overflow:auto}`,
		`.block.scroll{min-height:140px;max-height:220px;overflow:auto}`,
		`.screen-event-card{align-items:flex-start;flex-wrap:wrap}`,
	} {
		if !strings.Contains(narrow, want) {
			t.Errorf("narrow detail flow missing %q", want)
		}
	}
}

func TestUISemanticMapUsesStableDarkModeTokens(t *testing.T) {
	css := string(consoleCSS)
	ui := string(uiJS)
	seen := map[string]string{}
	for _, token := range []string{"--map-ground", "--map-wall", "--map-grass", "--map-water", "--map-trail", "--map-player", "--map-sprite", "--map-warp"} {
		match := regexp.MustCompile(regexp.QuoteMeta(token) + `:(#[0-9a-fA-F]{6})`).FindStringSubmatch(css)
		if match == nil {
			t.Errorf("console.css missing concrete semantic map token %s", token)
			continue
		}
		if previous := seen[match[1]]; previous != "" {
			t.Errorf("semantic map tokens %s and %s share %s; map roles must remain distinguishable", previous, token, match[1])
		}
		seen[match[1]] = token
		if !strings.Contains(ui, `mapColor("`+token+`"`) {
			t.Errorf("map renderer does not consume %s", token)
		}
	}
}

func TestUIPartyAlwaysRendersSixAccessibleSlots(t *testing.T) {
	ui := string(uiJS)
	for _, want := range []string{
		`Array.from({length:6}`,
		`partyMembers[index]`,
		`Empty slot`,
		`aria-label="Party slot ${index + 1}: Empty slot"`,
	} {
		if !strings.Contains(ui, want) {
			t.Errorf("six-slot party contract missing %q", want)
		}
	}
}

func TestUIPartyTelemetryAbsenceStillRendersSlots(t *testing.T) {
	ui := string(uiJS)
	start := strings.Index(ui, "function partyHTML")
	end := strings.Index(ui[start:], "\n  function ")
	if start < 0 || end < 0 {
		t.Fatal("partyHTML bounds")
	}
	party := ui[start : start+end]
	if strings.Contains(party, `if (!p) return ""`) {
		t.Error("missing player telemetry must not blank the stable Game State bay")
	}
	for _, want := range []string{`Player telemetry unavailable`, `Array.from({length:6}`, `Empty slot`} {
		if !strings.Contains(party, want) {
			t.Errorf("missing-player party state missing %q", want)
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
	if !strings.Contains(css, `#detail-plan .plan-q{max-height:none;overflow:visible}`) {
		t.Error("Plan question must use the panel's overflow instead of creating a nested scrollbar")
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
	if strings.Contains(js, `block scroll screen-event-card`) {
		t.Error("last event must not reuse the four-column scroll block height")
	}
	eventCard := regexp.MustCompile(`\.screen-event-card\{[^}]+\}`).FindString(css)
	if !strings.Contains(eventCard, "display:flex") {
		t.Errorf("last event strip %q must sit on one compact row", eventCard)
	}
	if !strings.Contains(string(indexHTML), `id="detail-party"`) {
		t.Error("party must sit outside #detail-body so it does not take a 1fr track")
	}
}

func TestUIUsesSoftOperatorPalette(t *testing.T) {
	css := string(consoleCSS)
	for _, want := range []string{
		`--bg:#11141c`,
		`--shell:#0c0e14`,
		`--bay:#1a2030`,
		`--bay-2:#222a3c`,
		`--text:#e8ebd9`,
		`--muted:#a8b3c6`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("soft operator palette missing %q", want)
		}
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
	end := strings.Index(js, "function filterBtn")
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
