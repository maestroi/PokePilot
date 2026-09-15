package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const modelExperimentStateFile = "model-experiments.json"

type runExperimentMeta struct {
	RunID              string                   `json:"run_id"`
	Deployment         string                   `json:"deployment"`
	Inference          farm.InferenceIdentity   `json:"inference"`
	ExperimentID       string                   `json:"experiment_id,omitempty"`
	ExperimentArm      string                   `json:"experiment_arm,omitempty"`
	ExperimentCase     string                   `json:"experiment_case,omitempty"`
	Comparable         farm.ComparableRunConfig `json:"comparable"`
	ComparableHash     string                   `json:"comparable_hash"`
	PlayStyle          string                   `json:"play_style,omitempty"`
	RiskTolerance      string                   `json:"risk_tolerance,omitempty"`
	WildEncounters     string                   `json:"wild_encounters,omitempty"`
	MaxParallelWorkers int                      `json:"max_parallel_workers,omitempty"`
}

type experimentRecord struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	CreatedAt time.Time              `json:"created_at"`
	Request   farm.ExperimentRequest `json:"request"`
	RunIDs    []string               `json:"run_ids"`
}

type modelExperimentState struct {
	Runs        map[string]runExperimentMeta `json:"runs"`
	Experiments map[string]experimentRecord  `json:"experiments"`
}

type modelExperimentController struct {
	wall           *Wall
	fallback       http.Handler
	registrySource string
	registry       farm.ModelRegistry
	registryMu     sync.RWMutex
	mu             sync.Mutex
	leaseMu        sync.Mutex
	state          modelExperimentState
	path           string
	client         *http.Client
}

type deploymentView struct {
	farm.ModelDeployment
	State        string `json:"state"`
	Loaded       string `json:"loaded_deployment,omitempty"`
	ActiveLeases int    `json:"active_leases,omitempty"`
	Queued       int    `json:"queued,omitempty"`
	Error        string `json:"error,omitempty"`
}

type modelHostStatus struct {
	State              string `json:"state"`
	DeploymentID       string `json:"deployment_id,omitempty"`
	ModelID            string `json:"model_id,omitempty"`
	Health             string `json:"health,omitempty"`
	ActiveLeases       int    `json:"active_leases,omitempty"`
	MaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`
	Error              string `json:"error,omitempty"`
}

// modelExperimentHTTPHandler adds #723/#724 operator behavior around the wall
// without changing the runner-facing compatibility handler. If no registry is
// configured the wrapper is a no-op for legacy runs.
func modelExperimentHTTPHandler(w *Wall, fallback http.Handler) http.Handler {
	controller := &modelExperimentController{
		wall: w, fallback: fallback,
		state:  modelExperimentState{Runs: map[string]runExperimentMeta{}, Experiments: map[string]experimentRecord{}},
		client: &http.Client{Timeout: 2 * time.Second},
	}
	if w.dumpsDir != "" {
		controller.path = filepath.Join(w.dumpsDir, modelExperimentStateFile)
		controller.loadState()
	}
	if path := strings.TrimSpace(os.Getenv("POKEPILOT_MODEL_REGISTRY")); path != "" {
		controller.registrySource = path
		registry, err := farm.LoadModelRegistry(path)
		if err != nil {
			logModelExperiment("model registry %s: %v", path, err)
		} else {
			controller.registry = registry
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", controller.handleModels)
	mux.HandleFunc("PATCH /v1/models/{id}", controller.handlePatchModel)
	mux.HandleFunc("POST /v1/experiments", controller.handleCreateExperiment)
	mux.HandleFunc("GET /v1/experiments", controller.handleExperiments)
	mux.HandleFunc("GET /v1/experiments/{id}", controller.handleExperiment)
	mux.HandleFunc("POST /v1/specs", controller.handleSpec)
	mux.HandleFunc("POST /v1/lease", controller.handleLease)
	mux.HandleFunc("POST /v1/runs/{id}/finish", controller.handleFinish)
	mux.HandleFunc("GET /v1/dashboard", controller.handleDashboard)
	mux.Handle("/", fallback)
	return mux
}

func logModelExperiment(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "pokewall: "+format+"\n", args...)
}

func (c *modelExperimentController) handleModels(w http.ResponseWriter, _ *http.Request) {
	deployments := c.enabledDeployments()
	views := make([]deploymentView, 0, len(deployments))
	statuses := map[string]modelHostStatus{}
	queued := c.queuedByDeployment()
	active := c.activeByDeployment()
	for _, d := range deployments {
		view := deploymentView{ModelDeployment: d, State: "ready", ActiveLeases: active[d.ID], Queued: queued[d.ID]}
		if d.ControlURL != "" {
			status, err := c.hostStatus(d)
			if err != nil {
				view.State = "unavailable"
				view.Error = err.Error()
			} else {
				statuses[d.ControlURL] = status
				view.Loaded, view.Error = status.DeploymentID, status.Error
				if status.ActiveLeases > view.ActiveLeases {
					view.ActiveLeases = status.ActiveLeases
				}
				switch {
				case status.State == "loading" && status.DeploymentID == d.ID:
					view.State = "loading"
				case status.State == "ready" && status.DeploymentID == d.ID:
					view.State = "ready"
				case status.ActiveLeases > 0 && status.DeploymentID != d.ID:
					view.State = "busy"
				case status.State == "failed" && status.DeploymentID == d.ID:
					view.State = "failed"
				default:
					view.State = "available"
				}
			}
		}
		views = append(views, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": views, "hosts": statuses})
}

func (c *modelExperimentController) handlePatchModel(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "deployment id is required"})
		return
	}
	if strings.TrimSpace(c.registrySource) == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "model registry is not configured"})
		return
	}
	var body struct {
		MaxParallelWorkers int `json:"max_parallel_workers"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSmallControlBody)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	updated, err := farm.UpdateDeploymentParallelLimit(c.registrySource, id, body.MaxParallelWorkers)
	if errors.Is(err, farm.ErrDeploymentNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	c.registryMu.Lock()
	_ = c.registry.SetParallelLimit(id, body.MaxParallelWorkers)
	c.registryMu.Unlock()
	writeJSON(w, http.StatusOK, deploymentView{ModelDeployment: updated, State: "ready"})
}

