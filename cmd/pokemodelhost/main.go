// Command pokemodelhost owns one switchable inference process on an inference
// VM. It exposes only models declared in its local config, refuses model
// switches while a run lease is active, and verifies the OpenAI-compatible
// endpoint before reporting ready.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

type hostConfig struct {
	HostID      string      `json:"host_id"`
	Compute     string      `json:"compute"`
	Listen      string      `json:"listen,omitempty"`
	TokenEnv    string      `json:"token_env,omitempty"`
	LoadTimeout string      `json:"load_timeout,omitempty"`
	PollEvery   string      `json:"poll_every,omitempty"`
	Models      []hostModel `json:"models"`
}

type hostModel struct {
	DeploymentID       string            `json:"deployment_id"`
	ModelID            string            `json:"model_id"`
	Revision           string            `json:"revision,omitempty"`
	Artifact           string            `json:"artifact,omitempty"`
	Quantization       string            `json:"quantization,omitempty"`
	Endpoint           string            `json:"endpoint"`
	HealthURL          string            `json:"health_url,omitempty"`
	APIModel           string            `json:"api_model"`
	Engine             string            `json:"engine,omitempty"`
	Version            string            `json:"engine_version,omitempty"`
	MaxParallelWorkers int               `json:"max_parallel_workers,omitempty"`
	Command            string            `json:"command,omitempty"`
	Args               []string          `json:"args,omitempty"`
	Env                map[string]string `json:"env,omitempty"`
}

type hostStatus struct {
	HostID             string     `json:"host_id"`
	Compute            string     `json:"compute"`
	State              string     `json:"state"`
	DeploymentID       string     `json:"deployment_id,omitempty"`
	ModelID            string     `json:"model_id,omitempty"`
	Endpoint           string     `json:"endpoint,omitempty"`
	APIModel           string     `json:"api_model,omitempty"`
	Health             string     `json:"health"`
	ActiveLeases       int        `json:"active_leases"`
	MaxParallelWorkers int        `json:"max_parallel_workers,omitempty"`
	LeaseRunIDs        []string   `json:"lease_run_ids,omitempty"`
	Error              string     `json:"error,omitempty"`
	Since              *time.Time `json:"since,omitempty"`
}

type lifecycleService struct {
	mu          sync.Mutex
	cfg         hostConfig
	models      map[string]hostModel
	token       string
	loadTimeout time.Duration
	pollEvery   time.Duration
	client      *http.Client
	state       string
	loaded      string
	lastErr     string
	since       time.Time
	leases      map[string]string
	cmd         *exec.Cmd
	generation  uint64
}

func newLifecycleService(cfg hostConfig) (*lifecycleService, error) {
	if strings.TrimSpace(cfg.HostID) == "" || strings.TrimSpace(cfg.Compute) == "" {
		return nil, errors.New("host_id and compute are required")
	}
	models := make(map[string]hostModel, len(cfg.Models))
	for _, model := range cfg.Models {
		id := strings.TrimSpace(model.DeploymentID)
		if id == "" || strings.TrimSpace(model.ModelID) == "" || strings.TrimSpace(model.Endpoint) == "" || strings.TrimSpace(model.APIModel) == "" {
			return nil, fmt.Errorf("invalid model definition for deployment %q", id)
		}
		if _, exists := models[id]; exists {
			return nil, fmt.Errorf("duplicate deployment_id %q", id)
		}
		if model.MaxParallelWorkers < 0 {
			return nil, fmt.Errorf("deployment %q has invalid max_parallel_workers %d", id, model.MaxParallelWorkers)
		}
		models[id] = model
	}
	loadTimeout := 3 * time.Minute
	if cfg.LoadTimeout != "" {
		parsed, err := time.ParseDuration(cfg.LoadTimeout)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("invalid load_timeout %q", cfg.LoadTimeout)
		}
		loadTimeout = parsed
	}
	pollEvery := 500 * time.Millisecond
	if cfg.PollEvery != "" {
		parsed, err := time.ParseDuration(cfg.PollEvery)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("invalid poll_every %q", cfg.PollEvery)
		}
		pollEvery = parsed
	}
	return &lifecycleService{
		cfg: cfg, models: models, token: strings.TrimSpace(os.Getenv(cfg.TokenEnv)),
		loadTimeout: loadTimeout, pollEvery: pollEvery,
		client: &http.Client{Timeout: 2 * time.Second}, state: "idle", leases: map[string]string{},
	}, nil
}

