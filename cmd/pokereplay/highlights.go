package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
	mediaartifact "github.com/maestroi/pokepilot/media/artifact"
	"github.com/maestroi/pokepilot/media/compositor"
	mediahighlight "github.com/maestroi/pokepilot/media/highlight"
)

const (
	highlightRenderMode = "highlight"
	highlightTargetSecondsEnv = "POKEPILOT_HIGHLIGHT_TARGET_SECONDS"
	highlightMergeGapSecondsEnv = "POKEPILOT_HIGHLIGHT_MERGE_GAP_SECONDS"
	highlightPolicyJSONEnv = "POKEPILOT_HIGHLIGHT_POLICY_JSON"
)

type highlightStatus struct {
	RunID        string `json:"run_id"`
	JobID        string `json:"job_id,omitempty"`
	State        string `json:"state"`
	ObjectKey    string `json:"object_key,omitempty"`
	ManifestKey  string `json:"manifest_key,omitempty"`
	Size         int64  `json:"size,omitempty"`
	Error        string `json:"error,omitempty"`
	LastError    string `json:"last_error,omitempty"`
	JobState     string `json:"job_state,omitempty"`
	Stage        string `json:"stage,omitempty"`
	RetryCount   int    `json:"retry_count,omitempty"`
	FailureClass string `json:"failure_class,omitempty"`
	Windows      int    `json:"windows,omitempty"`
	WindowsDone  int    `json:"windows_done,omitempty"`
	DurationMS   int64  `json:"duration_ms,omitempty"`
	PlanHash     string `json:"plan_hash,omitempty"`
}

type highlightManifest struct {
	Version   int                     `json:"version"`
	RunID     string                  `json:"run_id"`
	Plan      mediahighlight.Plan     `json:"plan"`
	ObjectKey string                  `json:"object_key"`
	Clips     []highlightManifestClip `json:"clips"`
}

type highlightManifestClip struct {
	Index      int                       `json:"index"`
	Attempt    int                       `json:"attempt"`
	StartFrame uint64                    `json:"start_frame"`
	EndFrame   uint64                    `json:"end_frame"`
	ObjectKey  string                    `json:"object_key"`
	Events     []mediahighlight.EventRef `json:"events"`
}

func highlightPolicyFromEnv() mediahighlight.Policy {
	policy := mediahighlight.DefaultPolicy()
	if raw := strings.TrimSpace(os.Getenv(highlightPolicyJSONEnv)); raw != "" {
		var configured mediahighlight.Policy
		if err := json.Unmarshal([]byte(raw), &configured); err != nil {
			log.Printf("pokereplay: ignoring invalid %s: %v", highlightPolicyJSONEnv, err)
		} else {
			policy = configured
		}
	}
	if seconds := positiveEnvInt(highlightTargetSecondsEnv); seconds > 0 {
		policy.TargetDurationMS = int64(seconds) * 1000
	}
	if seconds := nonNegativeEnvInt(highlightMergeGapSecondsEnv); seconds >= 0 {
		policy.MergeGapMS = int64(seconds) * 1000
	}
	return policy
}

func positiveEnvInt(name string) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		log.Printf("pokereplay: ignoring invalid %s=%q", name, raw)
		return 0
	}
	return value
}

func nonNegativeEnvInt(name string) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return -1
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		log.Printf("pokereplay: ignoring invalid %s=%q", name, raw)
		return -1
	}
	return value
}

func (s *replayServer) highlightPlan(ctx context.Context, runID string, recordings []replayRecording) (mediahighlight.Plan, error) {
	timelines := make([]farm.MediaTimeline, 0, len(recordings))
	for _, recording := range recordings {
		timeline, err := s.mediaTimelineAttempt(ctx, runID, recording.Attempt)
		if errors.Is(err, errMediaTimelineNotFound) {
			continue
		}
		if err != nil {
			return mediahighlight.Plan{}, err
		}
		timelines = append(timelines, timeline)
	}
	return mediahighlight.Build(timelines, highlightPolicyFromEnv()), nil
}