func (c *modelExperimentController) enabledDeployments() []farm.ModelDeployment {
	c.registryMu.RLock()
	defer c.registryMu.RUnlock()
	return c.registry.EnabledDeployments()
}

func (c *modelExperimentController) deployment(id string) (farm.ModelDeployment, bool) {
	c.registryMu.RLock()
	defer c.registryMu.RUnlock()
	return c.registry.Deployment(id)
}

func (c *modelExperimentController) liveParallelLimit(id string) int {
	if d, ok := c.deployment(id); ok {
		return d.ParallelLimit()
	}
	return 1
}

func (c *modelExperimentController) handleSpec(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSmallControlBody))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	deployment, _ := raw["llm_deployment"].(string)
	if strings.TrimSpace(deployment) == "" {
		c.forwardBody(w, r, body)
		return
	}
	meta, err := c.resolveRunMeta(raw, strings.TrimSpace(deployment), "", "", "")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	c.applyDeployment(raw, meta)
	encoded, _ := json.Marshal(raw)
	capture := httptest.NewRecorder()
	c.forwardBody(capture, r, encoded)
	copyRecorder(w, capture)
	if capture.Code >= 200 && capture.Code < 300 {
		c.mu.Lock()
		c.state.Runs[meta.RunID] = meta
		c.persistLocked()
		c.mu.Unlock()
	}
}

func (c *modelExperimentController) handleLease(w http.ResponseWriter, r *http.Request) {
	c.leaseMu.Lock()
	defer c.leaseMu.Unlock()

	if !c.prepareNextDeployment() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	capture := httptest.NewRecorder()
	c.fallback.ServeHTTP(capture, r)
	if capture.Code == http.StatusNoContent || capture.Code < 200 || capture.Code >= 300 {
		copyRecorder(w, capture)
		return
	}
	var spec farm.Spec
	if err := json.Unmarshal(capture.Body.Bytes(), &spec); err != nil {
		copyRecorder(w, capture)
		return
	}
	meta, ok := c.runMeta(spec.RunID)
	if !ok {
		copyRecorder(w, capture)
		return
	}
	if meta.Inference.ControlURL != "" {
		status, code, err := c.hostAction(meta.Inference, "/v1/leases/acquire", map[string]any{"run_id": spec.RunID, "deployment_id": meta.Deployment, "max_parallel_workers": c.liveParallelLimit(meta.Deployment)})
		if err != nil || code >= 300 || status.State != "ready" || status.DeploymentID != meta.Deployment {
			c.requeueLease(spec.RunID)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "deployment lost readiness while leasing", "deployment": meta.Deployment, "status": status})
			return
		}
	}
	spec.LLMDeployment = meta.Deployment
	spec.Inference = &meta.Inference
	spec.ExperimentID, spec.ExperimentArm, spec.ExperimentCase = meta.ExperimentID, meta.ExperimentArm, meta.ExperimentCase
	spec.LLMProfile = c.compatibilityProfile(meta.Deployment, spec.LLMProfile)
	writeJSON(w, http.StatusOK, spec)
}

