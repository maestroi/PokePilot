package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

const archiveRuntimeStateFile = "archive-runtime.json"

type archiveRuntimeState struct {
	StartedAt map[string]int64 `json:"started_at,omitempty"`
}

type archiveController struct {
	wall     *Wall
	fallback http.Handler
	mu       sync.Mutex
	state    archiveRuntimeState
	path     string
}

// archiveHTTPHandler enriches the finished-run dashboard after the model
// wrapper has attached immutable inference/experiment identity. Keeping this
// outside the compatibility wall lets the archive evolve without changing the
// runner-facing wire or making legacy runs unreadable.
func archiveHTTPHandler(w *Wall, fallback http.Handler) http.Handler {
	controller := &archiveController{
		wall:     w,
		fallback: fallback,
		state:    archiveRuntimeState{StartedAt: map[string]int64{}},
	}
	if w.dumpsDir != "" {
		controller.path = filepath.Join(w.dumpsDir, archiveRuntimeStateFile)
		controller.loadState()
	}
	return controller
}

func (c *archiveController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/dashboard" && strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("status")), statusDone):
		c.handleDashboard(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/lease":
		c.handleLease(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/runs/"):
		c.handleDelete(w, r)
	default:
		c.fallback.ServeHTTP(w, r)
	}
}

func (c *archiveController) handleLease(w http.ResponseWriter, r *http.Request) {
	recorder := httptest.NewRecorder()
	c.fallback.ServeHTTP(recorder, r)
	if recorder.Code >= 200 && recorder.Code < 300 && recorder.Code != http.StatusNoContent {
		var spec farm.Spec
		if json.Unmarshal(recorder.Body.Bytes(), &spec) == nil && strings.TrimSpace(spec.RunID) != "" {
			c.rememberStarted(spec.RunID, spec.Attempt, time.Now().Unix())
		}
	}
	copyRecorder(w, recorder)
}

func (c *archiveController) handleDelete(w http.ResponseWriter, r *http.Request) {
	recorder := httptest.NewRecorder()
	c.fallback.ServeHTTP(recorder, r)
	if recorder.Code >= 200 && recorder.Code < 300 {
		runID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/runs/"))
		if runID != "" && !strings.Contains(runID, "/") {
			c.forgetStarted(runID)
		}
	}
	copyRecorder(w, recorder)
}

func (c *archiveController) handleDashboard(w http.ResponseWriter, r *http.Request) {
	limit, offset, errMessage := archivePage(r)
	if errMessage != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMessage})
		return
	}

	// Existing outcome/mode/starter filters still run in the compatibility
	// dashboard. Remove only pagination so every additional filter and sort is
	// applied to the complete matching history before slicing the requested page.
	clone := r.Clone(r.Context())
	clonedURL := *r.URL
	query := clonedURL.Query()
	query.Del("limit")
	query.Del("offset")
	clonedURL.RawQuery = query.Encode()
	clone.URL = &clonedURL

	recorder := httptest.NewRecorder()
	c.fallback.ServeHTTP(recorder, clone)
	if recorder.Code < 200 || recorder.Code >= 300 {
		copyRecorder(w, recorder)
		return
	}

	var document map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &document); err != nil {
		copyRecorder(w, recorder)
		return
	}

	rawRuns, _ := document["runs"].([]any)
	allRows := make([]map[string]any, 0, len(rawRuns))
	for _, raw := range rawRuns {
		run, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		c.enrichRuntime(run)
		allRows = append(allRows, run)
	}

	facets := archiveFacets(allRows)
	mergeArchiveFacets(document, facets)

	filtered := make([]map[string]any, 0, len(allRows))
	for _, run := range allRows {
		if archiveRunMatches(run, r.URL.Query()) {
			filtered = append(filtered, run)
		}
	}

	sortKey := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	if sortKey == "" {
		sortKey = "finished"
	}
	if !archiveSortSupported(sortKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported archive sort: " + sortKey})
		return
	}
	direction := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("direction")))
	if direction == "" {
		if sortKey == "finished" || sortKey == "progress" {
			direction = "desc"
		} else {
			direction = "asc"
		}
	}
	if direction != "asc" && direction != "desc" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "direction must be asc or desc"})
		return
	}
	archiveSortRows(filtered, sortKey, direction)

	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	paged := filtered[offset:end]

	runs := make([]any, 0, len(paged))
	for _, run := range paged {
		runs = append(runs, run)
	}
	document["runs"] = runs
	document["total"] = total
	writeJSON(w, http.StatusOK, document)
}