func (s *replayServer) highlightProfile(plan mediahighlight.Plan) mediaartifact.Profile {
	profile := s.replayProfileForMode(replayModeBroadcast, replaySegmentFrames())
	profile.Name = mediaartifact.ProfileHighlight
	profile.EventPolicy = fmt.Sprintf("highlight-policy-v%d", plan.PolicyVersion)
	profile.SegmentPolicy = mediaartifact.Component{ID: "highlight-windows", Version: fmt.Sprintf("v%d", plan.Version)}
	profile.EditPlan = &mediaartifact.Content{
		Version: fmt.Sprintf("highlight-plan-v%d", plan.Version),
		SHA256:  plan.Hash,
	}
	return mediaartifact.NormalizeProfile(profile)
}

func (s *replayServer) highlightIdentity(runID string, recordings []replayRecording, plan mediahighlight.Plan) mediaartifact.IdentityInput {
	selected := make(map[int]struct{}, len(plan.Windows))
	for _, window := range plan.Windows {
		selected[window.Attempt] = struct{}{}
	}
	input := mediaartifact.IdentityInput{Profile: s.highlightProfile(plan)}
	for _, recording := range recordings {
		if _, ok := selected[recording.Attempt]; ok {
			input.Sources = append(input.Sources, mediaartifact.Source{
				Attempt: recording.Attempt, SHA256: recording.Artifact.SHA256, FallbackID: recording.Artifact.ObjectKey,
			})
			input.Timelines = append(input.Timelines, mediaartifact.Content{
				Attempt: recording.Attempt,
				Version: fmt.Sprintf("media-timeline-v%d", farm.MediaTimelineVersion),
				SHA256: recording.Timeline.SHA256, FallbackID: recording.Timeline.ObjectKey,
			})
		}
	}
	return input
}

func (s *replayServer) highlightKeys(runID string, recordings []replayRecording, plan mediahighlight.Plan) (string, string) {
	base := replaySetCacheKey(runID, recordings)
	token := mediaartifact.Token(s.highlightIdentity(runID, recordings, plan))
	dir := path.Join(path.Dir(base), "highlights")
	return path.Join(dir, "highlight-"+token+".mp4"), path.Join(dir, "highlight-"+token+".json")
}

func highlightClipCacheKey(videoKey string, window mediahighlight.Window) string {
	stem := strings.TrimSuffix(path.Base(videoKey), path.Ext(videoKey))
	return path.Join(path.Dir(videoKey), "clips", stem,
		fmt.Sprintf("%03d-a%d-f%d-%d.mp4", window.Index, window.Attempt, window.StartFrame, window.EndFrame))
}

func (s *replayServer) highlightStatus(ctx context.Context, runID string, recordings []replayRecording, plan mediahighlight.Plan) highlightStatus {
	status := highlightStatus{
		RunID: runID, Windows: len(plan.Windows), DurationMS: plan.DurationMS, PlanHash: plan.Hash,
	}
	if len(plan.Windows) == 0 {
		status.State = "empty"
		return status
	}
	if s.store == nil {
		status.State = "disabled"
		status.Error = "S3 artifact storage is not configured for the replay service"
		return status
	}
	videoKey, manifestKey := s.highlightKeys(runID, recordings, plan)
	status.ObjectKey, status.ManifestKey = videoKey, manifestKey
	status.JobID = farm.MediaRenderJobID(videoKey)
	video, videoErr := s.store.HeadObject(ctx, videoKey)
	_, manifestErr := s.store.HeadObject(ctx, manifestKey)
	if videoErr == nil && manifestErr == nil {
		status.State, status.JobState, status.Stage, status.Size = "ready", farm.MediaRenderJobReady, farm.MediaRenderJobReady, video.Size
		return status
	}
	if videoErr != nil && !artifactstore.IsNotFound(videoErr) {
		status.State, status.Error = "error", videoErr.Error()
		return status
	}
	if manifestErr != nil && !artifactstore.IsNotFound(manifestErr) {
		status.State, status.Error = "error", manifestErr.Error()
		return status
	}
	if job, ok, err := s.getRenderJob(ctx, status.JobID); err == nil && ok {
		return highlightStatusFromJob(job, plan, manifestKey)
	}
	s.mu.Lock()
	local, ok := s.jobs[videoKey]
	s.mu.Unlock()
	if ok {
		status.State = local.State
		status.Size = local.Size
		status.Error = local.Error
		status.LastError = local.LastError
		status.Stage = local.Stage
		status.WindowsDone = local.SegmentsDone
		status.FailureClass = local.FailureClass
		return status
	}
	status.State = "missing"
	return status
}