func (s *lifecycleService) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("GET /v1/status", s.handleStatus)
	mux.HandleFunc("POST /v1/load", s.handleLoad)
	mux.HandleFunc("POST /v1/leases/acquire", s.handleAcquire)
	mux.HandleFunc("POST /v1/leases/release", s.handleRelease)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" && strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) != s.token {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *lifecycleService) handleModels(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	models := make([]hostModel, 0, len(s.models))
	for _, model := range s.models {
		model.Command = ""
		model.Args = nil
		model.Env = nil
		models = append(models, model)
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"host_id": s.cfg.HostID, "compute": s.cfg.Compute, "models": models})
}

func (s *lifecycleService) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.status())
}

type loadRequest struct {
	DeploymentID string `json:"deployment_id"`
}

type leaseRequest struct {
	RunID              string `json:"run_id"`
	DeploymentID       string `json:"deployment_id"`
	MaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return false
	}
	return true
}

func (s *lifecycleService) handleLoad(w http.ResponseWriter, r *http.Request) {
	var in loadRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	status, code, err := s.requestLoad(strings.TrimSpace(in.DeploymentID), "", 0)
	if err != nil {
		writeJSON(w, code, map[string]any{"error": err.Error(), "status": status})
		return
	}
	writeJSON(w, code, status)
}

func (s *lifecycleService) handleAcquire(w http.ResponseWriter, r *http.Request) {
	var in leaseRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	in.RunID = strings.TrimSpace(in.RunID)
	in.DeploymentID = strings.TrimSpace(in.DeploymentID)
	if in.RunID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id is required"})
		return
	}
	status, code, err := s.requestLoad(in.DeploymentID, in.RunID, in.MaxParallelWorkers)
	if err != nil {
		writeJSON(w, code, map[string]any{"error": err.Error(), "status": status})
		return
	}
	writeJSON(w, code, status)
}

func (s *lifecycleService) handleRelease(w http.ResponseWriter, r *http.Request) {
	var in leaseRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	runID := strings.TrimSpace(in.RunID)
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id is required"})
		return
	}
	s.mu.Lock()
	delete(s.leases, runID)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, s.status())
}

func (s *lifecycleService) requestLoad(deploymentID, leaseRunID string, requestedLimit int) (hostStatus, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	model, ok := s.models[deploymentID]
	if !ok {
		status := s.statusLocked()
		return status, http.StatusNotFound, fmt.Errorf("deployment %q is not approved on this host", deploymentID)
	}
	hardLimit := model.MaxParallelWorkers
	if hardLimit <= 0 {
		hardLimit = 1
	}
	limit := requestedLimit
	if limit <= 0 {
		limit = hardLimit
	}
	if current, ok := s.leases[leaseRunID]; leaseRunID != "" && ok && current != deploymentID {
		status := s.statusLocked()
		return status, http.StatusConflict, fmt.Errorf("run %q already leases deployment %q", leaseRunID, current)
	}
	if s.loaded != "" && s.loaded != deploymentID && len(s.leases) > 0 {
		status := s.statusLocked()
		return status, http.StatusConflict, fmt.Errorf("host busy: %d active run lease(s) use %q", len(s.leases), s.loaded)
	}
	if leaseRunID != "" {
		if _, already := s.leases[leaseRunID]; !already && len(s.leases) >= limit {
			status := s.statusLocked()
			return status, http.StatusTooManyRequests, fmt.Errorf("deployment %q is at its %d-worker concurrency limit", deploymentID, limit)
		}
		s.leases[leaseRunID] = deploymentID
	}
	if s.loaded == deploymentID && (s.state == "ready" || s.state == "loading") {
		code := http.StatusOK
		if s.state == "loading" {
			code = http.StatusAccepted
		}
		return s.statusLocked(), code, nil
	}
	if err := s.startLoadLocked(model); err != nil {
		if leaseRunID != "" {
			delete(s.leases, leaseRunID)
		}
		return s.statusLocked(), http.StatusInternalServerError, err
	}
	return s.statusLocked(), http.StatusAccepted, nil
}

