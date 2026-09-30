package farm

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	MediaRenderJobVersion = 1

	MediaRenderJobQueued     = "queued"
	MediaRenderJobPreparing  = "preparing"
	MediaRenderJobRendering  = "rendering"
	MediaRenderJobAssembling = "assembling"
	MediaRenderJobUploading  = "uploading"
	MediaRenderJobReady      = "ready"
	MediaRenderJobFailed     = "failed"
	MediaRenderJobCancelled  = "cancelled"

	MediaRenderFailureInfrastructure = "infrastructure"
	MediaRenderFailureInvalidRequest = "invalid_request"
	MediaRenderFailureResource       = "resource"
	MediaRenderFailureTimeout        = "timeout"
	MediaRenderFailureCancelled      = "cancelled"
)

type MediaRenderJob struct {
	Version        int    `json:"version"`
	ID             string `json:"id"`
	Identity       string `json:"identity"`
	RunID          string `json:"run_id"`
	Attempts       []int  `json:"attempts,omitempty"`
	Mode           string `json:"mode"`
	ArtifactKey    string `json:"artifact_key"`
	State          string `json:"state"`
	Stage          string `json:"stage,omitempty"`
	SegmentsTotal  int    `json:"segments_total,omitempty"`
	SegmentsDone   int    `json:"segments_done,omitempty"`
	WorkerID       string `json:"worker_id,omitempty"`
	LeaseExpiresAt int64  `json:"lease_expires_at_unix_ms,omitempty"`
	RetryCount     int    `json:"retry_count,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	FailureClass   string `json:"failure_class,omitempty"`
	ResultSize     int64  `json:"result_size,omitempty"`
	CreatedAt      int64  `json:"created_at_unix_ms"`
	UpdatedAt      int64  `json:"updated_at_unix_ms"`
	StartedAt      int64  `json:"started_at_unix_ms,omitempty"`
	FinishedAt     int64  `json:"finished_at_unix_ms,omitempty"`
}

type MediaRenderJobCreateRequest struct {
	Identity    string `json:"identity"`
	RunID       string `json:"run_id"`
	Attempts    []int  `json:"attempts,omitempty"`
	Mode        string `json:"mode"`
	ArtifactKey string `json:"artifact_key"`
}

type MediaRenderJobClaimRequest struct {
	WorkerID string `json:"worker_id"`
}

type MediaRenderJobProgressRequest struct {
	WorkerID      string `json:"worker_id"`
	State         string `json:"state,omitempty"`
	Stage         string `json:"stage,omitempty"`
	SegmentsTotal *int   `json:"segments_total,omitempty"`
	SegmentsDone  *int   `json:"segments_done,omitempty"`
}

type MediaRenderJobFinishRequest struct {
	WorkerID     string `json:"worker_id"`
	State        string `json:"state"`
	Stage        string `json:"stage,omitempty"`
	LastError    string `json:"last_error,omitempty"`
	FailureClass string `json:"failure_class,omitempty"`
	ResultSize   int64  `json:"result_size,omitempty"`
}

func MediaRenderJobID(identity string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(identity)))
	return "render-" + hex.EncodeToString(sum[:12])
}

func NewMediaRenderJob(req MediaRenderJobCreateRequest, now time.Time) (MediaRenderJob, error) {
	req.Identity = strings.TrimSpace(req.Identity)
	req.RunID = strings.TrimSpace(req.RunID)
	req.Mode = strings.TrimSpace(req.Mode)
	req.ArtifactKey = strings.TrimSpace(req.ArtifactKey)
	if req.Identity == "" {
		return MediaRenderJob{}, errors.New("media render identity is required")
	}
	if req.RunID == "" {
		return MediaRenderJob{}, errors.New("media render run_id is required")
	}
	if req.Mode == "" {
		return MediaRenderJob{}, errors.New("media render mode is required")
	}
	if req.ArtifactKey == "" {
		return MediaRenderJob{}, errors.New("media render artifact_key is required")
	}
	attempts := append([]int(nil), req.Attempts...)
	for _, attempt := range attempts {
		if attempt < 1 {
			return MediaRenderJob{}, errors.New("media render attempts must be positive")
		}
	}
	ms := now.UnixMilli()
	return MediaRenderJob{
		Version:     MediaRenderJobVersion,
		ID:          MediaRenderJobID(req.Identity),
		Identity:    req.Identity,
		RunID:       req.RunID,
		Attempts:    attempts,
		Mode:        req.Mode,
		ArtifactKey: req.ArtifactKey,
		State:       MediaRenderJobQueued,
		Stage:       MediaRenderJobQueued,
		CreatedAt:   ms,
		UpdatedAt:   ms,
	}, nil
}

func (j MediaRenderJob) Active() bool {
	switch j.State {
	case MediaRenderJobPreparing, MediaRenderJobRendering, MediaRenderJobAssembling, MediaRenderJobUploading:
		return true
	default:
		return false
	}
}

func (j MediaRenderJob) Terminal() bool {
	switch j.State {
	case MediaRenderJobReady, MediaRenderJobFailed, MediaRenderJobCancelled:
		return true
	default:
		return false
	}
}

func (j MediaRenderJob) Retryable() bool {
	switch j.State {
	case MediaRenderJobFailed:
		return j.FailureClass != MediaRenderFailureInvalidRequest
	case MediaRenderJobCancelled:
		return true
	default:
		return false
	}
}

func (j MediaRenderJob) Claimable(now time.Time) bool {
	if j.State == MediaRenderJobQueued {
		return true
	}
	return j.Active() && j.LeaseExpiresAt > 0 && j.LeaseExpiresAt <= now.UnixMilli()
}