func archivePage(r *http.Request) (limit, offset int, errMessage string) {
	q := r.URL.Query()
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return 0, 0, "limit must be a positive integer"
		}
		limit = value
	}
	if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return 0, 0, "offset must be a non-negative integer"
		}
		offset = value
	}
	return limit, offset, ""
}

func (c *archiveController) enrichRuntime(run map[string]any) {
	runID := archiveString(run["run_id"])
	if runID == "" {
		return
	}
	startedAt := c.startedAt(runID)
	if startedAt <= 0 {
		return
	}
	run["started_at"] = startedAt
	endedAt, ok := archiveNumber(run["ended_at"])
	if ok && endedAt >= float64(startedAt) {
		run["runtime_seconds"] = endedAt - float64(startedAt)
	}
}

func archiveRunMatches(run map[string]any, query mapQuery) bool {
	if value := strings.TrimSpace(query.Get("model")); value != "" && !archiveEqual(value, archiveModelID(run)) {
		return false
	}
	if value := strings.TrimSpace(query.Get("deployment")); value != "" && !archiveEqual(value, archiveDeploymentID(run)) {
		return false
	}
	if value := strings.TrimSpace(query.Get("goal")); value != "" && !archiveEqual(value, archiveString(run["goal"])) {
		return false
	}
	if value := strings.TrimSpace(query.Get("play_style")); value != "" && !archiveEqual(value, archiveString(run["play_style"])) {
		return false
	}
	if value := strings.TrimSpace(query.Get("compute")); value != "" && !archiveEqual(value, archiveCompute(run)) {
		return false
	}
	if value := strings.TrimSpace(query.Get("experiment")); value != "" && !archiveEqual(value, archiveString(run["experiment_id"])) {
		return false
	}
	if queryBool(query.Get("success")) && !archiveSuccess(run) {
		return false
	}
	return true
}

// mapQuery is the small subset of url.Values used by matching. Keeping the
// interface local makes the predicate straightforward to test.
type mapQuery interface {
	Get(string) string
}

func archiveEqual(want, got string) bool {
	return strings.EqualFold(strings.TrimSpace(want), strings.TrimSpace(got))
}

func archiveSuccess(run map[string]any) bool {
	if strings.EqualFold(strings.TrimSpace(archiveString(run["reason"])), "done") {
		return true
	}
	if stats := archiveMap(run["stats"]); stats != nil {
		if complete, ok := stats["goal_complete"].(bool); ok && complete {
			return true
		}
	}
	return false
}

func archiveSortSupported(key string) bool {
	switch key {
	case "finished", "frames", "rounds", "planner_time", "runtime", "progress", "strategic_calls":
		return true
	default:
		return false
	}
}

func archiveSortRows(rows []map[string]any, key, direction string) {
	sort.SliceStable(rows, func(i, j int) bool {
		left, leftOK := archiveSortMetric(rows[i], key)
		right, rightOK := archiveSortMetric(rows[j], key)
		if leftOK != rightOK {
			return leftOK // missing telemetry always sorts last
		}
		if leftOK && left != right {
			if direction == "desc" {
				return left > right
			}
			return left < right
		}
		// Stable deterministic tie-breaker: newest terminal run, then run id.
		leftEnded, _ := archiveNumber(rows[i]["ended_at"])
		rightEnded, _ := archiveNumber(rows[j]["ended_at"])
		if leftEnded != rightEnded {
			return leftEnded > rightEnded
		}
		return archiveString(rows[i]["run_id"]) < archiveString(rows[j]["run_id"])
	})
}

func archiveSortMetric(run map[string]any, key string) (float64, bool) {
	switch key {
	case "finished":
		return archiveNumber(run["ended_at"])
	case "frames":
		return archiveNumber(run["frame"])
	case "runtime":
		return archiveNumber(run["runtime_seconds"])
	case "progress":
		return archiveProgress(run)
	}
	stats := archiveMap(run["stats"])
	if stats == nil {
		return 0, false
	}
	switch key {
	case "rounds":
		if value, ok := archiveNumber(stats["rounds"]); ok {
			return value, true
		}
		return archiveNumber(stats["round"])
	case "strategic_calls":
		return archiveNumber(stats["strategic_calls"])
	case "planner_time":
		if value, ok := archiveNumber(stats["strategic_seconds"]); ok && value > 0 {
			return value, true
		}
		avg, avgOK := archiveNumber(stats["avg_seconds"])
		calls, callsOK := archiveNumber(stats["calls"])
		if avgOK && callsOK && calls > 0 {
			return avg * calls, true
		}
	}
	return 0, false
}

