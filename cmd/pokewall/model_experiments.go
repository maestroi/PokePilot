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
	State              string   `json:"state"`
	DeploymentID       string   `json:"deployment_id,omitempty"`
	ModelID            string   `json:"model_id,omitempty"`
	Health             string   `json:"health,omitempty"`
	ActiveLeases       int      `json:"active_leases,omitempty"`
	MaxParallelWorkers int      `json:"max_parallel_workers,omitempty"`
	LeaseRunIDs        []string `json:"lease_run_ids,omitempty"`
	Error              string   `json:"error,omitempty"`
}

type endpointModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
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
	controller.backfillRunsFromExperiments()
	controller.attachDeploymentsToTiles()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", controller.handleModels)
	mux.HandleFunc("POST /v1/models", controller.handleSaveModel)
	mux.HandleFunc("POST /v1/models/test", controller.handleTestModel)
	mux.HandleFunc("PATCH /v1/models/{id}", controller.handlePatchModel)
	mux.HandleFunc("DELETE /v1/models/{id}", controller.handleDeleteModel)
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
	c.reconcileHostLeases()
	deployments := c.enabledDeployments()
	views := make([]deploymentView, 0, len(deployments))
	statuses := map[string]modelHostStatus{}
	queued := c.queuedByDeployment()
	active := c.activeByDeployment()
	for _, d := range deployments {
		view := deploymentView{ModelDeployment: d, State: "ready", ActiveLeases: active[d.ID], Queued: queued[d.ID]}
		if d.ControlURL == "" && d.Discover {
			resolved, err := c.resolveDeployment(d)
			if err != nil {
				view.State = "unavailable"
				view.Error = err.Error()
			} else {
				view.ModelDeployment = resolved
			}
		}
		if d.ControlURL != "" {
			status, err := c.hostStatus(d)
			if err != nil {
				view.State = "unavailable"
				view.Error = err.Error()
			} else {
				statuses[d.ControlURL] = status
				view.Loaded, view.Error = status.DeploymentID, status.Error
				if status.DeploymentID == d.ID && strings.TrimSpace(status.ModelID) != "" {
					view.ModelID = status.ModelID
				}
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

func (c *modelExperimentController) reloadRegistry() error {
	registry, err := farm.LoadModelRegistry(c.registrySource)
	if err != nil {
		return err
	}
	c.registryMu.Lock()
	c.registry = registry
	c.registryMu.Unlock()
	return nil
}

func (c *modelExperimentController) handleSaveModel(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(c.registrySource) == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "model registry is not configured"})
		return
	}
	var deployment farm.ModelDeployment
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSmallControlBody)).Decode(&deployment); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if deployment.MaxParallelWorkers == 0 {
		deployment.MaxParallelWorkers = 1
	}
	updated, err := farm.UpsertModelDeployment(c.registrySource, deployment)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := c.reloadRegistry(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, deploymentView{ModelDeployment: updated, State: "ready"})
}

func (c *modelExperimentController) handleTestModel(w http.ResponseWriter, r *http.Request) {
	var deployment farm.ModelDeployment
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSmallControlBody)).Decode(&deployment); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if deployment.MaxParallelWorkers == 0 {
		deployment.MaxParallelWorkers = 1
	}
	if err := (farm.ModelRegistry{Deployments: []farm.ModelDeployment{deployment}}).Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	resolved, err := c.resolveDeployment(deployment)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, deploymentView{ModelDeployment: resolved, State: "ready"})
}

