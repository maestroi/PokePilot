package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

var (
	errMediaRenderJobAPIUnavailable = errors.New("media render job API unavailable")
	replayWorkerID                  = newReplayWorkerID()
)

func newReplayWorkerID() string {
	host, _ := os.Hostname()
	host = sanitizeKeySegment(host)
	if host == "" {
		host = "pokereplay"
	}
	return fmt.Sprintf("%s-%d-%d", host, os.Getpid(), time.Now().UnixNano())
}

func (s *replayServer) ensureRenderJob(ctx context.Context, runID string, recordings []replayRecording, mode replayMode, artifactKey string) (farm.MediaRenderJob, error) {
	attempts := make([]int, 0, len(recordings))
	for _, recording := range recordings {
		attempts = append(attempts, recording.Attempt)
	}
	request := farm.MediaRenderJobCreateRequest{
		Identity:    artifactKey,
		RunID:       runID,
		Attempts:    attempts,
		Mode:        string(mode),
		ArtifactKey: artifactKey,
	}
	var job farm.MediaRenderJob
	status, err := s.mediaJobRequest(ctx, http.MethodPost, "/v1/media/render-jobs", request, &job)
	if status == http.StatusNotFound {
		return farm.MediaRenderJob{}, errMediaRenderJobAPIUnavailable
	}
	if err != nil {
		return farm.MediaRenderJob{}, err
	}
	return job, nil
}

func (s *replayServer) getRenderJob(ctx context.Context, id string) (farm.MediaRenderJob, bool, error) {
	var job farm.MediaRenderJob
	status, err := s.mediaJobRequest(ctx, http.MethodGet, "/v1/media/render-jobs/"+id, nil, &job)
	if status == http.StatusNotFound {
		return farm.MediaRenderJob{}, false, nil
	}
	if err != nil {
		return farm.MediaRenderJob{}, false, err
	}
	return job, true, nil
}

func (s *replayServer) claimRenderJob(ctx context.Context, id string) (farm.MediaRenderJob, bool, error) {
	var job farm.MediaRenderJob
	status, err := s.mediaJobRequest(ctx, http.MethodPost, "/v1/media/render-jobs/"+id+"/claim", farm.MediaRenderJobClaimRequest{WorkerID: replayWorkerID}, &job)
	if status == http.StatusConflict {
		return job, false, nil
	}
	if status == http.StatusNotFound {
		return farm.MediaRenderJob{}, false, errMediaRenderJobAPIUnavailable
	}
	if err != nil {
		return farm.MediaRenderJob{}, false, err
	}
	return job, true, nil
}

func (s *replayServer) retryRenderJob(ctx context.Context, id string) (farm.MediaRenderJob, error) {
	var job farm.MediaRenderJob
	status, err := s.mediaJobRequest(ctx, http.MethodPost, "/v1/media/render-jobs/"+id+"/retry", struct{}{}, &job)
	if status == http.StatusNotFound {
		return farm.MediaRenderJob{}, errMediaRenderJobAPIUnavailable
	}
	if err != nil {
		return farm.MediaRenderJob{}, err
	}
	return job, nil
}

func (s *replayServer) heartbeatRenderJob(ctx context.Context, id, state, stage string, total, done *int) error {
	var job farm.MediaRenderJob
	_, err := s.mediaJobRequest(ctx, http.MethodPost, "/v1/media/render-jobs/"+id+"/heartbeat", farm.MediaRenderJobProgressRequest{
		WorkerID:      replayWorkerID,
		State:         state,
		Stage:         stage,
		SegmentsTotal: total,
		SegmentsDone:  done,
	}, &job)
	return err
}

func (s *replayServer) finishRenderJob(ctx context.Context, id, state, stage, lastError string, resultSize int64) error {
	var job farm.MediaRenderJob
	_, err := s.mediaJobRequest(ctx, http.MethodPost, "/v1/media/render-jobs/"+id+"/finish", farm.MediaRenderJobFinishRequest{
		WorkerID:   replayWorkerID,
		State:      state,
		Stage:      stage,
		LastError:  lastError,
		ResultSize: resultSize,
	}, &job)
	return err
}

func (s *replayServer) reconcileRenderJobReady(ctx context.Context, id string, size int64) {
	var job farm.MediaRenderJob
	_, err := s.mediaJobRequest(ctx, http.MethodPost, "/v1/media/render-jobs/"+id+"/reconcile-ready", map[string]int64{"size": size}, &job)
	if err != nil {
		log.Printf("pokereplay: reconcile ready render job %s: %v", id, err)
	}
}

