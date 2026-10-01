// Command pokereplay is the read/render sidecar for durable PokePilot run
// artifacts. It never owns run metadata: pokewall is the catalog and S3 is
// the blob store. This process only resolves a run's artifact references,
// renders deterministic .gbrun recordings through GomeBoy, caches derived
// MP4s back into S3, and streams artifact/video bytes to the private UI.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/media/compositor"
	mediasegment "github.com/maestroi/pokepilot/media/segment"
	redstarter "github.com/maestroi/pokepilot/red/starter"
)

const (
	serverReadHeaderTimeout = 5 * time.Second
	serverIdleTimeout       = 60 * time.Second
	serverShutdownTimeout   = 10 * time.Second
	wallTimeout             = 30 * time.Second
	renderTimeout           = 2 * time.Hour // per attempt segment, not per run
	maxWallResponseBytes    = 4 << 20
)

type artifactRef struct {
	Name       string `json:"name"`
	MediaType  string `json:"media_type,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	Store      string `json:"store,omitempty"`
	Bucket     string `json:"bucket,omitempty"`
	ObjectKey  string `json:"object_key,omitempty"`
	Size       int64  `json:"size,omitempty"`
	Inline     bool   `json:"inline"`
	Replayable bool   `json:"replayable,omitempty"`
}

type artifactList struct {
	RunID     string        `json:"run_id"`
	Attempt   int           `json:"attempt"`
	Artifacts []artifactRef `json:"artifacts"`
}

type replayRecording struct {
	Attempt  int
	Artifact artifactRef
	Timeline artifactRef
}

type replayStatus struct {
	RunID          string `json:"run_id"`
	JobID          string `json:"job_id,omitempty"`
	State          string `json:"state"`
	ObjectKey      string `json:"object_key,omitempty"`
	Size           int64  `json:"size,omitempty"`
	Error          string `json:"error,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	JobState       string `json:"job_state,omitempty"`
	Stage          string `json:"stage,omitempty"`
	RetryCount     int    `json:"retry_count,omitempty"`
	FailureClass   string `json:"failure_class,omitempty"`
	LegacyIdentity bool   `json:"legacy_identity,omitempty"`
	// Segments/SegmentsDone report per-attempt progress while generating.
	Segments     int `json:"segments,omitempty"`
	SegmentsDone int `json:"segments_done,omitempty"`
	// MissingAttempts lists attempts with no replayable recording (lost
	// worker, failed upload). When non-empty the replay covers only part of
	// the run and Partial is true.
	Partial         bool  `json:"partial,omitempty"`
	MissingAttempts []int `json:"missing_attempts,omitempty"`
}

type replayIdentity struct {
	ROMSHA256 string
	Metadata  map[string]string
}

type replayServer struct {
	wallBase         string
	romPath          string
	romLibrary       *replayROMLibrary
	streamBinary     string
	vaapi            bool
	vaapiReason      string
	ffmpegVAAPI      string
	store            *artifactstore.S3
	wallHTTP         *http.Client
	compositor       replayCompositor
	semanticRenderer *compositor.SemanticRenderer

	mu   sync.Mutex
	jobs map[string]replayStatus // cache object key -> latest local render state

	// rendering counts render goroutines in flight. /healthz exposes it so the
	// host updater only replaces this container when no video is mid-encode.
	rendering atomic.Int64

	capacity         replayCapacityConfig
	jobSlots         chan struct{}
	scratchFree      func(string) (uint64, error)
	resourceMu       sync.Mutex
	deferredJobs     map[string]replayDeferredJob
	activeJobs       map[string]time.Time
	activeCancels    map[string]context.CancelFunc
	encoderProcesses atomic.Int64
	renderFailures   atomic.Uint64
	renderRetries    atomic.Uint64
	renderCompleted  atomic.Uint64
	renderBytes      atomic.Uint64
	renderNanos      atomic.Uint64

	liveMu       sync.Mutex
	liveSessions map[string]*liveBroadcastSession

	parseRecording func([]byte) (replayIdentity, error)
	deriveROM      func([]byte, map[string]string, string) ([]byte, error)
}

func newReplayServer(wallBase, romPath, streamBinary string, store *artifactstore.S3) *replayServer {
	semanticRenderer, err := compositor.NewPublicSemanticRenderer()
	if err != nil {
		log.Printf("pokereplay: headless semantic renderer unavailable: %v", err)
	}
	capacity := replayCapacityFromEnv()
	if err := os.MkdirAll(capacity.ScratchDir, 0o755); err != nil {
		log.Printf("pokereplay: create scratch directory %s: %v", capacity.ScratchDir, err)
	}
	return &replayServer{
		wallBase:         strings.TrimRight(wallBase, "/"),
		romPath:          romPath,
		streamBinary:     streamBinary,
		store:            store,
		wallHTTP:         &http.Client{Timeout: wallTimeout},
		compositor:       compositor.NewFFmpeg("ffmpeg", nil),
		semanticRenderer: semanticRenderer,
		jobs:             make(map[string]replayStatus),
		capacity:         capacity,
		jobSlots:         make(chan struct{}, capacity.MaxJobs),
		scratchFree:      freeDiskBytes,
		deferredJobs:     make(map[string]replayDeferredJob),
		activeJobs:       make(map[string]time.Time),
		activeCancels:    make(map[string]context.CancelFunc),
		liveSessions:     make(map[string]*liveBroadcastSession),
	}
}

