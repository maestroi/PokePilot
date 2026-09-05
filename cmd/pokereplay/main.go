// Command pokereplay is the read/render sidecar for durable PokePilot run
// artifacts. It never owns run metadata: pokewall is the catalog and S3 is
// the blob store. This process only resolves a run's artifact references,
// renders deterministic .gbrun recordings through GomeBoy, caches derived
// MP4s back into S3, and streams artifact/video bytes to the private UI.
package main

import (
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
	"syscall"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
)

const (
	serverReadHeaderTimeout = 5 * time.Second
	serverIdleTimeout       = 60 * time.Second
	serverShutdownTimeout   = 10 * time.Second
	wallTimeout             = 30 * time.Second
	renderTimeout           = 2 * time.Hour
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

type replayStatus struct {
	RunID     string `json:"run_id"`
	State     string `json:"state"`
	ObjectKey string `json:"object_key,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Error     string `json:"error,omitempty"`
}

type replayServer struct {
	wallBase     string
	romPath      string
	streamBinary string
	vaapi        bool
	ffmpegVAAPI  string
	store        *artifactstore.S3
	wallHTTP     *http.Client

	mu   sync.Mutex
	jobs map[string]replayStatus // cache object key -> latest local render state
}

func newReplayServer(wallBase, romPath, streamBinary string, store *artifactstore.S3) *replayServer {
	return &replayServer{
		wallBase:     strings.TrimRight(wallBase, "/"),
		romPath:      romPath,
		streamBinary: streamBinary,
		store:        store,
		wallHTTP:     &http.Client{Timeout: wallTimeout},
		jobs:         make(map[string]replayStatus),
	}
}

func (s *replayServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "s3_configured": s.store != nil})
	})
	mux.HandleFunc("GET /v1/runs/{id}/replay/status", s.handleReplayStatus)
	mux.HandleFunc("POST /v1/runs/{id}/replay/render", s.handleReplayRender)
	mux.HandleFunc("GET /v1/runs/{id}/replay/video", s.handleReplayVideo)
	mux.HandleFunc("GET /v1/runs/{id}/artifacts/{name}/content", s.handleArtifactContent)
	return mux
}

func (s *replayServer) handleReplayStatus(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	recording, err := s.recording(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	status := s.replayStatus(r.Context(), runID, recording)
	writeJSON(w, http.StatusOK, status)
}

func (s *replayServer) handleReplayRender(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	recording, err := s.recording(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, replayStatus{RunID: runID, State: "disabled", Error: "S3 artifact storage is not configured for the replay service"})
		return
	}
	status := s.replayStatus(r.Context(), runID, recording)
	if status.State == "ready" {
		writeJSON(w, http.StatusOK, status)
		return
	}
	cacheKey := replayCacheKey(runID, recording)
	s.mu.Lock()
	if current, ok := s.jobs[cacheKey]; ok && current.State == "generating" {
		s.mu.Unlock()
		writeJSON(w, http.StatusAccepted, current)
		return
	}
	status = replayStatus{RunID: runID, State: "generating", ObjectKey: cacheKey}
	s.jobs[cacheKey] = status
	s.mu.Unlock()

	go s.render(runID, recording, cacheKey)
	writeJSON(w, http.StatusAccepted, status)
}

func (s *replayServer) handleReplayVideo(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	recording, err := s.recording(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	status := s.replayStatus(r.Context(), runID, recording)
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

func (s *replayServer) artifacts(ctx context.Context, runID string) (artifactList, error) {
	var out artifactList
	if strings.TrimSpace(runID) == "" {
		return out, errRunNotFound
	}
	endpoint := s.wallBase + "/v1/runs/" + url.PathEscape(runID) + "/artifacts"
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

func (s *replayServer) replayStatus(ctx context.Context, runID string, recording artifactRef) replayStatus {
	if s.store == nil {
		return replayStatus{RunID: runID, State: "disabled", Error: "S3 artifact storage is not configured for the replay service"}
	}
	cacheKey := replayCacheKey(runID, recording)
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

func (s *replayServer) render(runID string, recording artifactRef, cacheKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()
	setError := func(err error) {
		s.setJob(cacheKey, replayStatus{RunID: runID, State: "error", ObjectKey: cacheKey, Error: clipError(err)})
	}

	dir, err := os.MkdirTemp("", "pokereplay-*")
	if err != nil {
		setError(err)
		return
	}
	defer os.RemoveAll(dir)
	recordingPath := pathJoinOS(dir, "run.gbrun")
	videoPath := pathJoinOS(dir, "replay.mp4")
	if err := s.downloadRecording(ctx, runID, recording, recordingPath); err != nil {
		setError(err)
		return
	}

	cmd := exec.CommandContext(ctx, s.streamBinary, s.streamArgs(recordingPath, videoPath)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		setError(fmt.Errorf("gomeboy replay render: %w: %s", err, strings.TrimSpace(string(output))))
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
	s.setJob(cacheKey, replayStatus{RunID: runID, State: "ready", ObjectKey: obj.Key, Size: obj.Size})
}

func (s *replayServer) downloadRecording(ctx context.Context, runID string, recording artifactRef, destination string) error {
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer file.Close()
	h := sha256.New()
	writer := io.MultiWriter(file, h)

	if recording.Store == "" {
		endpoint := s.wallBase + "/v1/runs/" + url.PathEscape(runID) + "/artifacts/" + url.PathEscape(recording.Name) + "/content"
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
	serverImpl.vaapi = detectVAAPI()
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
	log.Printf("pokereplay listening on http://%s (wall %s, s3=%t, vaapi=%t)", *httpAddr, *wallBase, store != nil, serverImpl.vaapi)
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