func (s *replayServer) claimableRenderJobs(ctx context.Context) ([]farm.MediaRenderJob, error) {
	var response struct {
		Jobs []farm.MediaRenderJob `json:"jobs"`
	}
	status, err := s.mediaJobRequest(ctx, http.MethodGet, "/v1/media/render-jobs?claimable=1", nil, &response)
	if status == http.StatusNotFound {
		return nil, errMediaRenderJobAPIUnavailable
	}
	if err != nil {
		return nil, err
	}
	return response.Jobs, nil
}

func (s *replayServer) mediaJobRequest(ctx context.Context, method, endpoint string, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.wallBase+endpoint, reader)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := s.wallHTTP.Do(req)
	if err != nil {
		return 0, fmt.Errorf("pokewall media render jobs unavailable: %w", err)
	}
	defer res.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(res.Body, maxWallResponseBytes+1))
	if readErr != nil {
		return res.StatusCode, readErr
	}
	if len(raw) > maxWallResponseBytes {
		return res.StatusCode, fmt.Errorf("pokewall media render job response exceeds %d bytes", maxWallResponseBytes)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if out != nil && len(raw) > 0 {
			_ = json.Unmarshal(raw, out)
		}
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		if message, _ := payload["error"].(string); strings.TrimSpace(message) != "" {
			return res.StatusCode, errors.New(message)
		}
		return res.StatusCode, fmt.Errorf("pokewall media render jobs returned %s", res.Status)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return res.StatusCode, fmt.Errorf("decode pokewall media render job response: %w", err)
		}
	}
	return res.StatusCode, nil
}

func replayStatusFromMediaJob(job farm.MediaRenderJob) replayStatus {
	status := replayStatus{
		RunID:        job.RunID,
		JobID:        job.ID,
		ObjectKey:    job.ArtifactKey,
		Size:         job.ResultSize,
		Segments:     job.SegmentsTotal,
		SegmentsDone: job.SegmentsDone,
		JobState:     job.State,
		Stage:        job.Stage,
		RetryCount:   job.RetryCount,
		LastError:    job.LastError,
	}
	switch job.State {
	case farm.MediaRenderJobReady:
		status.State = "ready"
	case farm.MediaRenderJobFailed:
		status.State = "error"
		status.Error = job.LastError
	case farm.MediaRenderJobCancelled:
		status.State = "error"
		status.Error = firstNonEmpty(job.LastError, "render cancelled")
	default:
		status.State = "generating"
	}
	return status
}

func (s *replayServer) runRenderJobRecovery(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	recover := func() {
		jobs, err := s.claimableRenderJobs(ctx)
		if errors.Is(err, errMediaRenderJobAPIUnavailable) {
			return
		}
		if err != nil {
			log.Printf("pokereplay: list recoverable render jobs: %v", err)
			return
		}
		for _, job := range jobs {
			s.recoverRenderJob(ctx, job)
		}
	}
	recover()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recover()
		}
	}
}

func (s *replayServer) recoverRenderJob(ctx context.Context, job farm.MediaRenderJob) {
	if s.store == nil {
		return
	}
	mode, err := parseReplayMode(job.Mode)
	if err != nil {
		claimed, ok, claimErr := s.claimRenderJob(ctx, job.ID)
		if claimErr == nil && ok {
			_ = s.finishRenderJob(context.Background(), claimed.ID, farm.MediaRenderJobFailed, farm.MediaRenderJobFailed, err.Error(), 0)
		}
		return
	}
	recordings, err := s.recordingsForAttempts(ctx, job.RunID, job.Attempts)
	if err != nil {
		return
	}
	cacheKey := s.replayCacheKeyForMode(job.RunID, recordings, mode)
	if cacheKey != job.ArtifactKey {
		claimed, ok, claimErr := s.claimRenderJob(ctx, job.ID)
		if claimErr == nil && ok {
			_ = s.finishRenderJob(context.Background(), claimed.ID, farm.MediaRenderJobFailed, farm.MediaRenderJobFailed, "render identity no longer matches source recordings", 0)
		}
		return
	}
	claimed, ok, err := s.claimRenderJob(ctx, job.ID)
	if err != nil || !ok {
		return
	}
	go s.render(claimed.ID, job.RunID, recordings, cacheKey, mode)
}

func mediaRenderJobLeaseLost(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "owned by another worker") ||
		strings.Contains(message, "not active") ||
		strings.Contains(message, "media render job not found")
}

func (s *replayServer) keepRenderJobLease(ctx context.Context, jobID string, onLeaseLost func()) context.CancelFunc {
	heartbeatCtx, cancel := context.WithCancel(ctx)
	if jobID == "" {
		return cancel
	}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				if err := s.heartbeatRenderJob(heartbeatCtx, jobID, "", "", nil, nil); err != nil {
					log.Printf("pokereplay: heartbeat render job %s: %v", jobID, err)
					if mediaRenderJobLeaseLost(err) {
						if onLeaseLost != nil {
							onLeaseLost()
						}
						return
					}
				}
			}
		}
	}()
	return cancel
}