func highlightStatusFromJob(job farm.MediaRenderJob, plan mediahighlight.Plan, manifestKey string) highlightStatus {
	base := replayStatusFromMediaJob(job)
	return highlightStatus{
		RunID: base.RunID, JobID: base.JobID, State: base.State, ObjectKey: base.ObjectKey, ManifestKey: manifestKey,
		Size: base.Size, Error: base.Error, LastError: base.LastError, JobState: base.JobState, Stage: base.Stage,
		RetryCount: base.RetryCount, FailureClass: base.FailureClass,
		Windows: len(plan.Windows), WindowsDone: base.SegmentsDone, DurationMS: plan.DurationMS, PlanHash: plan.Hash,
	}
}

func (s *replayServer) handleHighlightStatus(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	recordings, err := s.recordings(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	plan, err := s.highlightPlan(r.Context(), runID, recordings)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s.highlightStatus(r.Context(), runID, recordings, plan))
}

func (s *replayServer) handleHighlightRender(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	recordings, err := s.recordings(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	plan, err := s.highlightPlan(r.Context(), runID, recordings)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	status := s.highlightStatus(r.Context(), runID, recordings, plan)
	if status.State == "empty" || status.State == "ready" {
		writeJSON(w, http.StatusOK, status)
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}
	videoKey, manifestKey := s.highlightKeys(runID, recordings, plan)
	job, jobErr := s.ensureRenderJobMode(r.Context(), runID, recordings, highlightRenderMode, videoKey)
	if jobErr == nil {
		if job.State == farm.MediaRenderJobFailed || job.State == farm.MediaRenderJobCancelled {
			if job.State == farm.MediaRenderJobFailed && !job.Retryable() {
				writeJSON(w, http.StatusConflict, highlightStatusFromJob(job, plan, manifestKey))
				return
			}
			retried, retryErr := s.retryRenderJob(r.Context(), job.ID)
			if retryErr != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": retryErr.Error()})
				return
			}
			s.renderRetries.Add(1)
			job = retried
		} else if job.Active() {
			writeJSON(w, http.StatusAccepted, highlightStatusFromJob(job, plan, manifestKey))
			return
		}
		release, reason, detail := s.tryAdmitRenderJob(job.ID)
		if release == nil {
			status = highlightStatusFromJob(job, plan, manifestKey)
			status.Stage, status.LastError = "queued_"+reason, detail
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
			writeJSON(w, http.StatusAccepted, highlightStatusFromJob(claimed, plan, manifestKey))
			return
		}
		go func() {
			defer release()
			s.renderHighlights(claimed.ID, runID, recordings, plan, videoKey, manifestKey)
		}()
		writeJSON(w, http.StatusAccepted, highlightStatusFromJob(claimed, plan, manifestKey))
		return
	}
	if !errors.Is(jobErr, errMediaRenderJobAPIUnavailable) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": jobErr.Error()})
		return
	}
	release, reason, detail := s.tryAdmitRenderJob(videoKey)
	if release == nil {
		status.State, status.Stage, status.LastError = "generating", "queued_"+reason, detail
		s.setJob(videoKey, replayStatus{RunID: runID, State: "generating", ObjectKey: videoKey, Stage: status.Stage, LastError: detail})
		writeJSON(w, http.StatusAccepted, status)
		return
	}
	s.setJob(videoKey, replayStatus{RunID: runID, State: "generating", ObjectKey: videoKey, Stage: farm.MediaRenderJobPreparing})
	go func() {
		defer release()
		s.renderHighlights("", runID, recordings, plan, videoKey, manifestKey)
	}()
	status.State, status.Stage = "generating", farm.MediaRenderJobPreparing
	writeJSON(w, http.StatusAccepted, status)
}

