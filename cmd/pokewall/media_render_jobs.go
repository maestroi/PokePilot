package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

const (
	mediaRenderJobStateFile = "media-render-jobs.json"
	mediaRenderLease        = 90 * time.Second
)

type mediaRenderJobState struct {
	Jobs map[string]farm.MediaRenderJob `json:"jobs"`
}

type mediaRenderJobController struct {
	wall *Wall

	mu      sync.Mutex
	loaded  bool
	loadErr error
	state   mediaRenderJobState
}

var mediaRenderJobControllers sync.Map // *Wall -> *mediaRenderJobController

func mediaRenderJobsFor(w *Wall) *mediaRenderJobController {
	if existing, ok := mediaRenderJobControllers.Load(w); ok {
		c := existing.(*mediaRenderJobController)
		c.ensureLoaded()
		return c
	}
	c := &mediaRenderJobController{
		wall:  w,
		state: mediaRenderJobState{Jobs: map[string]farm.MediaRenderJob{}},
	}
	actual, _ := mediaRenderJobControllers.LoadOrStore(w, c)
	out := actual.(*mediaRenderJobController)
	out.ensureLoaded()
	return out
}

func (c *mediaRenderJobController) ensureLoaded() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded {
		return
	}
	c.loaded = true
	c.loadErr = c.loadLocked()
	if c.state.Jobs == nil {
		c.state.Jobs = map[string]farm.MediaRenderJob{}
	}
}

func (c *mediaRenderJobController) ensure(req farm.MediaRenderJobCreateRequest) (farm.MediaRenderJob, bool, error) {
	job, err := farm.NewMediaRenderJob(req, time.Now())
	if err != nil {
		return farm.MediaRenderJob{}, false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadErr != nil {
		return farm.MediaRenderJob{}, false, c.loadErr
	}
	if existing, ok := c.state.Jobs[job.ID]; ok {
		if existing.Identity != job.Identity {
			return farm.MediaRenderJob{}, false, fmt.Errorf("media render job id collision for %s", job.ID)
		}
		return existing, false, nil
	}
	c.state.Jobs[job.ID] = job
	if err := c.persistLocked(); err != nil {
		delete(c.state.Jobs, job.ID)
		return farm.MediaRenderJob{}, false, err
	}
	return job, true, nil
}

type mediaRenderJobList struct {
	Jobs   []farm.MediaRenderJob `json:"jobs"`
	Total  int                   `json:"total"`
	States map[string]int        `json:"states"`
}

func (c *mediaRenderJobController) get(id string) (farm.MediaRenderJob, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	job, ok := c.state.Jobs[strings.TrimSpace(id)]
	return job, ok
}

func (c *mediaRenderJobController) list(runID, state string, limit int) mediaRenderJobList {
	c.mu.Lock()
	defer c.mu.Unlock()

	runID = strings.TrimSpace(runID)
	state = strings.TrimSpace(state)
	jobs := make([]farm.MediaRenderJob, 0, len(c.state.Jobs))
	counts := map[string]int{}
	for _, job := range c.state.Jobs {
		if runID != "" && job.RunID != runID {
			continue
		}
		if state != "" && job.State != state {
			continue
		}
		jobs = append(jobs, job)
		counts[job.State]++
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].UpdatedAt == jobs[j].UpdatedAt {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].UpdatedAt > jobs[j].UpdatedAt
	})
	total := len(jobs)
	if limit > 0 && len(jobs) > limit {
		jobs = jobs[:limit]
	}
	return mediaRenderJobList{Jobs: jobs, Total: total, States: counts}
}

func (c *mediaRenderJobController) claimable() []farm.MediaRenderJob {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	out := make([]farm.MediaRenderJob, 0)
	for _, job := range c.state.Jobs {
		if job.Claimable(now) {
			out = append(out, job)
		}
	}
	return out
}