// prepareNextDeployment keeps model switching outside the runner lease. It may
// initiate a load and return false; idle workers will poll again while the host
// reports loading. Queued specs whose host is busy are skipped in favor of a
// runnable deployment.
func (c *modelExperimentController) prepareNextDeployment() bool {
	c.wall.mu.Lock()
	queue := append([]string(nil), c.wall.queue...)
	c.wall.mu.Unlock()
	if len(queue) == 0 {
		return true // let the normal lease handler produce 204
	}
	for _, runID := range queue {
		meta, ok := c.runMeta(runID)
		if !ok {
			c.moveQueueFront(runID)
			return true
		}
		if c.deploymentAtCapacity(meta) {
			continue
		}
		if meta.Inference.ControlURL == "" {
			c.moveQueueFront(runID)
			return true
		}
		status, err := c.hostStatusIdentity(meta.Inference)
		if err != nil {
			continue
		}
		if status.State == "ready" && status.DeploymentID == meta.Deployment {
			c.moveQueueFront(runID)
			return true
		}
		if status.ActiveLeases > 0 && status.DeploymentID != meta.Deployment {
			continue
		}
		if status.State != "loading" || status.DeploymentID != meta.Deployment {
			_, _, _ = c.hostAction(meta.Inference, "/v1/load", map[string]string{"deployment_id": meta.Deployment})
		}
	}
	return false
}

func (c *modelExperimentController) deploymentAtCapacity(meta runExperimentMeta) bool {
	limit := c.liveParallelLimit(meta.Deployment)
	c.wall.mu.Lock()
	activeIDs := make([]string, 0)
	for runID, tile := range c.wall.tiles {
		if tile != nil && !tile.Finished && (tile.Status == statusLeased || tile.Status == statusRunning) {
			activeIDs = append(activeIDs, runID)
		}
	}
	c.wall.mu.Unlock()
	active := 0
	for _, runID := range activeIDs {
		if other, ok := c.runMeta(runID); ok && other.Deployment == meta.Deployment {
			active++
		}
	}
	return active >= limit
}

func (c *modelExperimentController) activeByDeployment() map[string]int {
	c.wall.mu.Lock()
	activeIDs := make([]string, 0)
	for runID, tile := range c.wall.tiles {
		if tile != nil && !tile.Finished && (tile.Status == statusLeased || tile.Status == statusRunning) {
			activeIDs = append(activeIDs, runID)
		}
	}
	c.wall.mu.Unlock()
	out := map[string]int{}
	for _, runID := range activeIDs {
		if meta, ok := c.runMeta(runID); ok {
			out[meta.Deployment]++
		}
	}
	return out
}

func (c *modelExperimentController) queuedByDeployment() map[string]int {
	c.wall.mu.Lock()
	queue := append([]string(nil), c.wall.queue...)
	c.wall.mu.Unlock()
	out := map[string]int{}
	for _, runID := range queue {
		if meta, ok := c.runMeta(runID); ok {
			out[meta.Deployment]++
		}
	}
	return out
}

func (c *modelExperimentController) requeueLease(runID string) {
	c.wall.mu.Lock()
	tile := c.wall.tiles[runID]
	if tile != nil && !tile.Finished && tile.Status == statusLeased {
		alreadyQueued := false
		for _, queuedID := range c.wall.queue {
			if queuedID == runID {
				alreadyQueued = true
				break
			}
		}
		if !alreadyQueued {
			c.wall.queue = append([]string{runID}, c.wall.queue...)
		}
		tile.Status = statusQueued
		tile.lastUpdate = time.Now()
	}
	c.wall.mu.Unlock()
	c.wall.saveState()
}