func archiveProgress(run map[string]any) (float64, bool) {
	badges := 0
	if player := archiveMap(run["player"]); player != nil {
		if values, ok := player["badges"].([]any); ok {
			badges = len(values)
		}
	}
	if stats := archiveMap(run["stats"]); stats != nil {
		if current, ok := archiveNumber(stats["goal_current"]); ok {
			// Badges dominate generic goal counters without pretending unrelated
			// goals are directly comparable.
			return float64(badges)*1000000 + current, true
		}
	}
	if badges > 0 {
		return float64(badges) * 1000000, true
	}
	return 0, false
}

type archiveFacetView struct {
	Models      []string
	Deployments []string
	Goals       []string
	PlayStyles  []string
	Computes    []string
	Experiments []string
}

func archiveFacets(rows []map[string]any) archiveFacetView {
	models := map[string]struct{}{}
	deployments := map[string]struct{}{}
	goals := map[string]struct{}{}
	playStyles := map[string]struct{}{}
	computes := map[string]struct{}{}
	experiments := map[string]struct{}{}
	for _, run := range rows {
		archiveAddFacet(models, archiveModelID(run))
		archiveAddFacet(deployments, archiveDeploymentID(run))
		archiveAddFacet(goals, archiveString(run["goal"]))
		archiveAddFacet(playStyles, archiveString(run["play_style"]))
		archiveAddFacet(computes, archiveCompute(run))
		archiveAddFacet(experiments, archiveString(run["experiment_id"]))
	}
	return archiveFacetView{
		Models:      archiveSortedFacet(models),
		Deployments: archiveSortedFacet(deployments),
		Goals:       archiveSortedFacet(goals),
		PlayStyles:  archiveSortedFacet(playStyles),
		Computes:    archiveSortedFacet(computes),
		Experiments: archiveSortedFacet(experiments),
	}
}

func mergeArchiveFacets(document map[string]any, view archiveFacetView) {
	facets := archiveMap(document["history_facets"])
	if facets == nil {
		facets = map[string]any{}
		document["history_facets"] = facets
	}
	facets["models"] = view.Models
	facets["deployments"] = view.Deployments
	facets["goals"] = view.Goals
	facets["play_styles"] = view.PlayStyles
	facets["computes"] = view.Computes
	facets["experiments"] = view.Experiments
}

func archiveModelID(run map[string]any) string {
	if inference := archiveMap(run["inference"]); inference != nil {
		if value := archiveString(inference["model_id"]); value != "" {
			return value
		}
	}
	if stats := archiveMap(run["stats"]); stats != nil {
		if value := archiveString(stats["model"]); value != "" {
			return value
		}
	}
	return archiveString(run["llm_profile"])
}

func archiveDeploymentID(run map[string]any) string {
	if value := archiveString(run["llm_deployment"]); value != "" {
		return value
	}
	if inference := archiveMap(run["inference"]); inference != nil {
		return archiveString(inference["deployment_id"])
	}
	return ""
}

func archiveCompute(run map[string]any) string {
	if inference := archiveMap(run["inference"]); inference != nil {
		return archiveString(inference["compute"])
	}
	return ""
}

func archiveAddFacet(values map[string]struct{}, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		values[value] = struct{}{}
	}
}

func archiveSortedFacet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func archiveMap(value any) map[string]any {
	out, _ := value.(map[string]any)
	return out
}

func archiveString(value any) string {
	text, _ := value.(string)
	return text
}

func archiveNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint64:
		return float64(number), true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func (c *archiveController) rememberStarted(runID string, attempt int, startedAt int64) {
	runID = strings.TrimSpace(runID)
	if runID == "" || startedAt <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.StartedAt == nil {
		c.state.StartedAt = map[string]int64{}
	}
	if attempt <= 1 || c.state.StartedAt[runID] == 0 {
		c.state.StartedAt[runID] = startedAt
		c.persistLocked()
	}
}

func (c *archiveController) forgetStarted(runID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.state.StartedAt[runID]; !ok {
		return
	}
	delete(c.state.StartedAt, runID)
	c.persistLocked()
}

func (c *archiveController) startedAt(runID string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.StartedAt[runID]
}

func (c *archiveController) loadState() {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	var state archiveRuntimeState
	if json.Unmarshal(data, &state) != nil {
		return
	}
	if state.StartedAt == nil {
		state.StartedAt = map[string]int64{}
	}
	c.state = state
}

func (c *archiveController) persistLocked() {
	if c.path == "" {
		return
	}
	data, err := json.MarshalIndent(c.state, "", "  ")
	if err != nil {
		return
	}
	_ = writeAtomic(c.path, data, 0o644)
}