func (s *lifecycleService) startLoadLocked(model hostModel) error {
	s.generation++
	generation := s.generation
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
		s.cmd = nil
	}
	s.loaded = model.DeploymentID
	s.state = "loading"
	s.lastErr = ""
	s.since = time.Now().UTC()
	if model.Command != "" {
		cmd := exec.Command(model.Command, model.Args...)
		cmd.Env = append([]string{}, os.Environ()...)
		for key, value := range model.Env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		if err := cmd.Start(); err != nil {
			s.state = "failed"
			s.lastErr = err.Error()
			return fmt.Errorf("start inference server: %w", err)
		}
		s.cmd = cmd
		go s.watchProcess(generation, cmd)
	}
	go s.waitReady(generation, model)
	return nil
}

func (s *lifecycleService) watchProcess(generation uint64, cmd *exec.Cmd) {
	err := cmd.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generation {
		return
	}
	s.state = "failed"
	if err != nil {
		s.lastErr = "inference process exited: " + err.Error()
	} else {
		s.lastErr = "inference process exited before becoming ready"
	}
}

func (s *lifecycleService) waitReady(generation uint64, model hostModel) {
	deadline := time.Now().Add(s.loadTimeout)
	for {
		if s.endpointReady(model) {
			s.mu.Lock()
			if generation == s.generation && s.loaded == model.DeploymentID && s.state == "loading" {
				s.state = "ready"
				s.lastErr = ""
			}
			s.mu.Unlock()
			return
		}
		if time.Now().After(deadline) {
			s.mu.Lock()
			if generation == s.generation && s.state == "loading" {
				s.state = "failed"
				s.lastErr = "endpoint did not become healthy before load timeout"
			}
			s.mu.Unlock()
			return
		}
		time.Sleep(s.pollEvery)
	}
}

func (s *lifecycleService) endpointReady(model hostModel) bool {
	url := strings.TrimSpace(model.HealthURL)
	if url == "" {
		url = strings.TrimRight(model.Endpoint, "/") + "/models"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func (s *lifecycleService) status() hostStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *lifecycleService) statusLocked() hostStatus {
	status := hostStatus{HostID: s.cfg.HostID, Compute: s.cfg.Compute, State: s.state, Health: "unavailable", ActiveLeases: len(s.leases), Error: s.lastErr}
	if !s.since.IsZero() {
		since := s.since
		status.Since = &since
	}
	if model, ok := s.models[s.loaded]; ok {
		status.DeploymentID, status.ModelID, status.Endpoint, status.APIModel = model.DeploymentID, model.ModelID, model.Endpoint, model.APIModel
		status.MaxParallelWorkers = model.MaxParallelWorkers
		if status.MaxParallelWorkers <= 0 {
			status.MaxParallelWorkers = 1
		}
	}
	if s.state == "ready" {
		status.Health = "ready"
	} else if s.state == "loading" {
		status.Health = "loading"
	} else if s.state == "failed" {
		status.Health = "failed"
	}
	for runID := range s.leases {
		status.LeaseRunIDs = append(status.LeaseRunIDs, runID)
	}
	return status
}

func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}

func loadConfig(path string) (hostConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return hostConfig{}, err
	}
	var cfg hostConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return hostConfig{}, err
	}
	return cfg, nil
}

func main() {
	configPath := flag.String("config", os.Getenv("POKEPILOT_MODELHOST_CONFIG"), "model-host JSON config")
	listen := flag.String("http", "", "listen address override")
	flag.Parse()
	if strings.TrimSpace(*configPath) == "" {
		log.Fatal("pokemodelhost: -config or POKEPILOT_MODELHOST_CONFIG is required")
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("pokemodelhost: config: %v", err)
	}
	service, err := newLifecycleService(cfg)
	if err != nil {
		log.Fatalf("pokemodelhost: %v", err)
	}
	addr := strings.TrimSpace(*listen)
	if addr == "" {
		addr = strings.TrimSpace(cfg.Listen)
	}
	if addr == "" {
		addr = ":8091"
	}
	server := &http.Server{Addr: addr, Handler: service.handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("pokemodelhost: %s (%s) listening on %s", cfg.HostID, cfg.Compute, addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