func (c *modelExperimentController) moveQueueFront(runID string) {
	c.wall.mu.Lock()
	defer c.wall.mu.Unlock()
	for i, id := range c.wall.queue {
		if id != runID || i == 0 {
			continue
		}
		copy(c.wall.queue[1:i+1], c.wall.queue[0:i])
		c.wall.queue[0] = runID
		return
	}
}

func (c *modelExperimentController) handleFinish(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	capture := httptest.NewRecorder()
	c.fallback.ServeHTTP(capture, r)
	copyRecorder(w, capture)
	if capture.Code < 200 || capture.Code >= 300 {
		return
	}
	if meta, ok := c.runMeta(runID); ok && meta.Inference.ControlURL != "" {
		_, _, _ = c.hostAction(meta.Inference, "/v1/leases/release", map[string]string{"run_id": runID, "deployment_id": meta.Deployment})
	}
}

func (c *modelExperimentController) handleDashboard(w http.ResponseWriter, r *http.Request) {
	capture := httptest.NewRecorder()
	c.fallback.ServeHTTP(capture, r)
	if capture.Code < 200 || capture.Code >= 300 {
		copyRecorder(w, capture)
		return
	}
	var doc map[string]any
	if err := json.Unmarshal(capture.Body.Bytes(), &doc); err != nil {
		copyRecorder(w, capture)
		return
	}
	if runs, ok := doc["runs"].([]any); ok {
		for _, item := range runs {
			run, _ := item.(map[string]any)
			runID, _ := run["run_id"].(string)
			if meta, ok := c.runMeta(runID); ok {
				run["llm_deployment"] = meta.Deployment
				run["inference"] = meta.Inference
				run["experiment_id"] = meta.ExperimentID
				run["experiment_arm"] = meta.ExperimentArm
				run["experiment_case"] = meta.ExperimentCase
				run["comparable_hash"] = meta.ComparableHash
				run["max_parallel_workers"] = meta.MaxParallelWorkers
			}
		}
	}
	writeJSON(w, http.StatusOK, doc)
}

func (c *modelExperimentController) handleCreateExperiment(w http.ResponseWriter, r *http.Request) {
	var request farm.ExperimentRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxSmallControlBody)
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		request.Name = "paired-model-experiment"
	}
	if request.Goal == "" {
		request.Goal = "Earn the Boulder Badge."
	}
	if request.ArmA.Name == "" {
		request.ArmA.Name = "A"
	}
	if request.ArmB.Name == "" {
		request.ArmB.Name = "B"
	}
	if request.ArmA.Deployment == request.ArmB.Deployment || request.ArmA.Deployment == "" || request.ArmB.Deployment == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "arm_a and arm_b must select two distinct deployments"})
		return
	}
	for _, arm := range []*farm.ExperimentArm{&request.ArmA, &request.ArmB} {
		d, ok := c.deployment(arm.Deployment)
		if !ok || !d.Enabled {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "deployment " + arm.Deployment + " is unavailable"})
			return
		}
		limit := d.ParallelLimit()
		if arm.MaxParallelWorkers < 0 || arm.MaxParallelWorkers > limit {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("deployment %s allows at most %d parallel worker(s)", arm.Deployment, limit)})
			return
		}
		if arm.MaxParallelWorkers == 0 {
			arm.MaxParallelWorkers = limit
		}
	}
	seeds := append([]int64(nil), request.Seeds...)
	if len(seeds) == 0 {
		count := request.SeedCount
		if count <= 0 {
			count = 20
		}
		if count > 1000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "seed_count may not exceed 1000"})
			return
		}
		for i := 1; i <= count; i++ {
			seeds = append(seeds, int64(i))
		}
	}
	request.Seeds, request.SeedCount = seeds, len(seeds)
	experimentID := "exp-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	record := experimentRecord{ID: experimentID, Name: request.Name, CreatedAt: time.Now().UTC(), Request: request}

	for _, seed := range seeds {
		caseID := experimentID + "-seed-" + strconv.FormatInt(seed, 10)
		for _, arm := range []struct {
			key string
			cfg farm.ExperimentArm
		}{{"a", request.ArmA}, {"b", request.ArmB}} {
			runID := caseID + "-" + arm.key
			raw := map[string]any{
				"run_id": runID, "seed": seed, "planner": "llm", "starter": request.Starter, "dest": "", "goal": request.Goal,
				"llm_deployment": arm.cfg.Deployment, "reasoning_effort": request.ReasoningEffort, "max_parallel_workers": arm.cfg.MaxParallelWorkers,
				"fps": request.FPS, "max_rounds": request.MaxRounds, "max_frames": request.MaxFrames,
				"play_style": request.PlayStyle, "risk_tolerance": request.RiskTolerance, "wild_encounters": request.WildEncounters,
			}
			meta, err := c.resolveRunMeta(raw, arm.cfg.Deployment, experimentID, arm.key, caseID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			c.applyDeployment(raw, meta)
			body, _ := json.Marshal(raw)
			result := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/specs", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			c.fallback.ServeHTTP(result, req)
			if result.Code < 200 || result.Code >= 300 {
				writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("enqueue %s failed: %s", runID, strings.TrimSpace(result.Body.String()))})
				return
			}
			c.mu.Lock()
			c.state.Runs[runID] = meta
			c.mu.Unlock()
			record.RunIDs = append(record.RunIDs, runID)
		}
	}
	c.mu.Lock()
	c.state.Experiments[experimentID] = record
	c.persistLocked()
	c.mu.Unlock()
	writeJSON(w, http.StatusCreated, c.experimentView(record))
}