func (c *mediaRenderJobController) claim(id, worker string) (farm.MediaRenderJob, bool, error) {
	id = strings.TrimSpace(id)
	worker = strings.TrimSpace(worker)
	if worker == "" {
		return farm.MediaRenderJob{}, false, errors.New("worker_id is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadErr != nil {
		return farm.MediaRenderJob{}, false, c.loadErr
	}
	job, ok := c.state.Jobs[id]
	if !ok {
		return farm.MediaRenderJob{}, false, os.ErrNotExist
	}
	now := time.Now()
	if !job.Claimable(now) {
		return job, false, nil
	}
	if job.Active() {
		job.RetryCount++
		job.LastError = "recovered after expired worker lease"
	}
	job.State = farm.MediaRenderJobPreparing
	job.Stage = farm.MediaRenderJobPreparing
	job.WorkerID = worker
	job.LeaseExpiresAt = now.Add(mediaRenderLease).UnixMilli()
	job.UpdatedAt = now.UnixMilli()
	if job.StartedAt == 0 {
		job.StartedAt = now.UnixMilli()
	}
	job.FinishedAt = 0
	c.state.Jobs[id] = job
	if err := c.persistLocked(); err != nil {
		return farm.MediaRenderJob{}, false, err
	}
	return job, true, nil
}

func (c *mediaRenderJobController) progress(id string, req farm.MediaRenderJobProgressRequest) (farm.MediaRenderJob, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadErr != nil {
		return farm.MediaRenderJob{}, c.loadErr
	}
	job, ok := c.state.Jobs[strings.TrimSpace(id)]
	if !ok {
		return farm.MediaRenderJob{}, os.ErrNotExist
	}
	if strings.TrimSpace(req.WorkerID) == "" || req.WorkerID != job.WorkerID {
		return job, errors.New("media render job is owned by another worker")
	}
	if !job.Active() {
		return job, fmt.Errorf("media render job is not active (state %s)", job.State)
	}
	if req.State != "" {
		switch req.State {
		case farm.MediaRenderJobPreparing, farm.MediaRenderJobRendering, farm.MediaRenderJobAssembling, farm.MediaRenderJobUploading:
			job.State = req.State
		default:
			return job, fmt.Errorf("invalid active media render state %q", req.State)
		}
	}
	if req.Stage != "" {
		job.Stage = strings.TrimSpace(req.Stage)
	} else {
		job.Stage = job.State
	}
	if req.SegmentsTotal != nil {
		job.SegmentsTotal = *req.SegmentsTotal
	}
	if req.SegmentsDone != nil {
		job.SegmentsDone = *req.SegmentsDone
	}
	if job.SegmentsTotal > 0 && job.SegmentsDone > job.SegmentsTotal {
		job.SegmentsDone = job.SegmentsTotal
	}
	now := time.Now()
	job.LeaseExpiresAt = now.Add(mediaRenderLease).UnixMilli()
	job.UpdatedAt = now.UnixMilli()
	c.state.Jobs[job.ID] = job
	if err := c.persistLocked(); err != nil {
		return farm.MediaRenderJob{}, err
	}
	return job, nil
}

func (c *mediaRenderJobController) finish(id string, req farm.MediaRenderJobFinishRequest) (farm.MediaRenderJob, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadErr != nil {
		return farm.MediaRenderJob{}, c.loadErr
	}
	job, ok := c.state.Jobs[strings.TrimSpace(id)]
	if !ok {
		return farm.MediaRenderJob{}, os.ErrNotExist
	}
	if strings.TrimSpace(req.WorkerID) == "" || req.WorkerID != job.WorkerID {
		return job, errors.New("media render job is owned by another worker")
	}
	switch req.State {
	case farm.MediaRenderJobReady, farm.MediaRenderJobFailed:
	default:
		return job, fmt.Errorf("invalid terminal media render state %q", req.State)
	}
	now := time.Now().UnixMilli()
	job.State = req.State
	job.Stage = strings.TrimSpace(req.Stage)
	if job.Stage == "" {
		job.Stage = req.State
	}
	job.LastError = strings.TrimSpace(req.LastError)
	job.ResultSize = req.ResultSize
	job.WorkerID = ""
	job.LeaseExpiresAt = 0
	job.UpdatedAt = now
	job.FinishedAt = now
	c.state.Jobs[job.ID] = job
	if err := c.persistLocked(); err != nil {
		return farm.MediaRenderJob{}, err
	}
	return job, nil
}

func (c *mediaRenderJobController) reconcileReady(id string, size int64) (farm.MediaRenderJob, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadErr != nil {
		return farm.MediaRenderJob{}, c.loadErr
	}
	job, ok := c.state.Jobs[strings.TrimSpace(id)]
	if !ok {
		return farm.MediaRenderJob{}, os.ErrNotExist
	}
	now := time.Now().UnixMilli()
	job.State = farm.MediaRenderJobReady
	job.Stage = farm.MediaRenderJobReady
	job.ResultSize = size
	job.LastError = ""
	job.WorkerID = ""
	job.LeaseExpiresAt = 0
	job.UpdatedAt = now
	job.FinishedAt = now
	c.state.Jobs[job.ID] = job
	if err := c.persistLocked(); err != nil {
		return farm.MediaRenderJob{}, err
	}
	return job, nil
}

func (c *mediaRenderJobController) cancel(id string) (farm.MediaRenderJob, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadErr != nil {
		return farm.MediaRenderJob{}, c.loadErr
	}
	job, ok := c.state.Jobs[strings.TrimSpace(id)]
	if !ok {
		return farm.MediaRenderJob{}, os.ErrNotExist
	}
	if job.State == farm.MediaRenderJobReady {
		return job, errors.New("ready media render job cannot be cancelled")
	}
	now := time.Now().UnixMilli()
	job.State = farm.MediaRenderJobCancelled
	job.Stage = farm.MediaRenderJobCancelled
	job.WorkerID = ""
	job.LeaseExpiresAt = 0
	job.UpdatedAt = now
	job.FinishedAt = now
	c.state.Jobs[job.ID] = job
	if err := c.persistLocked(); err != nil {
		return farm.MediaRenderJob{}, err
	}
	return job, nil
}

func (c *mediaRenderJobController) retry(id string) (farm.MediaRenderJob, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadErr != nil {
		return farm.MediaRenderJob{}, c.loadErr
	}
	job, ok := c.state.Jobs[strings.TrimSpace(id)]
	if !ok {
		return farm.MediaRenderJob{}, os.ErrNotExist
	}
	if job.State != farm.MediaRenderJobFailed && job.State != farm.MediaRenderJobCancelled {
		return job, fmt.Errorf("media render job cannot retry from state %s", job.State)
	}
	now := time.Now().UnixMilli()
	job.State = farm.MediaRenderJobQueued
	job.Stage = farm.MediaRenderJobQueued
	job.WorkerID = ""
	job.LeaseExpiresAt = 0
	job.LastError = ""
	job.ResultSize = 0
	job.UpdatedAt = now
	job.FinishedAt = 0
	c.state.Jobs[job.ID] = job
	if err := c.persistLocked(); err != nil {
		return farm.MediaRenderJob{}, err
	}
	return job, nil
}

func (c *mediaRenderJobController) loadLocked() error {
	if cp := controlPlaneFor(c.wall); cp != nil {
		if _, err := cp.db.Exec(`CREATE TABLE IF NOT EXISTS media_render_job_state (
			id SMALLINT PRIMARY KEY CHECK (id = 1),
			state_json JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
			return fmt.Errorf("create media render job state: %w", err)
		}
		var raw []byte
		err := cp.db.QueryRow(`SELECT state_json FROM media_render_job_state WHERE id=1`).Scan(&raw)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("load media render job state: %w", err)
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &c.state); err != nil {
				return fmt.Errorf("decode media render job state: %w", err)
			}
		}
		return nil
	}
	path := c.localStatePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read media render job state: %w", err)
	}
	if err := json.Unmarshal(data, &c.state); err != nil {
		return fmt.Errorf("decode media render job state: %w", err)
	}
	return nil
}

func (c *mediaRenderJobController) persistLocked() error {
	raw, err := json.Marshal(c.state)
	if err != nil {
		return err
	}
	if cp := controlPlaneFor(c.wall); cp != nil {
		if _, err := cp.db.Exec(`CREATE TABLE IF NOT EXISTS media_render_job_state (
			id SMALLINT PRIMARY KEY CHECK (id = 1),
			state_json JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
			return err
		}
		_, err = cp.db.Exec(`INSERT INTO media_render_job_state(id,state_json,updated_at)
			VALUES(1,$1::jsonb,NOW())
			ON CONFLICT(id) DO UPDATE SET state_json=EXCLUDED.state_json,updated_at=NOW()`, string(raw))
		return err
	}
	path := c.localStatePath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeAtomic(path, raw, 0o644)
}

func (c *mediaRenderJobController) localStatePath() string {
	if c.wall.statePath != "" {
		return c.wall.statePath + ".media-renders.json"
	}
	if c.wall.dumpsDir != "" {
		return filepath.Join(c.wall.dumpsDir, mediaRenderJobStateFile)
	}
	return ""
}

func mediaRenderJobHTTPHandler(w *Wall, next http.Handler) http.Handler {
	controller := mediaRenderJobsFor(w)
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/media/render-jobs", func(res http.ResponseWriter, req *http.Request) {
		req.Body = http.MaxBytesReader(res, req.Body, maxSmallControlBody)
		var body farm.MediaRenderJobCreateRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid media render job: " + err.Error()})
			return
		}
		job, created, err := controller.ensure(body)
		if err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		writeJSON(res, status, job)
	})
	mux.HandleFunc("GET /v1/media/render-jobs", func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("claimable") == "1" {
			writeJSON(res, http.StatusOK, map[string]any{"jobs": controller.claimable()})
			return
		}
		limit := 50
		if raw := strings.TrimSpace(req.URL.Query().Get("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 200 {
				writeJSON(res, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 200"})
				return
			}
			limit = parsed
		}
		writeJSON(res, http.StatusOK, controller.list(req.URL.Query().Get("run_id"), req.URL.Query().Get("state"), limit))
	})
	mux.HandleFunc("GET /v1/media/render-jobs/{id}", func(res http.ResponseWriter, req *http.Request) {
		job, ok := controller.get(req.PathValue("id"))
		if !ok {
			writeJSON(res, http.StatusNotFound, map[string]string{"error": "media render job not found"})
			return
		}
		writeJSON(res, http.StatusOK, job)
	})
	mux.HandleFunc("POST /v1/media/render-jobs/{id}/claim", func(res http.ResponseWriter, req *http.Request) {
		var body farm.MediaRenderJobClaimRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid media render claim: " + err.Error()})
			return
		}
		job, claimed, err := controller.claim(req.PathValue("id"), body.WorkerID)
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(res, http.StatusNotFound, map[string]string{"error": "media render job not found"})
			return
		}
		if err != nil {
			writeJSON(res, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		if !claimed {
			writeJSON(res, http.StatusConflict, job)
			return
		}
		writeJSON(res, http.StatusOK, job)
	})
	mux.HandleFunc("POST /v1/media/render-jobs/{id}/heartbeat", func(res http.ResponseWriter, req *http.Request) {
		var body farm.MediaRenderJobProgressRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid media render progress: " + err.Error()})
			return
		}
		job, err := controller.progress(req.PathValue("id"), body)
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(res, http.StatusNotFound, map[string]string{"error": "media render job not found"})
			return
		}
		if err != nil {
			writeJSON(res, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusOK, job)
	})
	mux.HandleFunc("POST /v1/media/render-jobs/{id}/finish", func(res http.ResponseWriter, req *http.Request) {
		var body farm.MediaRenderJobFinishRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid media render finish: " + err.Error()})
			return
		}
		job, err := controller.finish(req.PathValue("id"), body)
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(res, http.StatusNotFound, map[string]string{"error": "media render job not found"})
			return
		}
		if err != nil {
			writeJSON(res, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusOK, job)
	})
	mux.HandleFunc("POST /v1/media/render-jobs/{id}/reconcile-ready", func(res http.ResponseWriter, req *http.Request) {
		var body struct {
			Size int64 `json:"size"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid media render reconciliation: " + err.Error()})
			return
		}
		job, err := controller.reconcileReady(req.PathValue("id"), body.Size)
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(res, http.StatusNotFound, map[string]string{"error": "media render job not found"})
			return
		}
		if err != nil {
			writeJSON(res, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusOK, job)
	})
	mux.HandleFunc("POST /v1/media/render-jobs/{id}/cancel", func(res http.ResponseWriter, req *http.Request) {
		job, err := controller.cancel(req.PathValue("id"))
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(res, http.StatusNotFound, map[string]string{"error": "media render job not found"})
			return
		}
		if err != nil {
			writeJSON(res, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusOK, job)
	})
	mux.HandleFunc("POST /v1/media/render-jobs/{id}/retry", func(res http.ResponseWriter, req *http.Request) {
		job, err := controller.retry(req.PathValue("id"))
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(res, http.StatusNotFound, map[string]string{"error": "media render job not found"})
			return
		}
		if err != nil {
			writeJSON(res, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusOK, job)
	})
	mux.Handle("/", next)
	return mux
}