func (c *modelExperimentController) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "deployment id is required"})
		return
	}
	if strings.TrimSpace(c.registrySource) == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "model registry is not configured"})
		return
	}
	active := c.activeByDeployment()[id]
	queued := c.queuedByDeployment()[id]
	if active > 0 || queued > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "deployment still has active or queued runs",
			"active_leases": active,
			"queued": queued,
		})
		return
	}
	if err := farm.DeleteModelDeployment(c.registrySource, id); err != nil {
		if errors.Is(err, farm.ErrDeploymentNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := c.reloadRegistry(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func (c *modelExperimentController) resolvedDeployment(id string) (farm.ModelDeployment, error) {
	d, ok := c.deployment(id)
	if !ok || !d.Enabled {
		return farm.ModelDeployment{}, fmt.Errorf("deployment %q is unavailable", id)
	}
	return c.resolveDeployment(d)
}

// resolveDeployment binds an endpoint declaration to what it is actually
// serving. Switchable model hosts remain authoritative via their control API;
// pinned/generic OpenAI-compatible endpoints can opt into /v1/models discovery.
// We only auto-select when there is one unambiguous model (or the configured
// api_model is present), so a multi-model cloud endpoint is never guessed.
func (c *modelExperimentController) resolveDeployment(d farm.ModelDeployment) (farm.ModelDeployment, error) {
	if d.ControlURL != "" {
		status, err := c.hostStatus(d)
		if err != nil {
			return farm.ModelDeployment{}, err
		}
		if status.DeploymentID == d.ID && strings.TrimSpace(status.ModelID) != "" && status.ModelID != d.ModelID {
			d.ModelID = status.ModelID
			d.Revision, d.Artifact, d.Quantization = "", "", ""
		}
		return d, nil
	}
	if !d.Discover {
		return d, nil
	}

	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(d.Endpoint, "/")+"/models", nil)
	if err != nil {
		return farm.ModelDeployment{}, err
	}
	if d.TokenEnv != "" {
		if token := strings.TrimSpace(os.Getenv(d.TokenEnv)); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return farm.ModelDeployment{}, fmt.Errorf("discover %s: %w", d.Endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return farm.ModelDeployment{}, fmt.Errorf("discover %s: HTTP %s", d.Endpoint, resp.Status)
	}
	var models endpointModelsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&models); err != nil {
		return farm.ModelDeployment{}, fmt.Errorf("discover %s: decode models: %w", d.Endpoint, err)
	}
	ids := make([]string, 0, len(models.Data))
	for _, model := range models.Data {
		if id := strings.TrimSpace(model.ID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return farm.ModelDeployment{}, fmt.Errorf("discover %s: endpoint reported no models", d.Endpoint)
	}
	chosen := ""
	for _, id := range ids {
		if d.APIModel != "" && id == d.APIModel {
			chosen = id
			break
		}
	}
	if chosen == "" && len(ids) == 1 {
		chosen = ids[0]
	}
	if chosen == "" {
		return farm.ModelDeployment{}, fmt.Errorf("discover %s: %d models reported and configured api_model %q did not match", d.Endpoint, len(ids), d.APIModel)
	}
	if chosen != d.APIModel || chosen != d.ModelID {
		d.APIModel = chosen
		d.ModelID = chosen
		d.Label = chosen + " · " + d.Compute
		// A changed runtime model invalidates artifact-specific comparability
		// metadata from the static registry. Keep hardware/engine identity, but
		// do not pretend the old model hash/quantization still applies.
		d.Revision, d.Artifact, d.Quantization = "", "", ""
	}
	return d, nil
}

func (c *modelExperimentController) refreshRunInference(meta runExperimentMeta) (runExperimentMeta, error) {
	d, err := c.resolvedDeployment(meta.Deployment)
	if err != nil {
		return meta, err
	}
	meta.Inference = d.Identity()
	meta.MaxParallelWorkers = d.ParallelLimit()
	c.mu.Lock()
	if _, exists := c.state.Runs[meta.RunID]; exists {
		c.state.Runs[meta.RunID] = meta
		c.persistLocked()
	}
	c.mu.Unlock()
	return meta, nil
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
	meta, ok := c.bindingForRun(spec.RunID, spec.LLMDeployment)
	if !ok {
		copyRecorder(w, capture)
		return
	}
	refreshed, refreshErr := c.refreshRunInference(meta)
	if refreshErr != nil {
		c.requeueLease(spec.RunID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "deployment endpoint unavailable while leasing", "deployment": spec.LLMDeployment, "detail": refreshErr.Error()})
		return
	}
	meta = refreshed
	if meta.Inference.ControlURL != "" {
		status, code, err := c.hostAction(meta.Inference, "/v1/leases/acquire", map[string]any{"run_id": spec.RunID, "deployment_id": meta.Deployment, "max_parallel_workers": c.liveParallelLimit(meta.Deployment)})
		if err != nil || code >= 300 || status.State != "ready" || status.DeploymentID != meta.Deployment {
			c.releaseHostLease(spec.RunID, meta.Inference)
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
	c.reconcileHostLeases()
	c.wall.mu.Lock()
	queue := append([]string(nil), c.wall.queue...)
	c.wall.mu.Unlock()
	if len(queue) == 0 {
		return true // let the normal lease handler produce 204
	}
	for _, runID := range queue {
		meta, ok := c.bindingForRun(runID, c.tileDeployment(runID))
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
	if strings.TrimSpace(meta.Deployment) == "" {
		return false
	}
	limit := c.liveParallelLimit(meta.Deployment)
	active := 0
	for _, dep := range c.liveDeployments(true) {
		if dep == meta.Deployment {
			active++
		}
	}
	return active >= limit
}

func (c *modelExperimentController) activeByDeployment() map[string]int {
	out := map[string]int{}
	for _, dep := range c.liveDeployments(true) {
		out[dep]++
	}
	return out
}

func (c *modelExperimentController) queuedByDeployment() map[string]int {
	out := map[string]int{}
	for _, dep := range c.liveDeployments(false) {
		out[dep]++
	}
	return out
}

func (c *modelExperimentController) liveDeployments(active bool) []string {
	c.wall.mu.Lock()
	type pending struct {
		runID      string
		deployment string
	}
	var rows []pending
	if active {
		for runID, tile := range c.wall.tiles {
			if tile == nil || tile.Finished || (tile.Status != statusLeased && tile.Status != statusRunning) {
				continue
			}
			rows = append(rows, pending{runID: runID, deployment: tile.LLMDeployment})
		}
	} else {
		for _, runID := range c.wall.queue {
			deployment := ""
			if tile := c.wall.tiles[runID]; tile != nil {
				deployment = tile.LLMDeployment
			}
			rows = append(rows, pending{runID: runID, deployment: deployment})
		}
	}
	c.wall.mu.Unlock()

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		dep := row.deployment
		if dep == "" {
			if meta, ok := c.runMeta(row.runID); ok {
				dep = meta.Deployment
			}
		}
		if dep != "" {
			out = append(out, dep)
		}
	}
	return out
}

func (c *modelExperimentController) tileDeployment(runID string) string {
	c.wall.mu.Lock()
	defer c.wall.mu.Unlock()
	if tile := c.wall.tiles[runID]; tile != nil {
		return tile.LLMDeployment
	}
	return ""
}

func (c *modelExperimentController) bindingForRun(runID, deployment string) (runExperimentMeta, bool) {
	if meta, ok := c.runMeta(runID); ok {
		return meta, true
	}
	deployment = strings.TrimSpace(deployment)
	if deployment == "" {
		deployment = c.tileDeployment(runID)
	}
	if deployment == "" {
		return runExperimentMeta{}, false
	}
	if d, ok := c.deployment(deployment); ok {
		c.wall.mu.Lock()
		expID, expArm, expCase := "", "", ""
		if tile := c.wall.tiles[runID]; tile != nil {
			expID, expArm, expCase = tile.ExperimentID, tile.ExperimentArm, tile.ExperimentCase
		}
		c.wall.mu.Unlock()
		return runExperimentMeta{RunID: runID, Deployment: d.ID, Inference: d.Identity(), ExperimentID: expID, ExperimentArm: expArm, ExperimentCase: expCase, MaxParallelWorkers: d.ParallelLimit()}, true
	}
	return runExperimentMeta{RunID: runID, Deployment: deployment}, true
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
	if meta, ok := c.bindingForRun(runID, ""); ok && meta.Inference.ControlURL != "" {
		c.releaseHostLease(runID, meta.Inference)
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
			if meta, ok := c.bindingForRun(runID, stringValue(run["llm_deployment"])); ok {
				run["llm_deployment"] = meta.Deployment
				run["inference"] = meta.Inference
				if meta.ExperimentID != "" {
					run["experiment_id"] = meta.ExperimentID
					run["experiment_arm"] = meta.ExperimentArm
					run["experiment_case"] = meta.ExperimentCase
				}
				if meta.ComparableHash != "" {
					run["comparable_hash"] = meta.ComparableHash
				}
				if meta.MaxParallelWorkers > 0 {
					run["max_parallel_workers"] = meta.MaxParallelWorkers
				}
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
	request.Game = strings.ToLower(strings.TrimSpace(request.Game))
	if request.Game == "" {
		request.Game = "pokemon-red"
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
				"run_id": runID, "seed": seed, "game": request.Game, "planner": "llm", "starter": request.Starter, "dest": "", "goal": request.Goal,
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
	GoalSuccesses           int            `json:"goal_successes"`
	BoulderSuccesses        int            `json:"boulder_successes"`
	SuccessRate             float64        `json:"success_rate"`
	Badges                  int            `json:"badges"`
	Rounds                  int            `json:"rounds"`
	Frames                  uint64         `json:"frames"`
	MedianRoundsToGoal      float64        `json:"median_rounds_to_goal"`
	MedianFramesToGoal      uint64         `json:"median_frames_to_goal"`
	AvgRunSeconds           float64        `json:"avg_run_seconds"`
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
	successRounds           []float64
	successFrames           []float64
	runSeconds              float64
	runSamples              int
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

type experimentPairedSummary struct {
	AWins           int `json:"a_wins"`
	BWins           int `json:"b_wins"`
	Ties            int `json:"ties"`
	ComparablePairs int `json:"comparable_pairs"`
	CompletedPairs  int `json:"completed_pairs"`
	ExcludedPairs   int `json:"excluded_pairs"`
}

type experimentIdentityView struct {
	Game           string                 `json:"game"`
	GitRevision    string                 `json:"git_revision,omitempty"`
	ROMIdentity    string                 `json:"rom_identity,omitempty"`
	PromptIdentity string                 `json:"prompt_identity,omitempty"`
	ArmA           farm.InferenceIdentity `json:"arm_a"`
	ArmB           farm.InferenceIdentity `json:"arm_b"`
}

func (c *modelExperimentController) experimentView(record experimentRecord) map[string]any {
	rows := make(map[string]tileRow, len(record.RunIDs))
	for _, id := range record.RunIDs {
		if row, ok := c.wall.snapshotRun(id); ok {
			rows[id] = row
		}
	}
	armA, armB := armAggregate{}, armAggregate{}
	pairs := make([]pairResult, 0, len(record.Request.Seeds))
	paired := experimentPairedSummary{}
	identity := experimentIdentityView{Game: record.Request.Game}
	for _, seed := range record.Request.Seeds {
		caseID := record.ID + "-seed-" + strconv.FormatInt(seed, 10)
		idA, idB := caseID+"-a", caseID+"-b"
		tA, okA := rows[idA]
		tB, okB := rows[idB]
		metaA, haveMetaA := c.runMeta(idA)
		metaB, haveMetaB := c.runMeta(idB)
		if identity.GitRevision == "" && haveMetaA {
			identity.Game = metaA.Comparable.Game
			identity.GitRevision = metaA.Comparable.GitRevision
			identity.ROMIdentity = metaA.Comparable.ROMIdentity
			identity.PromptIdentity = metaA.Comparable.PromptIdentity
			identity.ArmA = metaA.Inference
		}
		if identity.ArmB.DeploymentID == "" && haveMetaB {
			identity.ArmB = metaB.Inference
		}
		pair := pairResult{
			Seed: seed, StatusA: tA.Status, StatusB: tB.Status,
			SuccessA: rowGoalSuccess(tA, record.Request.Goal), SuccessB: rowGoalSuccess(tB, record.Request.Goal),
		}
		pair.Comparable, pair.Reason = comparablePair(metaA, haveMetaA, metaB, haveMetaB)
		if !pair.Comparable {
			paired.ExcludedPairs++
			pairs = append(pairs, pair)
			continue
		}
		paired.ComparablePairs++
		if okA {
			accumulateArm(&armA, tA, pair.SuccessA)
		}
		if okB {
			accumulateArm(&armB, tB, pair.SuccessB)
		}
		if tA.Status == statusDone && tB.Status == statusDone {
			paired.CompletedPairs++
			switch {
			case pair.SuccessA && !pair.SuccessB:
				pair.Winner = "a"
				paired.AWins++
			case pair.SuccessB && !pair.SuccessA:
				pair.Winner = "b"
				paired.BWins++
			default:
				pair.Winner = "tie"
				paired.Ties++
			}
		}
		pairs = append(pairs, pair)
	}
	finalizeArm(&armA)
	finalizeArm(&armB)
	return map[string]any{
		"id": record.ID, "name": record.Name, "created_at": record.CreatedAt, "request": record.Request,
		"total_pairs": len(record.Request.Seeds), "arm_a": armA, "arm_b": armB,
		"paired": paired, "identity": identity, "pairs": pairs,
	}
}

func comparablePair(a runExperimentMeta, haveA bool, b runExperimentMeta, haveB bool) (bool, string) {
	if !haveA || !haveB {
		return false, "experiment run metadata is missing"
	}
	if a.ComparableHash == "" || b.ComparableHash == "" || a.ComparableHash != b.ComparableHash {
		return false, "matched configuration identity differs or is missing"
	}
	var missing []string
	for name, value := range map[string]string{
		"game": a.Comparable.Game, "git revision": a.Comparable.GitRevision,
		"ROM identity": a.Comparable.ROMIdentity, "prompt identity": a.Comparable.PromptIdentity,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return false, "missing comparability identity: " + strings.Join(missing, ", ")
	}
	return true, ""
}

func accumulateArm(out *armAggregate, row tileRow, goalSuccess bool) {
	out.Runs++
	if out.ReplanReasons == nil {
		out.ReplanReasons = map[string]int{}
	}
	if out.FinalStopReasons == nil {
		out.FinalStopReasons = map[string]int{}
	}
	if row.Status == statusDone && row.Reason != "" {
		out.FinalStopReasons[row.Reason]++
	}
	if row.Status == statusDone {
		out.Done++
	}
	if goalSuccess {
		out.GoalSuccesses++
	}
	if rowBoulderSuccess(row) {
		out.BoulderSuccesses++
	}
	if row.Player != nil {
		out.Badges += len(row.Player.Badges)
	}
	out.Frames += row.Frame
	if row.Status == statusDone && row.QueuedAt > 0 && row.EndedAt >= row.QueuedAt {
		out.runSeconds += float64(row.EndedAt - row.QueuedAt)
		out.runSamples++
	}
	if row.Stats != nil {
		s := row.Stats
		out.Rounds += s.Rounds
		if goalSuccess {
			if s.Rounds > 0 {
				out.successRounds = append(out.successRounds, float64(s.Rounds))
			}
			if row.Frame > 0 {
				out.successFrames = append(out.successFrames, float64(row.Frame))
			}
		}
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
		out.SuccessRate = float64(out.GoalSuccesses) / float64(out.Done)
	}
	if out.runSamples > 0 {
		out.AvgRunSeconds = out.runSeconds / float64(out.runSamples)
	}
	if len(out.successRounds) > 0 {
		sort.Float64s(out.successRounds)
		out.MedianRoundsToGoal = percentile(out.successRounds, 0.50)
	}
	if len(out.successFrames) > 0 {
		sort.Float64s(out.successFrames)
		out.MedianFramesToGoal = uint64(percentile(out.successFrames, 0.50))
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

func rowGoalSuccess(row tileRow, goal string) bool {
	if row.Stats != nil && row.Stats.GoalComplete {
		return true
	}
	if strings.Contains(strings.ToLower(goal), "boulder") {
		return rowBoulderSuccess(row)
	}
	return false
}

func rowBoulderSuccess(row tileRow) bool {
	if row.Player != nil {
		for _, badge := range row.Player.Badges {
			if strings.EqualFold(badge, "boulder") || strings.EqualFold(badge, "boulder badge") {
				return true
			}
		}
	}
	return row.Stats != nil && row.Stats.GoalComplete && strings.Contains(strings.ToLower(row.Stats.GoalSummary), "boulder")
}

func experimentROMIdentity(gameID string) string {
	gameID = strings.ToUpper(strings.TrimSpace(gameID))
	gameID = strings.NewReplacer("-", "_", " ", "_").Replace(gameID)
	if gameID != "" {
		if value := strings.TrimSpace(os.Getenv("POKEPILOT_ROM_SHA256_" + gameID)); value != "" {
			return value
		}
	}
	return strings.TrimSpace(os.Getenv("POKEPILOT_ROM_SHA256"))
}

func (c *modelExperimentController) resolveRunMeta(raw map[string]any, deployment, experimentID, arm, caseID string) (runExperimentMeta, error) {
	d, err := c.resolvedDeployment(deployment)
	if err != nil {
		return runExperimentMeta{}, err
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
		GitRevision: c.wall.Version, ROMIdentity: experimentROMIdentity(stringValue(raw["game"])), PromptIdentity: strings.TrimSpace(os.Getenv("POKEPILOT_PROMPT_SHA256")),
		Game: stringValue(raw["game"]), Seed: int64Number(raw["seed"]), Starter: stringValue(raw["starter"]), Goal: stringValue(raw["goal"]), PlayStyle: stringValue(raw["play_style"]),
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

func (c *modelExperimentController) backfillRunsFromExperiments() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, record := range c.state.Experiments {
		for _, runID := range record.RunIDs {
			if _, ok := c.state.Runs[runID]; ok {
				continue
			}
			arm := record.Request.ArmA
			armKey := "a"
			if strings.HasSuffix(runID, "-b") {
				arm = record.Request.ArmB
				armKey = "b"
			}
			if strings.TrimSpace(arm.Deployment) == "" {
				continue
			}
			meta := runExperimentMeta{RunID: runID, Deployment: arm.Deployment, ExperimentID: record.ID, ExperimentArm: armKey, ExperimentCase: strings.TrimSuffix(strings.TrimSuffix(runID, "-a"), "-b"), MaxParallelWorkers: arm.MaxParallelWorkers}
			if d, ok := c.deployment(arm.Deployment); ok {
				meta.Inference = d.Identity()
				if meta.MaxParallelWorkers <= 0 {
					meta.MaxParallelWorkers = d.ParallelLimit()
				}
			}
			c.state.Runs[runID] = meta
		}
	}
}

func (c *modelExperimentController) attachDeploymentsToTiles() {
	c.mu.Lock()
	runs := make(map[string]runExperimentMeta, len(c.state.Runs))
	for id, meta := range c.state.Runs {
		runs[id] = meta
	}
	c.mu.Unlock()

	c.wall.mu.Lock()
	defer c.wall.mu.Unlock()
	for id, meta := range runs {
		tile := c.wall.tiles[id]
		if tile == nil || meta.Deployment == "" {
			continue
		}
		if tile.LLMDeployment == "" {
			tile.LLMDeployment = meta.Deployment
		}
		if tile.ExperimentID == "" {
			tile.ExperimentID, tile.ExperimentArm, tile.ExperimentCase = meta.ExperimentID, meta.ExperimentArm, meta.ExperimentCase
		}
	}
}

func (c *modelExperimentController) liveHostLeaseRunIDs() map[string]struct{} {
	c.wall.mu.Lock()
	defer c.wall.mu.Unlock()
	out := make(map[string]struct{})
	for runID, tile := range c.wall.tiles {
		if tile == nil || tile.Finished {
			continue
		}
		if tile.Status == statusLeased || tile.Status == statusRunning {
			out[runID] = struct{}{}
		}
	}
	return out
}

func (c *modelExperimentController) releaseHostLease(runID string, identity farm.InferenceIdentity) {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(identity.ControlURL) == "" {
		return
	}
	_, _, _ = c.hostAction(identity, "/v1/leases/release", map[string]string{"run_id": runID, "deployment_id": identity.DeploymentID})
}

func (c *modelExperimentController) reconcileHostLeases() {
	live := c.liveHostLeaseRunIDs()
	seen := map[string]struct{}{}
	for _, d := range c.enabledDeployments() {
		if strings.TrimSpace(d.ControlURL) == "" {
			continue
		}
		if _, ok := seen[d.ControlURL]; ok {
			continue
		}
		seen[d.ControlURL] = struct{}{}
		status, err := c.hostStatusIdentity(d.Identity())
		if err != nil {
			continue
		}
		for _, runID := range status.LeaseRunIDs {
			if _, ok := live[runID]; ok {
				continue
			}
			c.releaseHostLease(runID, d.Identity())
		}
	}
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