func (s *replayServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		health := map[string]any{
			"status":            "ok",
			"s3_configured":     s.store != nil,
			"encoder":           s.encoderName(),
			"vaapi":             s.vaapi,
			"vaapi_reason":      s.vaapiReason,
			"renderer":          broadcastRendererVersion,
			"semantic_renderer": compositor.PublicSemanticRendererVersion(),
			"live_fps":          liveBroadcastFPS,
			"active_renders":    s.rendering.Load(),
		}
		for key, value := range s.replayResourceHealth() {
			health[key] = value
		}
		s.liveMu.Lock()
		health["live_sessions"] = len(s.liveSessions)
		s.liveMu.Unlock()
		writeJSON(w, http.StatusOK, health)
	})
	mux.HandleFunc("GET /v1/runs/{id}/replay/status", s.handleReplayStatus)
	mux.HandleFunc("POST /v1/runs/{id}/replay/render", s.handleReplayRender)
	mux.HandleFunc("POST /v1/runs/{id}/replay/cancel", s.handleReplayCancel)
	mux.HandleFunc("GET /v1/runs/{id}/replay/video", s.handleReplayVideo)
	mux.HandleFunc("GET /v1/runs/{id}/replay/semantic", s.handleReplaySemantic)
	mux.HandleFunc("GET /v1/runs/{id}/highlights/status", s.handleHighlightStatus)
	mux.HandleFunc("POST /v1/runs/{id}/highlights/render", s.handleHighlightRender)
	mux.HandleFunc("GET /v1/runs/{id}/highlights/video", s.handleHighlightVideo)
	mux.HandleFunc("GET /v1/runs/{id}/highlights/manifest", s.handleHighlightManifest)
	mux.HandleFunc("GET /v1/runs/{id}/live/status", s.handleLiveStatus)
	mux.HandleFunc("GET /v1/runs/{id}/live/stream.mjpeg", s.handleLiveStream)
	mux.HandleFunc("GET /v1/runs/{id}/artifacts/{name}/content", s.handleArtifactContent)
	mux.HandleFunc("DELETE /v1/runs/{id}/artifacts", s.handleArtifactDelete)
	return mux
}

