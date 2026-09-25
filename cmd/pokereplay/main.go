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
	redstarter "github.com/maestroi/pokepilot/red/starter"
	"golang.org/x/sync/errgroup"
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
}

type replayStatus struct {
	RunID     string `json:"run_id"`
	State     string `json:"state"`
	ObjectKey string `json:"object_key,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Error     string `json:"error,omitempty"`
	// Segments/SegmentsDone report per-attempt progress while generating.
	Segments     int `json:"segments,omitempty"`
	SegmentsDone int `json:"segments_done,omitempty"`
}

type replayIdentity struct {
	ROMSHA256 string
	Metadata  map[string]string
}

type replayServer struct {
	wallBase     string
	romPath      string
	streamBinary string
	vaapi        bool
	vaapiReason  string
	ffmpegVAAPI  string
	store        *artifactstore.S3
	wallHTTP     *http.Client
	compositor   replayCompositor

	mu   sync.Mutex
	jobs map[string]replayStatus // cache object key -> latest local render state

	parseRecording func([]byte) (replayIdentity, error)
	deriveROM      func([]byte, map[string]string, string) ([]byte, error)
}

func newReplayServer(wallBase, romPath, streamBinary string, store *artifactstore.S3) *replayServer {
	return &replayServer{
		wallBase:     strings.TrimRight(wallBase, "/"),
		romPath:      romPath,
		streamBinary: streamBinary,
		store:        store,
		wallHTTP:     &http.Client{Timeout: wallTimeout},
		compositor:   &ffmpegBroadcastCompositor{binary: "ffmpeg"},
		jobs:         make(map[string]replayStatus),
	}
}

func (s *replayServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":        "ok",
			"s3_configured": s.store != nil,
			"encoder":       s.encoderName(),
			"vaapi":         s.vaapi,
			"vaapi_reason":  s.vaapiReason,
			"renderer":      broadcastRendererVersion,
		})
	})
	mux.HandleFunc("GET /v1/runs/{id}/replay/status", s.handleReplayStatus)
	mux.HandleFunc("POST /v1/runs/{id}/replay/render", s.handleReplayRender)
	mux.HandleFunc("GET /v1/runs/{id}/replay/video", s.handleReplayVideo)
	mux.HandleFunc("GET /v1/runs/{id}/replay/semantic", s.handleReplaySemantic)
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
	recordings, err := s.recordings(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	status := s.replayStatus(r.Context(), runID, recordings, mode)
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
	if status.State == "ready" {
		writeJSON(w, http.StatusOK, status)
		return
	}
	cacheKey := s.replayCacheKeyForMode(runID, recordings, mode)
	s.mu.Lock()
	if current, ok := s.jobs[cacheKey]; ok && current.State == "generating" {
		s.mu.Unlock()
		writeJSON(w, http.StatusAccepted, current)
		return
	}
	status = replayStatus{RunID: runID, State: "generating", ObjectKey: cacheKey}
	s.jobs[cacheKey] = status
	s.mu.Unlock()

	go s.render(runID, recordings, cacheKey, mode)
	writeJSON(w, http.StatusAccepted, status)
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

func (s *replayServer) handleArtifactContent(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	name := r.PathValue("name")
	list, err := s.artifacts(r.Context(), runID)
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
		s.proxyInlineArtifact(w, r, runID, name)
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
	latest, err := s.artifacts(ctx, runID)
	if err != nil {
		return nil, err
	}
	latestAttempt := latest.Attempt
	if latestAttempt < 1 {
		latestAttempt = 1
	}
	if latestAttempt == 1 {
		artifact, ok := findArtifact(latest.Artifacts, "run.gbrun")
		if !ok || !artifact.Replayable {
			return nil, errRecordingNotFound
		}
		return []replayRecording{{Attempt: 1, Artifact: artifact}}, nil
	}

	recordings := make([]replayRecording, 0, latestAttempt)
	for attempt := 1; attempt <= latestAttempt; attempt++ {
		list, err := s.artifactsAttempt(ctx, runID, attempt)
		if errors.Is(err, errRunNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		artifact, ok := findArtifact(list.Artifacts, "run.gbrun")
		if !ok || !artifact.Replayable {
			continue
		}
		recordings = append(recordings, replayRecording{Attempt: attempt, Artifact: artifact})
	}
	if len(recordings) == 0 {
		return nil, errRecordingNotFound
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
	if obj, err := s.store.HeadObject(ctx, cacheKey); err == nil {
		return replayStatus{RunID: runID, State: "ready", ObjectKey: cacheKey, Size: obj.Size}
	} else if !artifactstore.IsNotFound(err) {
		return replayStatus{RunID: runID, State: "error", ObjectKey: cacheKey, Error: err.Error()}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if status, ok := s.jobs[cacheKey]; ok {
		return status
	}
	return replayStatus{RunID: runID, State: "missing", ObjectKey: cacheKey}
}

func (s *replayServer) render(runID string, recordings []replayRecording, cacheKey string, mode replayMode) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := time.Now()
	encoder := s.encoderName()
	log.Printf("pokereplay render start run=%s key=%s encoder=%s vaapi=%t segments=%d", runID, cacheKey, encoder, s.vaapi, len(recordings))
	setError := func(err error) {
		log.Printf("pokereplay render fail run=%s key=%s encoder=%s dur=%s err=%v", runID, cacheKey, encoder, time.Since(started).Round(time.Millisecond), err)
		s.setJob(cacheKey, replayStatus{RunID: runID, State: "error", ObjectKey: cacheKey, Error: clipError(err)})
	}

	dir, err := os.MkdirTemp("", "pokereplay-*")
	if err != nil {
		setError(err)
		return
	}
	defer os.RemoveAll(dir)
	semanticSegments := make([]semanticReplaySegment, len(recordings))
	for index, recording := range recordings {
		recordingPath := pathJoinOS(dir, fmt.Sprintf("segment-%03d.gbrun", index+1))
		if err := s.downloadRecording(ctx, runID, recording.Artifact, recordingPath, recording.Attempt); err != nil {
			setError(fmt.Errorf("attempt %d recording: %w", recording.Attempt, err))
			return
		}
		romPath, err := s.prepareStreamROM(dir, recordingPath)
		if err != nil {
			setError(fmt.Errorf("attempt %d replay ROM: %w", recording.Attempt, err))
			return
		}
		semanticSegments[index] = semanticReplaySegment{Attempt: recording.Attempt, RecordingPath: recordingPath, ReplayROMPath: romPath}
	}

	// Each attempt is rendered and cached in S3 under its own key, so a
	// restarted sidecar (every image roll recreates it) or a failed segment
	// only costs the segments still in flight: the next request reuses the rest.
	segmentVideos := make([]string, len(recordings))
	var done atomic.Int32
	g, gctx := errgroup.WithContext(ctx)
	for index, recording := range recordings {
		g.Go(func() error {
			video, err := s.renderCachedSegment(gctx, runID, recording, semanticSegments[index], dir, index, mode)
			if err != nil {
				return fmt.Errorf("attempt %d: %w", recording.Attempt, err)
			}
			segmentVideos[index] = video
			s.setJob(cacheKey, replayStatus{RunID: runID, State: "generating", ObjectKey: cacheKey, Segments: len(recordings), SegmentsDone: int(done.Add(1))})
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		setError(err)
		return
	}

	size := int64(0)
	if len(segmentVideos) > 1 {
		videoPath := pathJoinOS(dir, "replay.mp4")
		if err := concatReplaySegments(ctx, dir, segmentVideos, videoPath); err != nil {
			setError(err)
			return
		}
		file, err := os.Open(videoPath)
		if err != nil {
			setError(err)
			return
		}
		obj, err := s.store.PutObjectReader(ctx, cacheKey, "video/mp4", file)
		file.Close()
		if err != nil {
			setError(err)
			return
		}
		size = obj.Size
	} else if info, err := os.Stat(segmentVideos[0]); err == nil {
		// A single segment's cache key is the replay key; it is already uploaded.
		size = info.Size()
	}
	log.Printf("pokereplay render ok run=%s key=%s encoder=%s dur=%s size=%d", runID, cacheKey, encoder, time.Since(started).Round(time.Millisecond), size)
	s.setJob(cacheKey, replayStatus{RunID: runID, State: "ready", ObjectKey: cacheKey, Size: size})
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
	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()

	raw := video
	if mode == replayModeBroadcast {
		raw = pathJoinOS(dir, fmt.Sprintf("segment-%03d-raw.mp4", index+1))
	}
	if err := s.renderRecordingSegment(ctx, segment.ReplayROMPath, segment.RecordingPath, raw); err != nil {
		return "", err
	}
	if mode == replayModeBroadcast {
		if s.compositor == nil {
			return "", fmt.Errorf("broadcast compositor is not configured")
		}
		timeline, err := s.mediaTimelineOrEmpty(ctx, runID, recording.Attempt)
		if err != nil {
			return "", fmt.Errorf("media timeline: %w", err)
		}
		if err := s.compositor.Compose(ctx, broadcastScene{
			RunID:       runID,
			Attempt:     recording.Attempt,
			RawVideo:    raw,
			Destination: video,
			Timeline:    timeline,
			VAAPI:       s.vaapi,
			VAAPIDevice: vaapiDevice(),
		}); err != nil {
			return "", fmt.Errorf("broadcast renderer: %w", err)
		}
		_ = os.Remove(raw)
	}
	file, err := os.Open(video)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := s.store.PutObjectReader(ctx, key, "video/mp4", file); err != nil {
		return "", fmt.Errorf("cache segment: %w", err)
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
	output := &replayOutputTail{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gomeboy replay render: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
}

func concatReplaySegments(ctx context.Context, dir string, videos []string, destination string) error {
	var manifest strings.Builder
	for _, video := range videos {
		escaped := strings.ReplaceAll(video, "'", "'\\''")
		fmt.Fprintf(&manifest, "file '%s'\n", escaped)
	}
	manifestPath := pathJoinOS(dir, "segments.txt")
	if err := os.WriteFile(manifestPath, []byte(manifest.String()), 0o600); err != nil {
		return fmt.Errorf("write replay concat manifest: %w", err)
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "concat", "-safe", "0", "-i", manifestPath, "-c", "copy", "-movflags", "+faststart", destination)
	output := &replayOutputTail{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("concat replay segments: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
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

func (s *replayServer) proxyInlineArtifact(w http.ResponseWriter, r *http.Request, runID, name string) {
	endpoint := s.wallBase + "/v1/runs/" + url.PathEscape(runID) + "/artifacts/" + url.PathEscape(name) + "/content"
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
		// The existing render tests (and a corrupt download) still invoke
		// gomeboy-stream; let that command report the recording problem.
		return s.romPath, nil
	}
	base, err := os.ReadFile(s.romPath)
	if err != nil {
		return "", fmt.Errorf("read replay ROM: %w", err)
	}
	derive := s.deriveROM
	if derive == nil {
		derive = cartridgeForRecording
	}
	derived, err := derive(base, ident.Metadata, ident.ROMSHA256)
	if err != nil {
		return "", err
	}
	if bytes.Equal(derived, base) {
		return s.romPath, nil
	}
	path := pathJoinOS(workDir, "replay.gb")
	if err := os.WriteFile(path, derived, 0o644); err != nil {
		return "", err
	}
	return path, nil
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
	romPath := flag.String("rom", "/rom/pokemon_red.gb", "ROM path used for deterministic replay")
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

	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	log.Printf("pokereplay listening on http://%s (wall %s, s3=%t, encoder=%s, vaapi=%t, %s)", *httpAddr, *wallBase, store != nil, encoder, on, reason)
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("pokereplay: server stopped: %v", err)
		}
	case <-ctx.Done():
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