func (s *replayServer) handleHighlightVideo(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	recordings, err := s.recordings(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	plan, err := s.highlightPlan(r.Context(), runID, recordings)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	status := s.highlightStatus(r.Context(), runID, recordings, plan)
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
	copyObjectResponse(w, obj, "video/mp4", "highlights.mp4")
}

func (s *replayServer) handleHighlightManifest(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	recordings, err := s.recordings(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	plan, err := s.highlightPlan(r.Context(), runID, recordings)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	status := s.highlightStatus(r.Context(), runID, recordings, plan)
	if status.State != "ready" {
		writeJSON(w, http.StatusConflict, status)
		return
	}
	obj, err := s.store.GetObject(r.Context(), status.ManifestKey, "")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer obj.Body.Close()
	copyObjectResponse(w, obj, "application/json", "highlights.json")
}

func (s *replayServer) recoverHighlightRenderJob(ctx context.Context, job farm.MediaRenderJob) {
	recordings, err := s.recordingsForAttempts(ctx, job.RunID, job.Attempts)
	if err != nil {
		return
	}
	plan, err := s.highlightPlan(ctx, job.RunID, recordings)
	if err != nil || len(plan.Windows) == 0 {
		claimed, ok, claimErr := s.claimRenderJob(ctx, job.ID)
		if claimErr == nil && ok {
			message := "highlight plan has no eligible events"
			if err != nil { message = err.Error() }
			_ = s.finishRenderJob(context.Background(), claimed.ID, farm.MediaRenderJobFailed, farm.MediaRenderJobFailed, message, 0, farm.MediaRenderFailureInvalidRequest)
		}
		return
	}
	videoKey, manifestKey := s.highlightKeys(job.RunID, recordings, plan)
	if videoKey != job.ArtifactKey {
		claimed, ok, claimErr := s.claimRenderJob(ctx, job.ID)
		if claimErr == nil && ok {
			_ = s.finishRenderJob(context.Background(), claimed.ID, farm.MediaRenderJobFailed, farm.MediaRenderJobFailed, "highlight identity no longer matches source timeline", 0, farm.MediaRenderFailureInvalidRequest)
		}
		return
	}
	release, _, _ := s.tryAdmitRenderJob(job.ID)
	if release == nil {
		return
	}
	claimed, ok, err := s.claimRenderJob(ctx, job.ID)
	if err != nil || !ok {
		release()
		return
	}
	go func() {
		defer release()
		s.renderHighlights(claimed.ID, job.RunID, recordings, plan, videoKey, manifestKey)
	}()
}

func (s *replayServer) renderHighlights(jobID, runID string, recordings []replayRecording, plan mediahighlight.Plan, videoKey, manifestKey string) {
	s.rendering.Add(1)
	defer s.rendering.Add(-1)
	ctx, cancel := withReplayTimeout(context.Background(), s.capacity.JobTimeout)
	defer cancel()
	var cancelledByControl atomic.Bool
	cancelForControl := func() {
		cancelledByControl.Store(true)
		cancel()
	}
	controlID := firstNonEmpty(jobID, videoKey)
	s.registerRenderCancel(controlID, cancelForControl)
	defer s.unregisterRenderCancel(controlID)
	stopLease := s.keepRenderJobLease(ctx, jobID, cancelForControl)
	defer stopLease()
	started := time.Now()
	setError := func(err error) {
		if cancelledByControl.Load() {
			s.setJob(videoKey, replayStatus{RunID: runID, JobID: jobID, State: "error", ObjectKey: videoKey, Error: "render cancelled", JobState: farm.MediaRenderJobCancelled, Stage: farm.MediaRenderJobCancelled, FailureClass: farm.MediaRenderFailureCancelled})
			return
		}
		class := classifyRenderFailure(err, ctx.Err())
		s.renderFailures.Add(1)
		s.setJob(videoKey, replayStatus{RunID: runID, JobID: jobID, State: "error", ObjectKey: videoKey, Error: clipError(err), JobState: farm.MediaRenderJobFailed, Stage: farm.MediaRenderJobFailed, FailureClass: class})
		if jobID != "" {
			if finishErr := s.finishRenderJob(context.Background(), jobID, farm.MediaRenderJobFailed, farm.MediaRenderJobFailed, clipError(err), 0, class); finishErr != nil {
				log.Printf("pokereplay: persist failed highlight job %s: %v", jobID, finishErr)
			}
		}
	}

	dir, err := os.MkdirTemp(s.capacity.ScratchDir, replayScratchJobPrefix+"highlight-")
	if err != nil {
		setError(err)
		return
	}
	defer os.RemoveAll(dir)

	windowsByAttempt := make(map[int][]mediahighlight.Window)
	for _, window := range plan.Windows {
		windowsByAttempt[window.Attempt] = append(windowsByAttempt[window.Attempt], window)
	}
	var attempts []preparedReplayAttempt
	for _, recording := range recordings {
		windows := windowsByAttempt[recording.Attempt]
		if len(windows) == 0 {
			continue
		}
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
		parsed, err := gomeboy.LoadRecording(recordingPath)
		if err != nil {
			setError(fmt.Errorf("attempt %d load recording: %w", recording.Attempt, err))
			return
		}
		maxFrame := parsed.DurationFrames()
		segments := make([]replayVideoSegment, 0, len(windows))
		for _, window := range windows {
			if window.StartFrame > maxFrame {
				continue
			}
			end := window.EndFrame
			if end > maxFrame { end = maxFrame }
			segment := replayVideoSegment{
				Attempt: recording.Attempt, Index: window.Index, StartFrame: window.StartFrame, EndFrame: end,
				CacheKey: highlightClipCacheKey(videoKey, window),
				LocalPath: pathJoinOS(dir, fmt.Sprintf("highlight-%03d-a%d.mp4", window.Index, recording.Attempt)),
			}
			segments = append(segments, segment)
		}
		if len(segments) == 0 {
			continue
		}
		attempts = append(attempts, preparedReplayAttempt{
			Recording: recording,
			Semantic: semanticReplaySegment{Attempt: recording.Attempt, RecordingPath: recordingPath, ReplayROMPath: romPath},
			Parsed: parsed, Segments: segments,
		})
	}
	if len(attempts) == 0 {
		setError(fmt.Errorf("highlight plan produced no renderable windows"))
		return
	}

	total := totalReplayVideoSegments(attempts)
	ready, err := s.probeReplaySegmentCache(ctx, attempts)
	if err != nil {
		setError(fmt.Errorf("probe highlight clip cache: %w", err))
		return
	}
	s.setJob(videoKey, replayStatus{RunID: runID, JobID: jobID, State: "generating", ObjectKey: videoKey, Segments: total, SegmentsDone: ready, Stage: farm.MediaRenderJobRendering})
	if jobID != "" {
		if err := s.heartbeatRenderJob(ctx, jobID, farm.MediaRenderJobRendering, "highlight_windows", &total, &ready); err != nil && mediaRenderJobLeaseLost(err) {
			cancelForControl()
			return
		}
	}
	done := ready
	onReady := func() {
		done++
		s.setJob(videoKey, replayStatus{RunID: runID, JobID: jobID, State: "generating", ObjectKey: videoKey, Segments: total, SegmentsDone: done, Stage: farm.MediaRenderJobRendering})
		if jobID != "" {
			if err := s.heartbeatRenderJob(ctx, jobID, farm.MediaRenderJobRendering, "highlight_windows", &total, &done); err != nil && mediaRenderJobLeaseLost(err) {
				cancelForControl()
			}
		}
	}
	onStart := func(segment replayVideoSegment) {
		stage := fmt.Sprintf("highlight_window_%d", segment.Index+1)
		s.setJob(videoKey, replayStatus{RunID: runID, JobID: jobID, State: "generating", ObjectKey: videoKey, Segments: total, SegmentsDone: done, Stage: stage})
		if jobID != "" {
			if err := s.heartbeatRenderJob(ctx, jobID, farm.MediaRenderJobRendering, stage, &total, &done); err != nil && mediaRenderJobLeaseLost(err) {
				cancelForControl()
			}
		}
	}
	for i := range attempts {
		if err := s.renderAttemptVideoSegments(ctx, runID, replayModeBroadcast, &attempts[i], onStart, onReady); err != nil {
			setError(fmt.Errorf("attempt %d highlights: %w", attempts[i].Recording.Attempt, err))
			return
		}
	}

	if jobID != "" {
		_ = s.heartbeatRenderJob(ctx, jobID, farm.MediaRenderJobAssembling, "highlight_reel", nil, nil)
	}
	urls, err := replayVideoSegmentURLs(s, attempts)
	if err != nil {
		setError(err)
		return
	}
	reelPath := pathJoinOS(dir, "highlights.mp4")
	s.encoderProcesses.Add(1)
	concatErr := concatReplaySegmentURLs(ctx, dir, urls, reelPath)
	s.encoderProcesses.Add(-1)
	if concatErr != nil {
		setError(concatErr)
		return
	}
	expected := replayAttemptsDuration(attempts)
	if err := probeReplayVideo(ctx, reelPath, expected); err != nil {
		setError(fmt.Errorf("validate highlight reel: %w", err))
		return
	}
	file, err := os.Open(reelPath)
	if err != nil {
		setError(err)
		return
	}
	if jobID != "" {
		_ = s.heartbeatRenderJob(ctx, jobID, farm.MediaRenderJobUploading, "highlight_upload", nil, nil)
	}
	obj, err := s.store.PutObjectReader(ctx, videoKey, "video/mp4", file)
	_ = file.Close()
	if err != nil {
		setError(err)
		return
	}

	manifest := highlightManifest{Version: 1, RunID: runID, Plan: plan, ObjectKey: videoKey}
	windowByIndex := make(map[int]mediahighlight.Window, len(plan.Windows))
	for _, window := range plan.Windows { windowByIndex[window.Index] = window }
	for _, attempt := range attempts {
		for _, segment := range attempt.Segments {
			window := windowByIndex[segment.Index]
			manifest.Clips = append(manifest.Clips, highlightManifestClip{
				Index: segment.Index, Attempt: segment.Attempt, StartFrame: segment.StartFrame, EndFrame: segment.EndFrame,
				ObjectKey: segment.CacheKey, Events: append([]mediahighlight.EventRef(nil), window.Events...),
			})
		}
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		setError(err)
		return
	}
	if _, err := s.store.PutObjectReader(ctx, manifestKey, "application/json", bytes.NewReader(raw)); err != nil {
		setError(fmt.Errorf("persist highlight manifest: %w", err))
		return
	}

	elapsed := time.Since(started)
	s.recordRenderSuccess(obj.Size, elapsed)
	s.setJob(videoKey, replayStatus{RunID: runID, JobID: jobID, State: "ready", ObjectKey: videoKey, Size: obj.Size, Segments: total, SegmentsDone: total, JobState: farm.MediaRenderJobReady, Stage: farm.MediaRenderJobReady})
	if jobID != "" {
		if err := s.finishRenderJob(context.Background(), jobID, farm.MediaRenderJobReady, farm.MediaRenderJobReady, "", obj.Size); err != nil {
			log.Printf("pokereplay: persist ready highlight job %s: %v", jobID, err)
		}
	}
	log.Printf("pokereplay highlights ok run=%s key=%s windows=%d duration=%s size=%d dur=%s", runID, videoKey, total, expected.Round(time.Millisecond), obj.Size, elapsed.Round(time.Millisecond))
}

var _ = compositor.BroadcastVersion