func (s *replayServer) handleReplayStatus(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	mode, err := parseReplayMode(r.URL.Query().Get("mode"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	recordings, missing, err := s.recordingsWithGaps(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	status := s.replayStatus(r.Context(), runID, recordings, mode)
	status.Partial, status.MissingAttempts = len(missing) > 0, missing
	writeJSON(w, http.StatusOK, status)
}

func (s *replayServer) handleReplayRender(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	mode, err := parseReplayMode(r.URL.Query().Get("mode"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	recordings, err := s.recordings(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, replayStatus{RunID: runID, State: "disabled", Error: "S3 artifact storage is not configured for the replay service"})
		return
	}
	status := s.replayStatus(r.Context(), runID, recordings, mode)
	if status.State == "ready" && !status.LegacyIdentity {
		writeJSON(w, http.StatusOK, status)
		return
	}
	cacheKey := s.replayCacheKeyForMode(runID, recordings, mode)
	job, jobErr := s.ensureRenderJob(r.Context(), runID, recordings, mode, cacheKey)
	if jobErr == nil {
		if job.State == farm.MediaRenderJobFailed {
			if !job.Retryable() {
				writeJSON(w, http.StatusConflict, replayStatusFromMediaJob(job))
				return
			}
			retried, retryErr := s.retryRenderJob(r.Context(), job.ID)
			if retryErr != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": retryErr.Error()})
				return
			}
			s.renderRetries.Add(1)
			job = retried
		} else if job.State == farm.MediaRenderJobCancelled {
			retried, retryErr := s.retryRenderJob(r.Context(), job.ID)
			if retryErr != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": retryErr.Error()})
				return
			}
			s.renderRetries.Add(1)
			job = retried
		} else if job.Active() {
			writeJSON(w, http.StatusAccepted, replayStatusFromMediaJob(job))
			return
		}

		release, reason, detail := s.tryAdmitRenderJob(job.ID)
		if release == nil {
			status = replayStatusFromMediaJob(job)
			status.Stage = "queued_" + reason
			status.LastError = detail
			writeJSON(w, http.StatusAccepted, status)
			return
		}
		claimed, ok, claimErr := s.claimRenderJob(r.Context(), job.ID)
		if claimErr != nil {
			release()
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": claimErr.Error()})
			return
		}
		if !ok {
			release()
			writeJSON(w, http.StatusAccepted, replayStatusFromMediaJob(claimed))
			return
		}
		status = replayStatusFromMediaJob(claimed)
		go func() {
			defer release()
			s.render(claimed.ID, runID, recordings, cacheKey, mode)
		}()
		writeJSON(w, http.StatusAccepted, status)
		return
	}
	if !errors.Is(jobErr, errMediaRenderJobAPIUnavailable) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": jobErr.Error()})
		return
	}

	// Compatibility with an older/local wall that does not expose durable media
	// jobs yet. Production uses the wall-backed job authority above.
	s.mu.Lock()
	if current, ok := s.jobs[cacheKey]; ok && current.State == "generating" && current.Stage != "queued_max_jobs" && current.Stage != "queued_low_scratch" && current.Stage != "queued_scratch_unavailable" {
		s.mu.Unlock()
		writeJSON(w, http.StatusAccepted, current)
		return
	}
	s.mu.Unlock()

	release, reason, detail := s.tryAdmitRenderJob(cacheKey)
	if release == nil {
		status = replayStatus{RunID: runID, State: "generating", ObjectKey: cacheKey, Stage: "queued_" + reason, LastError: detail}
		s.setJob(cacheKey, status)
		writeJSON(w, http.StatusAccepted, status)
		return
	}
	status = replayStatus{RunID: runID, State: "generating", ObjectKey: cacheKey, Stage: farm.MediaRenderJobPreparing}
	s.setJob(cacheKey, status)
	go func() {
		defer release()
		s.render("", runID, recordings, cacheKey, mode)
	}()
	writeJSON(w, http.StatusAccepted, status)
}

func (s *replayServer) handleReplayCancel(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	mode, err := parseReplayMode(r.URL.Query().Get("mode"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	recordings, err := s.recordings(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	cacheKey := s.replayCacheKeyForMode(runID, recordings, mode)
	jobID := farm.MediaRenderJobID(cacheKey)
	job, err := s.cancelRenderJob(r.Context(), jobID)
	if err == nil {
		s.clearDeferredRenderJob(job.ID)
		s.cancelActiveRender(job.ID)
		writeJSON(w, http.StatusOK, replayStatusFromMediaJob(job))
		return
	}
	if !errors.Is(err, errMediaRenderJobAPIUnavailable) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	cancelled := s.cancelActiveRender(cacheKey)
	s.clearDeferredRenderJob(cacheKey)
	status := replayStatus{
		RunID: runID, State: "error", ObjectKey: cacheKey, Error: "render cancelled",
		JobState: farm.MediaRenderJobCancelled, Stage: farm.MediaRenderJobCancelled,
		FailureClass: farm.MediaRenderFailureCancelled,
	}
	if !cancelled {
		status.LastError = "render was queued or not active"
	}
	s.setJob(cacheKey, status)
	writeJSON(w, http.StatusOK, status)
}

func (s *replayServer) handleReplayVideo(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	mode, err := parseReplayMode(r.URL.Query().Get("mode"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	recordings, err := s.recordings(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	status := s.replayStatus(r.Context(), runID, recordings, mode)
	if status.State != "ready" {
		writeJSON(w, http.StatusConflict, status)
		return
	}
	obj, err := s.store.GetObject(r.Context(), status.ObjectKey, r.Header.Get("Range"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer obj.Body.Close()
	copyObjectResponse(w, obj, "video/mp4", "")
}

// artifactAttemptQuery parses an optional ?attempt= artifact selector. Zero
// means the run's latest artifact generation, which is the historical default.
func artifactAttemptQuery(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	attempt, err := strconv.Atoi(raw)
	if err != nil || attempt < 1 {
		return 0, fmt.Errorf("invalid attempt %q: want a positive integer", raw)
	}
	return attempt, nil
}

func (s *replayServer) handleArtifactContent(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	name := r.PathValue("name")
	// ?attempt= scopes the read to one attempt's artifact generation, so
	// evidence captured for an earlier failing attempt of an endless run is
	// still downloadable after the run moved on. Zero keeps the historical
	// latest-attempt behaviour.
	attempt, err := artifactAttemptQuery(r.URL.Query().Get("attempt"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	list, err := s.artifactsAttempt(r.Context(), runID, attempt)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	artifact, ok := findArtifact(list.Artifacts, name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "artifact not found"})
		return
	}
	if artifact.Store == "" {
		s.proxyInlineArtifact(w, r, runID, name, attempt)
		return
	}
	if artifact.Store != "s3" {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "unsupported artifact store " + artifact.Store})
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "S3 artifact storage is not configured for the replay service"})
		return
	}
	if artifact.Bucket != "" && artifact.Bucket != s.store.Bucket() {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "artifact bucket does not match configured replay bucket"})
		return
	}
	obj, err := s.store.GetObject(r.Context(), artifact.ObjectKey, r.Header.Get("Range"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer obj.Body.Close()
	copyObjectResponse(w, obj, artifact.MediaType, artifact.Name)
}

func (s *replayServer) recording(ctx context.Context, runID string) (artifactRef, error) {
	list, err := s.artifacts(ctx, runID)
	if err != nil {
		return artifactRef{}, err
	}
	for _, artifact := range list.Artifacts {
		if artifact.Name == "run.gbrun" && artifact.Replayable {
			return artifact, nil
		}
	}
	return artifactRef{}, errRecordingNotFound
}

func (s *replayServer) recordings(ctx context.Context, runID string) ([]replayRecording, error) {
	recordings, _, err := s.recordingsWithGaps(ctx, runID)
	return recordings, err
}

// recordingsWithGaps is recordings plus the attempts that had no replayable
// recording, so callers can say the replay is partial instead of hiding it.
func (s *replayServer) recordingsWithGaps(ctx context.Context, runID string) ([]replayRecording, []int, error) {
	latest, err := s.artifacts(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	latestAttempt := latest.Attempt
	if latestAttempt < 1 {
		latestAttempt = 1
	}
	if latestAttempt == 1 {
		artifact, ok := findArtifact(latest.Artifacts, "run.gbrun")
		if !ok || !artifact.Replayable {
			return nil, nil, errRecordingNotFound
		}
		timeline, _ := findArtifact(latest.Artifacts, farm.MediaTimelineArtifactName)
		return []replayRecording{{Attempt: 1, Artifact: artifact, Timeline: timeline}}, nil, nil
	}

	recordings := make([]replayRecording, 0, latestAttempt)
	var missing []int
	for attempt := 1; attempt <= latestAttempt; attempt++ {
		list, err := s.artifactsAttempt(ctx, runID, attempt)
		if errors.Is(err, errRunNotFound) {
			missing = append(missing, attempt)
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		artifact, ok := findArtifact(list.Artifacts, "run.gbrun")
		if !ok || !artifact.Replayable {
			missing = append(missing, attempt)
			continue
		}
		timeline, _ := findArtifact(list.Artifacts, farm.MediaTimelineArtifactName)
		recordings = append(recordings, replayRecording{Attempt: attempt, Artifact: artifact, Timeline: timeline})
	}
	if len(recordings) == 0 {
		return nil, nil, errRecordingNotFound
	}
	return recordings, missing, nil
}

func (s *replayServer) recordingsForAttempts(ctx context.Context, runID string, attempts []int) ([]replayRecording, error) {
	if len(attempts) == 0 {
		return nil, errors.New("media render job has no persisted source attempts")
	}
	recordings := make([]replayRecording, 0, len(attempts))
	seen := make(map[int]struct{}, len(attempts))
	for _, attempt := range attempts {
		if attempt < 1 {
			return nil, fmt.Errorf("invalid persisted replay attempt %d", attempt)
		}
		if _, ok := seen[attempt]; ok {
			return nil, fmt.Errorf("duplicate persisted replay attempt %d", attempt)
		}
		seen[attempt] = struct{}{}

		list, err := s.artifactsAttempt(ctx, runID, attempt)
		if err != nil {
			return nil, fmt.Errorf("attempt %d: %w", attempt, err)
		}
		artifact, ok := findArtifact(list.Artifacts, "run.gbrun")
		if !ok || !artifact.Replayable {
			return nil, fmt.Errorf("attempt %d: %w", attempt, errRecordingNotFound)
		}
		timeline, _ := findArtifact(list.Artifacts, farm.MediaTimelineArtifactName)
		recordings = append(recordings, replayRecording{Attempt: attempt, Artifact: artifact, Timeline: timeline})
	}
	return recordings, nil
}

func (s *replayServer) artifacts(ctx context.Context, runID string) (artifactList, error) {
	return s.artifactsAttempt(ctx, runID, 0)
}

func (s *replayServer) artifactsAttempt(ctx context.Context, runID string, attempt int) (artifactList, error) {
	var out artifactList
	if strings.TrimSpace(runID) == "" {
		return out, errRunNotFound
	}
	endpoint := s.wallBase + "/v1/runs/" + url.PathEscape(runID) + "/artifacts"
	if attempt > 0 {
		endpoint += "?attempt=" + strconv.Itoa(attempt)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return out, err
	}
	res, err := s.wallHTTP.Do(req)
	if err != nil {
		return out, fmt.Errorf("pokewall unavailable: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxWallResponseBytes+1))
	if err != nil {
		return out, err
	}
	if len(data) > maxWallResponseBytes {
		return out, fmt.Errorf("pokewall artifact response exceeds %d bytes", maxWallResponseBytes)
	}
	if res.StatusCode == http.StatusNotFound {
		return out, errRunNotFound
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return out, fmt.Errorf("pokewall returned %s: %s", res.Status, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("decode pokewall artifacts: %w", err)
	}
	return out, nil
}

func (s *replayServer) replayStatus(ctx context.Context, runID string, recordings []replayRecording, mode replayMode) replayStatus {
	if s.store == nil {
		return replayStatus{RunID: runID, State: "disabled", Error: "S3 artifact storage is not configured for the replay service"}
	}
	cacheKey := s.replayCacheKeyForMode(runID, recordings, mode)
	jobID := farm.MediaRenderJobID(cacheKey)
	if obj, err := s.store.HeadObject(ctx, cacheKey); err == nil {
		if job, ok, jobErr := s.getRenderJob(ctx, jobID); jobErr == nil && ok && job.State != farm.MediaRenderJobReady {
			s.reconcileRenderJobReady(ctx, jobID, obj.Size)
		}
		return replayStatus{RunID: runID, JobID: jobID, State: "ready", ObjectKey: cacheKey, Size: obj.Size, JobState: farm.MediaRenderJobReady, Stage: farm.MediaRenderJobReady}
	} else if !artifactstore.IsNotFound(err) {
		return replayStatus{RunID: runID, JobID: jobID, State: "error", ObjectKey: cacheKey, Error: err.Error()}
	}
	if job, ok, err := s.getRenderJob(ctx, jobID); err == nil && ok {
		return replayStatusFromMediaJob(job)
	}

	s.mu.Lock()
	if status, ok := s.jobs[cacheKey]; ok {
		if status.JobID == "" {
			status.JobID = jobID
		}
		s.mu.Unlock()
		return status
	}
	s.mu.Unlock()

	// Artifacts from before media identity-v1 remain readable. They are marked
	// explicitly so a render request can migrate to the canonical identity
	// instead of silently treating the legacy key as equivalent.
	legacyKey := s.legacyReplayCacheKeyForMode(runID, recordings, mode)
	if legacyKey != cacheKey {
		legacyJobID := farm.MediaRenderJobID(legacyKey)
		if obj, err := s.store.HeadObject(ctx, legacyKey); err == nil {
			return replayStatus{
				RunID: runID, JobID: legacyJobID, State: "ready", ObjectKey: legacyKey, Size: obj.Size,
				JobState: farm.MediaRenderJobReady, Stage: farm.MediaRenderJobReady, LegacyIdentity: true,
			}
		} else if !artifactstore.IsNotFound(err) {
			return replayStatus{RunID: runID, JobID: legacyJobID, State: "error", ObjectKey: legacyKey, Error: err.Error(), LegacyIdentity: true}
		}
	}
	return replayStatus{RunID: runID, JobID: jobID, State: "missing", ObjectKey: cacheKey}
}

func (s *replayServer) render(jobID, runID string, recordings []replayRecording, cacheKey string, mode replayMode) {
	s.rendering.Add(1)
	defer s.rendering.Add(-1)
	ctx, cancel := withReplayTimeout(context.Background(), s.capacity.JobTimeout)
	defer cancel()
	var cancelledByControl atomic.Bool
	cancelForControl := func() {
		cancelledByControl.Store(true)
		cancel()
	}
	controlID := firstNonEmpty(jobID, cacheKey)
	s.registerRenderCancel(controlID, cancelForControl)
	defer s.unregisterRenderCancel(controlID)
	stopLease := s.keepRenderJobLease(ctx, jobID, cancelForControl)
	defer stopLease()
	started := time.Now()
	encoder := s.encoderName()
	setError := func(err error) {
		if cancelledByControl.Load() {
			log.Printf("pokereplay render cancelled run=%s key=%s dur=%s", runID, cacheKey, time.Since(started).Round(time.Millisecond))
			s.setJob(cacheKey, replayStatus{
				RunID: runID, JobID: jobID, State: "error", ObjectKey: cacheKey, Error: "render cancelled",
				JobState: farm.MediaRenderJobCancelled, Stage: farm.MediaRenderJobCancelled, FailureClass: farm.MediaRenderFailureCancelled,
			})
			return
		}
		failureClass := classifyRenderFailure(err, ctx.Err())
		s.renderFailures.Add(1)
		log.Printf("pokereplay render fail run=%s key=%s encoder=%s class=%s dur=%s err=%v", runID, cacheKey, encoder, failureClass, time.Since(started).Round(time.Millisecond), err)
		s.setJob(cacheKey, replayStatus{
			RunID: runID, JobID: jobID, State: "error", ObjectKey: cacheKey, Error: clipError(err),
			JobState: farm.MediaRenderJobFailed, Stage: farm.MediaRenderJobFailed, FailureClass: failureClass,
		})
		if jobID != "" {
			if finishErr := s.finishRenderJob(context.Background(), jobID, farm.MediaRenderJobFailed, farm.MediaRenderJobFailed, clipError(err), 0, failureClass); finishErr != nil {
				log.Printf("pokereplay: persist failed render job %s: %v", jobID, finishErr)
			}
		}
	}

	dir, err := os.MkdirTemp(s.capacity.ScratchDir, replayScratchJobPrefix)
	if err != nil {
		setError(err)
		return
	}
	defer os.RemoveAll(dir)

	maxFrames := replaySegmentFrames()
	semanticSegments := make([]semanticReplaySegment, len(recordings))
	attempts := make([]preparedReplayAttempt, len(recordings))
	var segmentPlanErr error
	for index, recording := range recordings {
		recordingPath := pathJoinOS(dir, fmt.Sprintf("attempt-%03d.gbrun", recording.Attempt))
		if err := s.downloadRecording(ctx, runID, recording.Artifact, recordingPath, recording.Attempt); err != nil {
			setError(fmt.Errorf("attempt %d recording: %w", recording.Attempt, err))
			return
		}
		romPath, err := s.prepareStreamROM(dir, recordingPath)
		if err != nil {
			setError(fmt.Errorf("attempt %d replay ROM: %w", recording.Attempt, err))
			return
		}
		semantic := semanticReplaySegment{Attempt: recording.Attempt, RecordingPath: recordingPath, ReplayROMPath: romPath}
		semanticSegments[index] = semantic
		prepared, err := s.prepareReplayAttempt(runID, recording, semantic, dir, maxFrames)
		if err != nil {
			if segmentPlanErr == nil {
				segmentPlanErr = fmt.Errorf("attempt %d segment plan: %w", recording.Attempt, err)
			}
			continue
		}
		s.applyReplaySegmentMode(runID, mode, maxFrames, &prepared)
		attempts[index] = prepared
	}

	if segmentPlanErr != nil {
		log.Printf("pokereplay: bounded segment planning unavailable for run=%s; using whole-attempt compatibility path: %v", runID, segmentPlanErr)
		total, ready := len(recordings), 0
		s.setJob(cacheKey, replayStatus{RunID: runID, State: "generating", ObjectKey: cacheKey, Segments: total})
		if jobID != "" {
			if err := s.heartbeatRenderJob(ctx, jobID, farm.MediaRenderJobRendering, "legacy_attempts", &total, &ready); err != nil {
				if mediaRenderJobLeaseLost(err) {
					cancelForControl()
					return
				}
				log.Printf("pokereplay: persist legacy render start job=%s: %v", jobID, err)
			}
		}
		size, err := s.renderLegacyReplay(ctx, runID, recordings, semanticSegments, dir, mode, func(done, total int) {
			s.setJob(cacheKey, replayStatus{RunID: runID, State: "generating", ObjectKey: cacheKey, Segments: total, SegmentsDone: done})
			if jobID != "" {
				if err := s.heartbeatRenderJob(ctx, jobID, farm.MediaRenderJobRendering, "legacy_attempts", &total, &done); err != nil {
					if mediaRenderJobLeaseLost(err) {
						cancelForControl()
					} else {
						log.Printf("pokereplay: persist legacy render progress job=%s: %v", jobID, err)
					}
				}
			}
		})
		if err != nil {
			setError(err)
			return
		}
		elapsed := time.Since(started)
		log.Printf("pokereplay render ok run=%s key=%s encoder=%s dur=%s size=%d legacy=true", runID, cacheKey, encoder, elapsed.Round(time.Millisecond), size)
		s.recordRenderSuccess(size, elapsed)
		s.setJob(cacheKey, replayStatus{RunID: runID, State: "ready", ObjectKey: cacheKey, Size: size, Segments: total, SegmentsDone: total})
		if jobID != "" {
			if err := s.finishRenderJob(context.Background(), jobID, farm.MediaRenderJobReady, farm.MediaRenderJobReady, "", size); err != nil {
				log.Printf("pokereplay: persist ready legacy render job %s: %v", jobID, err)
			}
		}
		if err := s.renderSemanticReplay(ctx, runID, recordings, semanticSegments); err != nil {
			log.Printf("pokereplay semantic replay unavailable run=%s err=%v", runID, err)
		}
		return
	}

	total := totalReplayVideoSegments(attempts)
	ready, err := s.probeReplaySegmentCache(ctx, attempts)
	if err != nil {
		setError(fmt.Errorf("probe replay segment cache: %w", err))
		return
	}
	log.Printf("pokereplay render start run=%s key=%s encoder=%s vaapi=%t attempts=%d segments=%d cached=%d segment_frames=%d",
		runID, cacheKey, encoder, s.vaapi, len(recordings), total, ready, maxFrames)
	s.setJob(cacheKey, replayStatus{RunID: runID, State: "generating", ObjectKey: cacheKey, Segments: total, SegmentsDone: ready})
	if jobID != "" {
		if err := s.heartbeatRenderJob(ctx, jobID, farm.MediaRenderJobRendering, farm.MediaRenderJobRendering, &total, &ready); err != nil {
			log.Printf("pokereplay: persist render start job=%s: %v", jobID, err)
		}
	}

	done := ready
	onReady := func() {
		done++
		s.setJob(cacheKey, replayStatus{RunID: runID, State: "generating", ObjectKey: cacheKey, Segments: total, SegmentsDone: done})
		if jobID != "" {
			if progressErr := s.heartbeatRenderJob(ctx, jobID, farm.MediaRenderJobRendering, farm.MediaRenderJobRendering, &total, &done); progressErr != nil {
				log.Printf("pokereplay: persist render progress job=%s: %v", jobID, progressErr)
				if mediaRenderJobLeaseLost(progressErr) {
					cancelForControl()
				}
			}
		}
	}
	onStart := func(segment replayVideoSegment) {
		stage := fmt.Sprintf("rendering_attempt_%d_segment_%d", segment.Attempt, segment.Index+1)
		s.setJob(cacheKey, replayStatus{
			RunID: runID, State: "generating", ObjectKey: cacheKey,
			Segments: total, SegmentsDone: done, Stage: stage,
		})
		if jobID != "" {
			if progressErr := s.heartbeatRenderJob(ctx, jobID, farm.MediaRenderJobRendering, stage, &total, &done); progressErr != nil {
				log.Printf("pokereplay: persist active segment job=%s: %v", jobID, progressErr)
				if mediaRenderJobLeaseLost(progressErr) {
					cancelForControl()
				}
			}
		}
	}
	for index := range attempts {
		var renderErr error
		if mode == replayModeSemantic {
			renderErr = s.renderAttemptSemanticSegments(ctx, &attempts[index], onStart, onReady)
		} else {
			renderErr = s.renderAttemptVideoSegments(ctx, runID, mode, &attempts[index], onStart, onReady)
		}
		if renderErr != nil {
			setError(fmt.Errorf("attempt %d: %w", attempts[index].Recording.Attempt, renderErr))
			return
		}
	}

	if jobID != "" {
		if err := s.heartbeatRenderJob(ctx, jobID, farm.MediaRenderJobAssembling, farm.MediaRenderJobAssembling, nil, nil); err != nil {
			if mediaRenderJobLeaseLost(err) {
				cancelForControl()
				return
			}
			log.Printf("pokereplay: persist assembling job=%s: %v", jobID, err)
		}
	}
	segmentURLs, err := replayVideoSegmentURLs(s, attempts)
	if err != nil {
		setError(err)
		return
	}
	if len(segmentURLs) == 0 {
		setError(fmt.Errorf("replay segment plan produced no video"))
		return
	}
	videoPath := pathJoinOS(dir, "replay.mp4")
	s.encoderProcesses.Add(1)
	concatErr := concatReplaySegmentURLs(ctx, dir, segmentURLs, videoPath)
	s.encoderProcesses.Add(-1)
	if concatErr != nil {
		setError(concatErr)
		return
	}
	if err := probeReplayVideo(ctx, videoPath, replayAttemptsDuration(attempts)); err != nil {
		setError(fmt.Errorf("validate assembled replay: %w", err))
		return
	}

	file, err := os.Open(videoPath)
	if err != nil {
		setError(err)
		return
	}
	if jobID != "" {
		if err := s.heartbeatRenderJob(ctx, jobID, farm.MediaRenderJobUploading, farm.MediaRenderJobUploading, nil, nil); err != nil {
			if mediaRenderJobLeaseLost(err) {
				file.Close()
				cancelForControl()
				return
			}
			log.Printf("pokereplay: persist uploading job=%s: %v", jobID, err)
		}
	}
	obj, err := s.store.PutObjectReader(ctx, cacheKey, "video/mp4", file)
	file.Close()
	if err != nil {
		setError(err)
		return
	}
	size := obj.Size

	elapsed := time.Since(started)
	log.Printf("pokereplay render ok run=%s key=%s encoder=%s dur=%s size=%d segments=%d",
		runID, cacheKey, encoder, elapsed.Round(time.Millisecond), size, total)
	s.recordRenderSuccess(size, elapsed)
	s.setJob(cacheKey, replayStatus{RunID: runID, State: "ready", ObjectKey: cacheKey, Size: size, Segments: total, SegmentsDone: total})
	if jobID != "" {
		if err := s.finishRenderJob(context.Background(), jobID, farm.MediaRenderJobReady, farm.MediaRenderJobReady, "", size); err != nil {
			log.Printf("pokereplay: persist ready render job %s: %v", jobID, err)
		}
	}
	if err := s.renderSemanticReplay(ctx, runID, recordings, semanticSegments); err != nil {
		// The semantic cache is derived presentation data. Its failure must not
		// invalidate a deterministic recording or an otherwise healthy MP4.
		log.Printf("pokereplay semantic replay unavailable run=%s err=%v", runID, err)
	}
}

// renderCachedSegment returns a local MP4 for one attempt, downloading it from
// its S3 segment cache when present and otherwise rendering and caching it.
func (s *replayServer) renderCachedSegment(ctx context.Context, runID string, recording replayRecording, segment semanticReplaySegment, dir string, index int, mode replayMode) (string, error) {
	key := s.replayCacheKeyForMode(runID, []replayRecording{recording}, mode)
	video := pathJoinOS(dir, fmt.Sprintf("segment-%03d.mp4", index+1))
	if _, err := s.store.HeadObject(ctx, key); err == nil {
		return video, s.downloadObject(ctx, key, video)
	} else if !artifactstore.IsNotFound(err) {
		return "", err
	}

	release, err := acquireReplayRender(ctx)
	if err != nil {
		return "", fmt.Errorf("wait for replay render slot: %w", err)
	}
	defer release()
	ctx, cancel := withReplayTimeout(ctx, s.capacity.SegmentTimeout)
	defer cancel()

	raw := video
	if mode == replayModeBroadcast {
		raw = pathJoinOS(dir, fmt.Sprintf("segment-%03d-raw.mp4", index+1))
	}
	if err := s.renderRecordingSegment(ctx, segment.ReplayROMPath, segment.RecordingPath, raw); err != nil {
		return "", replayContextError(ctx, err)
	}
	if mode == replayModeBroadcast {
		if s.compositor == nil {
			return "", fmt.Errorf("broadcast compositor is not configured")
		}
		timeline, err := s.mediaTimelineOrEmpty(ctx, runID, recording.Attempt)
		if err != nil {
			return "", fmt.Errorf("media timeline: %w", err)
		}
		s.encoderProcesses.Add(1)
		err = s.compositor.Compose(ctx, broadcastScene{
			RunID:       runID,
			Attempt:     recording.Attempt,
			RawVideo:    raw,
			Destination: video,
			Timeline:    timeline,
			VAAPI:       s.vaapi,
			VAAPIDevice: vaapiDevice(),
		})
		s.encoderProcesses.Add(-1)
		if err != nil {
			return "", replayContextError(ctx, fmt.Errorf("broadcast renderer: %w", err))
		}
		_ = os.Remove(raw)
	}
	file, err := os.Open(video)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := s.store.PutObjectReader(ctx, key, "video/mp4", file); err != nil {
		return "", replayContextError(ctx, fmt.Errorf("cache segment: %w", err))
	}
	return video, nil
}

func (s *replayServer) downloadObject(ctx context.Context, key, destination string) error {
	obj, err := s.store.GetObject(ctx, key, "")
	if err != nil {
		return err
	}
	defer obj.Body.Close()
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, obj.Body); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func (s *replayServer) renderRecordingSegment(ctx context.Context, romPath, recordingPath, videoPath string) error {
	cmd := exec.CommandContext(ctx, s.streamBinary, s.streamArgs(romPath, recordingPath, videoPath)...)
	configureReplayProcessGroup(cmd)
	output := &replayOutputTail{}
	cmd.Stdout = output
	cmd.Stderr = output
	s.encoderProcesses.Add(1)
	defer s.encoderProcesses.Add(-1)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gomeboy replay render: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
}

func concatReplaySegments(ctx context.Context, dir string, videos []string, destination string) error {
	return mediasegment.ConcatFiles(ctx, dir, videos, destination, nil)
}

func (s *replayServer) downloadRecording(ctx context.Context, runID string, recording artifactRef, destination string, attempts ...int) error {
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer file.Close()
	h := sha256.New()
	writer := io.MultiWriter(file, h)

	if recording.Store == "" {
		endpoint := s.wallBase + "/v1/runs/" + url.PathEscape(runID) + "/artifacts/" + url.PathEscape(recording.Name) + "/content"
		if len(attempts) > 0 && attempts[0] > 0 {
			endpoint += "?attempt=" + strconv.Itoa(attempts[0])
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		res, err := s.wallHTTP.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("pokewall recording download returned %s", res.Status)
		}
		if _, err := io.Copy(writer, res.Body); err != nil {
			return err
		}
	} else {
		if recording.Store != "s3" {
			return fmt.Errorf("unsupported recording store %q", recording.Store)
		}
		if s.store == nil {
			return fmt.Errorf("S3 artifact storage is not configured")
		}
		if recording.Bucket != "" && recording.Bucket != s.store.Bucket() {
			return fmt.Errorf("recording bucket %q does not match configured bucket %q", recording.Bucket, s.store.Bucket())
		}
		obj, err := s.store.GetObject(ctx, recording.ObjectKey, "")
		if err != nil {
			return err
		}
		defer obj.Body.Close()
		if _, err := io.Copy(writer, obj.Body); err != nil {
			return err
		}
	}
	got := hex.EncodeToString(h.Sum(nil))
	if want := strings.ToLower(strings.TrimSpace(recording.SHA256)); want != "" && got != want {
		return fmt.Errorf("recording sha256 mismatch: got %s want %s", got, want)
	}
	return file.Sync()
}

func (s *replayServer) proxyInlineArtifact(w http.ResponseWriter, r *http.Request, runID, name string, attempt int) {
	endpoint := s.wallBase + "/v1/runs/" + url.PathEscape(runID) + "/artifacts/" + url.PathEscape(name) + "/content"
	if attempt > 0 {
		endpoint += "?attempt=" + strconv.Itoa(attempt)
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	res, err := s.wallHTTP.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer res.Body.Close()
	copyHeader(w.Header(), res.Header, "Content-Type", "Content-Disposition", "Content-Length", "Cache-Control")
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
}

func (s *replayServer) setJob(key string, status replayStatus) {
	s.mu.Lock()
	s.jobs[key] = status
	s.mu.Unlock()
	scheduleReplayJobExpiry(s, key, status)
}

func replaySetCacheKey(runID string, recordings []replayRecording) string {
	if len(recordings) == 1 {
		return replayCacheKey(runID, recordings[0].Artifact)
	}
	runSegment := sanitizeKeySegment(runID)
	if runSegment == "" {
		runSegment = "run"
	}
	dir := path.Join("derived", runSegment)
	if len(recordings) > 0 {
		if prefix, ok := recordingRunPrefix(recordings[len(recordings)-1].Artifact.ObjectKey); ok {
			dir = strings.TrimSuffix(prefix, "/")
		}
	}
	h := sha256.New()
	for _, recording := range recordings {
		fmt.Fprintf(h, "%d:%s:%s\n", recording.Attempt, strings.ToLower(strings.TrimSpace(recording.Artifact.SHA256)), strings.TrimSpace(recording.Artifact.ObjectKey))
	}
	fingerprint := hex.EncodeToString(h.Sum(nil))
	if len(fingerprint) > 12 {
		fingerprint = fingerprint[:12]
	}
	return path.Join(dir, "replay-full-"+fingerprint+".mp4")
}

func replayCacheKey(runID string, recording artifactRef) string {
	dir := path.Dir(strings.TrimPrefix(recording.ObjectKey, "/"))
	if recording.ObjectKey == "" || dir == "." {
		runSegment := sanitizeKeySegment(runID)
		if runSegment == "" {
			runSegment = "run"
		}
		dir = path.Join("derived", runSegment)
	}
	fingerprint := safeFingerprint(recording.SHA256)
	return path.Join(dir, "replay-"+fingerprint+".mp4")
}

func sanitizeKeySegment(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.TrimSpace(value) {
		allowed := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if allowed {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), ".-")
}

func safeFingerprint(sum string) string {
	sum = strings.ToLower(strings.TrimSpace(sum))
	var b strings.Builder
	for _, r := range sum {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			b.WriteRune(r)
		}
		if b.Len() == 12 {
			break
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func findArtifact(artifacts []artifactRef, name string) (artifactRef, bool) {
	for _, artifact := range artifacts {
		if artifact.Name == name {
			return artifact, true
		}
	}
	return artifactRef{}, false
}

func copyObjectResponse(w http.ResponseWriter, obj *artifactstore.ReadObject, fallbackType, filename string) {
	contentType := obj.ContentType
	if contentType == "" {
		contentType = fallbackType
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if obj.ContentRange != "" {
		w.Header().Set("Content-Range", obj.ContentRange)
	}
	if obj.AcceptRanges != "" {
		w.Header().Set("Accept-Ranges", obj.AcceptRanges)
	} else {
		w.Header().Set("Accept-Ranges", "bytes")
	}
	if obj.ETag != "" {
		w.Header().Set("ETag", obj.ETag)
	}
	if obj.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(obj.ContentLength, 10))
	}
	if filename != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeHeaderFilename(filename)))
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(obj.StatusCode)
	_, _ = io.Copy(w, obj.Body)
}

func safeHeaderFilename(name string) string {
	name = path.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, `"`, "_")
	name = strings.ReplaceAll(name, "\\", "_")
	if name == "" || name == "." {
		return "artifact.bin"
	}
	return name
}

func copyHeader(dst, src http.Header, names ...string) {
	for _, name := range names {
		if value := src.Get(name); value != "" {
			dst.Set(name, value)
		}
	}
}

func clipError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 4096 {
		msg = msg[:4096]
	}
	return msg
}

var (
	errRunNotFound       = errors.New("run not found")
	errRecordingNotFound = errors.New("run has no replayable run.gbrun artifact")
)

func writeReplayError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errRunNotFound), errors.Is(err, errRecordingNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func cartridgeForRecording(base []byte, metadata map[string]string, wantSHA256 string) ([]byte, error) {
	return redstarter.ReplayROM(base, metadata, wantSHA256)
}

func parseRecordingIdentity(data []byte) (replayIdentity, error) {
	rec, err := gomeboy.ParseRecording(data)
	if err != nil {
		return replayIdentity{}, err
	}
	return replayIdentity{ROMSHA256: rec.ROMSHA256, Metadata: rec.Metadata}, nil
}

func (s *replayServer) prepareStreamROM(workDir, recordingPath string) (string, error) {
	data, err := os.ReadFile(recordingPath)
	if err != nil {
		return "", err
	}
	parse := s.parseRecording
	if parse == nil {
		parse = parseRecordingIdentity
	}
	ident, err := parse(data)
	if err != nil {
		// Preserve the historical corrupt-recording behavior: let
		// gomeboy-stream report the recording problem against the fallback ROM.
		return s.romPath, nil
	}

	candidates := []string{s.romPath}
	if s.romLibrary != nil {
		candidates = s.romLibrary.candidates(ident.Metadata)
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no mounted replay ROM for game %q", strings.TrimSpace(ident.Metadata["game"]))
	}

	derive := s.deriveROM
	if derive == nil {
		derive = cartridgeForRecording
	}
	var candidateErrors []string
	for _, basePath := range candidates {
		if strings.TrimSpace(basePath) == "" {
			continue
		}
		base, readErr := os.ReadFile(basePath)
		if readErr != nil {
			candidateErrors = append(candidateErrors, fmt.Sprintf("%s: %v", basePath, readErr))
			continue
		}
		derived, deriveErr := derive(base, ident.Metadata, ident.ROMSHA256)
		if deriveErr != nil {
			candidateErrors = append(candidateErrors, fmt.Sprintf("%s: %v", basePath, deriveErr))
			continue
		}
		if bytes.Equal(derived, base) {
			return basePath, nil
		}
		name := "replay-" + safeFingerprint(ident.ROMSHA256) + ".gb"
		path := pathJoinOS(workDir, name)
		if err := os.WriteFile(path, derived, 0o644); err != nil {
			return "", err
		}
		return path, nil
	}
	if len(candidateErrors) == 0 {
		return "", fmt.Errorf("no mounted replay ROM matched recording sha256 %s", ident.ROMSHA256)
	}
	return "", fmt.Errorf("no mounted replay ROM matched recording sha256 %s: %s", ident.ROMSHA256, strings.Join(candidateErrors, "; "))
}

// pathJoinOS is intentionally tiny: temp paths are local filesystem paths,
// whereas replayCacheKey above must always use slash-separated S3 keys.
func pathJoinOS(dir, name string) string {
	return strings.TrimRight(dir, "/\\") + string(os.PathSeparator) + name
}

func main() {
	if filepath.Base(os.Args[0]) == "ffmpeg-vaapi" {
		os.Exit(runFFmpegVAAPI(os.Args[1:]))
	}
	httpAddr := flag.String("http", ":8080", "listen address for the replay HTTP API")
	wallBase := flag.String("wall", "", "pokewall base URL")
	romPath := flag.String("rom", "/rom/pokemon_red.gb", "fallback ROM path for legacy/unparseable replay recordings")
	romDir := flag.String("rom-dir", "/rom", "directory of mounted ROMs; cartridges are selected by detected game profile")
	streamBinary := flag.String("stream-binary", "/usr/local/bin/gomeboy-stream", "gomeboy-stream executable")
	flag.Parse()
	if strings.TrimSpace(*wallBase) == "" {
		log.Fatal("pokereplay: -wall is required")
	}

	store, configured, err := artifactstore.S3FromEnv()
	if err != nil {
		log.Printf("pokereplay: S3 configuration invalid; replay disabled: %v", err)
		store = nil
	} else if !configured {
		log.Printf("pokereplay: S3 not configured; artifact metadata remains browsable but replay cache is disabled")
	}
	serverImpl := newReplayServer(*wallBase, *romPath, *streamBinary, store)
	if err := prepareReplayScratch(serverImpl.capacity.ScratchDir); err != nil {
		log.Fatalf("pokereplay: prepare scratch directory: %v", err)
	}
	serverImpl.romLibrary = buildReplayROMLibrary(*romPath, *romDir)
	on, encoder, reason := currentVAAPI()
	serverImpl.vaapi = on
	serverImpl.vaapiReason = reason
	serverImpl.ffmpegVAAPI = defaultFFmpegVAAPI
	server := &http.Server{
		Addr:              *httpAddr,
		Handler:           serverImpl.handler(),
		ReadHeaderTimeout: serverReadHeaderTimeout,
		IdleTimeout:       serverIdleTimeout,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go serverImpl.runRenderJobRecovery(ctx, 10*time.Second)

	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	log.Printf("pokereplay listening on http://%s (wall %s, s3=%t, encoder=%s, vaapi=%t, %s)", *httpAddr, *wallBase, store != nil, encoder, on, reason)
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("pokereplay: server stopped: %v", err)
		}
	case <-ctx.Done():
		serverImpl.stopLiveSessions()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("pokereplay: graceful shutdown failed: %v", err)
			_ = server.Close()
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("pokereplay: server stopped during shutdown: %v", err)
		}
	}
}