func (c *modelExperimentController) handleExperiments(w http.ResponseWriter, _ *http.Request) {
	c.mu.Lock()
	records := make([]experimentRecord, 0, len(c.state.Experiments))
	for _, record := range c.state.Experiments {
		records = append(records, record)
	}
	c.mu.Unlock()
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.After(records[j].CreatedAt) })
	out := make([]any, 0, len(records))
	for _, record := range records {
		out = append(out, c.experimentView(record))
	}
	writeJSON(w, http.StatusOK, map[string]any{"experiments": out})
}

func (c *modelExperimentController) handleExperiment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c.mu.Lock()
	record, ok := c.state.Experiments[id]
	c.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "experiment not found"})
		return
	}
	writeJSON(w, http.StatusOK, c.experimentView(record))
}

type armAggregate struct {
	Runs                    int            `json:"runs"`
	Done                    int            `json:"done"`
	BoulderSuccesses        int            `json:"boulder_successes"`
	SuccessRate             float64        `json:"success_rate"`
	Badges                  int            `json:"badges"`
	Rounds                  int            `json:"rounds"`
	Frames                  uint64         `json:"frames"`
	Calls                   int            `json:"calls"`
	StrategicCalls          int            `json:"strategic_calls"`
	StrategicRejected       int            `json:"strategic_rejected"`
	PlanStepsProduced       int            `json:"plan_steps_produced"`
	PlanExecutions          int            `json:"plan_executions"`
	StepsSkipped            int            `json:"steps_skipped"`
	PlanExecutionFraction   float64        `json:"plan_execution_fraction"`
	Rejected                int            `json:"rejected"`
	TransportErrors         int            `json:"transport_errors"`
	Fallbacks               int            `json:"fallbacks"`
	PromptTokens            int            `json:"prompt_tokens"`
	CompletionTokens        int            `json:"completion_tokens"`
	StrategicSeconds        float64        `json:"strategic_seconds"`
	AvgStrategicCall        float64        `json:"avg_strategic_call_seconds"`
	P50StrategicCall        float64        `json:"p50_strategic_call_seconds"`
	P95StrategicCall        float64        `json:"p95_strategic_call_seconds"`
	AvgPrefillTPS           float64        `json:"avg_prefill_tps"`
	AvgDecodeTPS            float64        `json:"avg_decode_tps"`
	Blackouts               int            `json:"blackouts"`
	ObjectiveFailures       int            `json:"objective_failures"`
	StagnationReplans       int            `json:"stagnation_replans"`
	PlanExhaustionReplans   int            `json:"plan_exhaustion_replans"`
	ReplanReasons           map[string]int `json:"replan_reasons,omitempty"`
	FinalStopReasons        map[string]int `json:"final_stop_reasons,omitempty"`
	StrategicRecordsDropped int            `json:"strategic_records_dropped,omitempty"`
	latencies               []float64
	prefillSum              float64
	prefillSamples          int
	decodeSum               float64
	decodeSamples           int
}

type pairResult struct {
	Seed       int64  `json:"seed"`
	Comparable bool   `json:"comparable"`
	StatusA    string `json:"status_a"`
	StatusB    string `json:"status_b"`
	SuccessA   bool   `json:"success_a"`
	SuccessB   bool   `json:"success_b"`
	Winner     string `json:"winner,omitempty"`
	Reason     string `json:"non_comparable_reason,omitempty"`
}

func (c *modelExperimentController) experimentView(record experimentRecord) map[string]any {
	c.wall.mu.Lock()
	tiles := map[string]Tile{}
	for _, id := range record.RunIDs {
		if t := c.wall.tiles[id]; t != nil {
			tiles[id] = *t
		}
	}
	c.wall.mu.Unlock()
	armA, armB := armAggregate{}, armAggregate{}
	pairs := make([]pairResult, 0, len(record.Request.Seeds))
	winsA, winsB, ties := 0, 0, 0
	for _, seed := range record.Request.Seeds {
		caseID := record.ID + "-seed-" + strconv.FormatInt(seed, 10)
		idA, idB := caseID+"-a", caseID+"-b"
		tA, okA := tiles[idA]
		tB, okB := tiles[idB]
		if okA {
			accumulateArm(&armA, tA)
		}
		if okB {
			accumulateArm(&armB, tB)
		}
		metaA, haveMetaA := c.runMeta(idA)
		metaB, haveMetaB := c.runMeta(idB)
		pair := pairResult{Seed: seed, StatusA: tA.Status, StatusB: tB.Status, SuccessA: tileBoulderSuccess(tA), SuccessB: tileBoulderSuccess(tB)}
		pair.Comparable = haveMetaA && haveMetaB && metaA.ComparableHash != "" && metaA.ComparableHash == metaB.ComparableHash
		if !pair.Comparable {
			pair.Reason = "matched configuration identity differs or is missing"
		} else if tA.Status == statusDone && tB.Status == statusDone {
			switch {
			case pair.SuccessA && !pair.SuccessB:
				pair.Winner = "a"
				winsA++
			case pair.SuccessB && !pair.SuccessA:
				pair.Winner = "b"
				winsB++
			default:
				pair.Winner = "tie"
				ties++
			}
		}
		pairs = append(pairs, pair)
	}
	finalizeArm(&armA)
	finalizeArm(&armB)
	return map[string]any{
		"id": record.ID, "name": record.Name, "created_at": record.CreatedAt, "request": record.Request,
		"total_pairs": len(record.Request.Seeds), "arm_a": armA, "arm_b": armB,
		"paired": map[string]int{"a_wins": winsA, "b_wins": winsB, "ties": ties}, "pairs": pairs,
	}
}

func accumulateArm(out *armAggregate, tile Tile) {
	out.Runs++
	if out.ReplanReasons == nil {
		out.ReplanReasons = map[string]int{}
	}
	if out.FinalStopReasons == nil {
		out.FinalStopReasons = map[string]int{}
	}
	if tile.Status == statusDone && tile.Reason != "" {
		out.FinalStopReasons[tile.Reason]++
	}
	if tile.Status == statusDone {
		out.Done++
	}
	if tileBoulderSuccess(tile) {
		out.BoulderSuccesses++
	}
	if tile.Player != nil {
		out.Badges += len(tile.Player.Badges)
	}
	out.Frames += tile.Frame
	if tile.Stats != nil {
		s := tile.Stats
		out.Rounds += s.Rounds
		out.Calls += s.Calls
		out.StrategicCalls += s.StrategicCalls
		out.PlanExecutions += s.PlanExecutions
		out.StepsSkipped += s.StepsSkipped
		out.Rejected += s.Rejected
		out.TransportErrors += s.Transport
		out.Fallbacks += s.Fallbacks
		out.PromptTokens += s.PromptTokens
		out.CompletionTokens += s.CompletionTokens
		out.StrategicSeconds += s.StrategicSeconds
		out.StrategicRecordsDropped += s.StrategicRecordsDropped
		for reason, count := range s.ReplanReasons {
			out.ReplanReasons[reason] += count
		}
		out.Blackouts += s.ReplanReasons["blackout"]
		out.ObjectiveFailures += s.ReplanReasons["objective_failed"]
		out.StagnationReplans += s.ReplanReasons["stagnation"] + s.ReplanReasons["stuck"]
		out.PlanExhaustionReplans += s.ReplanReasons["plan_exhausted"] + s.ReplanReasons["plan_exhaustion"]
		for _, record := range s.StrategicRecords {
			out.latencies = append(out.latencies, record.DurationSeconds)
			out.PlanStepsProduced += len(record.PlanSteps)
			if record.Rejected {
				out.StrategicRejected++
			}
			if record.PrefillTPS > 0 {
				out.prefillSum += record.PrefillTPS
				out.prefillSamples++
			}
			if record.DecodeTPS > 0 {
				out.decodeSum += record.DecodeTPS
				out.decodeSamples++
			}
		}
	}
}

func finalizeArm(out *armAggregate) {
	if out.Done > 0 {
		out.SuccessRate = float64(out.BoulderSuccesses) / float64(out.Done)
	}
	if out.StrategicCalls > 0 {
		out.AvgStrategicCall = out.StrategicSeconds / float64(out.StrategicCalls)
	}
	if total := out.PlanExecutions + out.StepsSkipped; total > 0 {
		out.PlanExecutionFraction = float64(out.PlanExecutions) / float64(total)
	}
	if len(out.latencies) > 0 {
		sort.Float64s(out.latencies)
		out.P50StrategicCall = percentile(out.latencies, 0.50)
		out.P95StrategicCall = percentile(out.latencies, 0.95)
	}
	if out.prefillSamples > 0 {
		out.AvgPrefillTPS = out.prefillSum / float64(out.prefillSamples)
	}
	if out.decodeSamples > 0 {
		out.AvgDecodeTPS = out.decodeSum / float64(out.decodeSamples)
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	index := int(float64(len(sorted)-1)*p + 0.5)
	return sorted[index]
}

func tileBoulderSuccess(tile Tile) bool {
	if tile.Player != nil {
		for _, badge := range tile.Player.Badges {
			if strings.EqualFold(badge, "boulder") || strings.EqualFold(badge, "boulder badge") {
				return true
			}
		}
	}
	return tile.Stats != nil && tile.Stats.GoalComplete && strings.Contains(strings.ToLower(tile.Stats.GoalSummary), "boulder")
}

func (c *modelExperimentController) resolveRunMeta(raw map[string]any, deployment, experimentID, arm, caseID string) (runExperimentMeta, error) {
	d, ok := c.deployment(deployment)
	if !ok || !d.Enabled {
		return runExperimentMeta{}, fmt.Errorf("deployment %q is unavailable", deployment)
	}
	runID, _ := raw["run_id"].(string)
	if strings.TrimSpace(runID) == "" {
		return runExperimentMeta{}, fmt.Errorf("run_id is required")
	}
	parallel := intNumber(raw["max_parallel_workers"])
	if parallel < 0 {
		return runExperimentMeta{}, fmt.Errorf("max_parallel_workers may not be negative")
	}
	if parallel == 0 {
		parallel = d.ParallelLimit()
	}
	if parallel > d.ParallelLimit() {
		return runExperimentMeta{}, fmt.Errorf("deployment %q allows at most %d parallel worker(s)", deployment, d.ParallelLimit())
	}
	comparable := farm.ComparableRunConfig{
		GitRevision: c.wall.Version, ROMIdentity: strings.TrimSpace(os.Getenv("POKEPILOT_ROM_SHA256")), PromptIdentity: strings.TrimSpace(os.Getenv("POKEPILOT_PROMPT_SHA256")),
		Seed: int64Number(raw["seed"]), Starter: stringValue(raw["starter"]), Goal: stringValue(raw["goal"]), PlayStyle: stringValue(raw["play_style"]),
		RiskTolerance: stringValue(raw["risk_tolerance"]), WildEncounters: stringValue(raw["wild_encounters"]), ReasoningEffort: stringValue(raw["reasoning_effort"]),
		FPS: intNumber(raw["fps"]), MaxRounds: intNumber(raw["max_rounds"]), MaxFrames: intNumber(raw["max_frames"]), MaxParallelWorkers: parallel,
	}
	blob, _ := json.Marshal(comparable)
	hash := sha256.Sum256(blob)
	return runExperimentMeta{RunID: runID, Deployment: deployment, Inference: d.Identity(), ExperimentID: experimentID, ExperimentArm: arm, ExperimentCase: caseID, Comparable: comparable, ComparableHash: hex.EncodeToString(hash[:]), PlayStyle: comparable.PlayStyle, RiskTolerance: comparable.RiskTolerance, WildEncounters: comparable.WildEncounters, MaxParallelWorkers: parallel}, nil
}

func (c *modelExperimentController) applyDeployment(raw map[string]any, meta runExperimentMeta) {
	raw["llm_deployment"] = meta.Deployment
	raw["inference"] = meta.Inference
	raw["llm_profile"] = c.compatibilityProfile(meta.Deployment, stringValue(raw["llm_profile"]))
	if meta.ExperimentID != "" {
		raw["experiment_id"], raw["experiment_arm"], raw["experiment_case"] = meta.ExperimentID, meta.ExperimentArm, meta.ExperimentCase
	}
}

func (c *modelExperimentController) compatibilityProfile(deployment, fallback string) string {
	if d, ok := c.deployment(deployment); ok {
		return d.CompatibilityProfile()
	}
	return fallback
}

func (c *modelExperimentController) runMeta(runID string) (runExperimentMeta, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	meta, ok := c.state.Runs[runID]
	return meta, ok
}

func (c *modelExperimentController) hostStatus(d farm.ModelDeployment) (modelHostStatus, error) {
	return c.hostStatusIdentity(d.Identity())
}
func (c *modelExperimentController) hostStatusIdentity(identity farm.InferenceIdentity) (modelHostStatus, error) {
	status, code, err := c.hostAction(identity, "/v1/status", nil)
	if err != nil {
		return status, err
	}
	if code < 200 || code >= 300 {
		return status, fmt.Errorf("model host status returned %d", code)
	}
	return status, nil
}

func (c *modelExperimentController) hostAction(identity farm.InferenceIdentity, path string, body any) (modelHostStatus, int, error) {
	var status modelHostStatus
	method := http.MethodGet
	var reader io.Reader
	if body != nil {
		method = http.MethodPost
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, strings.TrimRight(identity.ControlURL, "/")+path, reader)
	if err != nil {
		return status, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if identity.TokenEnv != "" {
		if token := strings.TrimSpace(os.Getenv(identity.TokenEnv)); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return status, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_ = json.Unmarshal(data, &status)
		return status, resp.StatusCode, nil
	}
	var wrapped struct {
		Status modelHostStatus `json:"status"`
		Error  string          `json:"error"`
	}
	_ = json.Unmarshal(data, &wrapped)
	if wrapped.Status.State != "" {
		status = wrapped.Status
	}
	if wrapped.Error != "" {
		return status, resp.StatusCode, fmt.Errorf("%s", wrapped.Error)
	}
	return status, resp.StatusCode, fmt.Errorf("model host returned %d", resp.StatusCode)
}

func (c *modelExperimentController) forwardBody(w http.ResponseWriter, r *http.Request, body []byte) {
	clone := r.Clone(r.Context())
	clone.Body = io.NopCloser(bytes.NewReader(body))
	clone.ContentLength = int64(len(body))
	c.fallback.ServeHTTP(w, clone)
}

func copyRecorder(w http.ResponseWriter, rec *httptest.ResponseRecorder) {
	for key, values := range rec.Header() {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(rec.Code)
	_, _ = w.Write(rec.Body.Bytes())
}

func stringValue(v any) string { s, _ := v.(string); return s }
func intNumber(v any) int      { return int(int64Number(v)) }
func int64Number(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	case json.Number:
		out, _ := n.Int64()
		return out
	}
	return 0
}

func (c *modelExperimentController) loadState() {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	var state modelExperimentState
	if json.Unmarshal(data, &state) != nil {
		return
	}
	if state.Runs == nil {
		state.Runs = map[string]runExperimentMeta{}
	}
	if state.Experiments == nil {
		state.Experiments = map[string]experimentRecord{}
	}
	c.state = state
}

func (c *modelExperimentController) persistLocked() {
	if c.path == "" {
		return
	}
	data, err := json.MarshalIndent(c.state, "", "  ")
	if err != nil {
		return
	}
	tmp := c.path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, c.path)
	}
}
