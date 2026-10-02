package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultRTMPReconnectAttempts = 5
	defaultRTMPReconnectBase     = 250 * time.Millisecond
	defaultRTMPReconnectMax      = 4 * time.Second
	rtmpRequestLimit             = 64 << 10
	rtmpStopTimeout              = 3 * time.Second
	rtmpWriteTimeoutMicros       = 10_000_000
)

var errRTMPSourceEnded = errors.New("live media source ended")

type rtmpBroadcastRequest struct {
	Provider                string `json:"provider,omitempty"`
	Endpoint                string `json:"endpoint,omitempty"`
	StreamKey               string `json:"stream_key,omitempty"`
	Width                   int    `json:"width,omitempty"`
	Height                  int    `json:"height,omitempty"`
	FPS                     int    `json:"fps,omitempty"`
	VideoBitrateKbps        int    `json:"video_bitrate_kbps,omitempty"`
	Codec                   string `json:"codec,omitempty"`
	Preset                  string `json:"preset,omitempty"`
	KeyframeIntervalSeconds int    `json:"keyframe_interval_seconds,omitempty"`
	Audio                   *bool  `json:"audio,omitempty"`
	AudioBitrateKbps        int    `json:"audio_bitrate_kbps,omitempty"`
	AudioSampleRate         int    `json:"audio_sample_rate,omitempty"`
}

type rtmpBroadcastConfig struct {
	Provider                string
	Endpoint                string
	StreamKey               string
	Target                  string
	Host                    string
	Width                   int
	Height                  int
	FPS                     int
	VideoBitrateKbps        int
	Codec                   string
	Preset                  string
	KeyframeIntervalSeconds int
	Audio                   bool
	AudioBitrateKbps        int
	AudioSampleRate         int
}

type rtmpBroadcastStatus struct {
	RunID                   string `json:"run_id"`
	State                   string `json:"state"`
	Provider                string `json:"provider,omitempty"`
	Host                    string `json:"host,omitempty"`
	Width                   int    `json:"width,omitempty"`
	Height                  int    `json:"height,omitempty"`
	FPS                     int    `json:"fps,omitempty"`
	VideoBitrateKbps        int    `json:"video_bitrate_kbps,omitempty"`
	Codec                   string `json:"codec,omitempty"`
	Preset                  string `json:"preset,omitempty"`
	KeyframeIntervalSeconds int    `json:"keyframe_interval_seconds,omitempty"`
	Audio                   bool   `json:"audio"`
	AudioBitrateKbps        int    `json:"audio_bitrate_kbps,omitempty"`
	AudioSampleRate         int    `json:"audio_sample_rate,omitempty"`
	Reconnects              int    `json:"reconnects,omitempty"`
	LastError               string `json:"last_error,omitempty"`
	StartedAtMS             int64  `json:"started_at_unix_ms,omitempty"`
	UpdatedAtMS             int64  `json:"updated_at_unix_ms,omitempty"`
}

type rtmpProcess interface {
	StdinPipe() (io.WriteCloser, error)
	Start() error
	Wait() error
}

type rtmpProcessFactory func(context.Context, string, []string) rtmpProcess

type execRTMPProcess struct {
	cmd *exec.Cmd
}

func (p *execRTMPProcess) StdinPipe() (io.WriteCloser, error) { return p.cmd.StdinPipe() }
func (p *execRTMPProcess) Start() error                       { return p.cmd.Start() }
func (p *execRTMPProcess) Wait() error                        { return p.cmd.Wait() }

func defaultRTMPProcessFactory(ctx context.Context, binary string, args []string) rtmpProcess {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return &execRTMPProcess{cmd: cmd}
}

type rtmpBroadcastSession struct {
	runID  string
	config rtmpBroadcastConfig
	source *liveBroadcastSession

	processFactory rtmpProcessFactory
	maxReconnects  int
	backoffBase    time.Duration
	backoffMax     time.Duration

	mu     sync.Mutex
	status rtmpBroadcastStatus
	cancel context.CancelFunc
	done   chan struct{}
}

func normalizeRTMPBroadcastRequest(req rtmpBroadcastRequest) (rtmpBroadcastConfig, error) {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = "generic"
	}

	cfg := rtmpBroadcastConfig{
		Provider:                provider,
		Endpoint:                strings.TrimSpace(req.Endpoint),
		StreamKey:               strings.TrimSpace(req.StreamKey),
		Width:                   req.Width,
		Height:                  req.Height,
		FPS:                     req.FPS,
		VideoBitrateKbps:        req.VideoBitrateKbps,
		Codec:                   strings.TrimSpace(req.Codec),
		Preset:                  strings.TrimSpace(req.Preset),
		KeyframeIntervalSeconds: req.KeyframeIntervalSeconds,
		AudioBitrateKbps:        req.AudioBitrateKbps,
		AudioSampleRate:         req.AudioSampleRate,
	}

	switch provider {
	case "generic":
		if cfg.Endpoint == "" {
			return rtmpBroadcastConfig{}, errors.New("generic RTMP broadcast requires endpoint")
		}
		setRTMPDefaults(&cfg, 1280, 720, liveBroadcastFPS, 4000, false, 128, 48000)
	case "twitch":
		if cfg.Endpoint == "" {
			cfg.Endpoint = "rtmps://live.twitch.tv/app"
		}
		if cfg.StreamKey == "" {
			return rtmpBroadcastConfig{}, errors.New("Twitch broadcast requires stream_key")
		}
		setRTMPDefaults(&cfg, 1280, 720, 30, 4500, true, 160, 48000)
	case "youtube", "youtube_live":
		cfg.Provider = "youtube"
		if cfg.Endpoint == "" {
			cfg.Endpoint = "rtmps://a.rtmp.youtube.com/live2"
		}
		if cfg.StreamKey == "" {
			return rtmpBroadcastConfig{}, errors.New("YouTube Live broadcast requires stream_key")
		}
		setRTMPDefaults(&cfg, 1280, 720, 30, 4500, true, 128, 48000)
	default:
		return rtmpBroadcastConfig{}, errors.New("provider must be generic, twitch, or youtube")
	}
	if req.Audio != nil {
		cfg.Audio = *req.Audio
	}

	if err := validateRTMPDimensions(cfg.Width, cfg.Height); err != nil {
		return rtmpBroadcastConfig{}, err
	}
	if cfg.FPS < 1 || cfg.FPS > 60 {
		return rtmpBroadcastConfig{}, errors.New("fps must be between 1 and 60")
	}
	if cfg.VideoBitrateKbps < 250 || cfg.VideoBitrateKbps > 20000 {
		return rtmpBroadcastConfig{}, errors.New("video_bitrate_kbps must be between 250 and 20000")
	}
	if cfg.KeyframeIntervalSeconds < 1 || cfg.KeyframeIntervalSeconds > 10 {
		return rtmpBroadcastConfig{}, errors.New("keyframe_interval_seconds must be between 1 and 10")
	}
	if !validFFmpegAtom(cfg.Codec) {
		return rtmpBroadcastConfig{}, errors.New("codec contains unsupported characters")
	}
	if !validFFmpegAtom(cfg.Preset) {
		return rtmpBroadcastConfig{}, errors.New("preset contains unsupported characters")
	}
	if cfg.Audio {
		if cfg.AudioBitrateKbps < 32 || cfg.AudioBitrateKbps > 320 {
			return rtmpBroadcastConfig{}, errors.New("audio_bitrate_kbps must be between 32 and 320")
		}
		if cfg.AudioSampleRate < 8000 || cfg.AudioSampleRate > 96000 {
			return rtmpBroadcastConfig{}, errors.New("audio_sample_rate must be between 8000 and 96000")
		}
	}

	cfg.Target = cfg.Endpoint
	if cfg.StreamKey != "" {
		cfg.Target = strings.TrimRight(cfg.Endpoint, "/") + "/" + strings.TrimLeft(cfg.StreamKey, "/")
	}
	if len(cfg.Target) > 8192 {
		return rtmpBroadcastConfig{}, errors.New("RTMP target is too long")
	}
	parsed, err := url.Parse(cfg.Target)
	if err != nil || parsed.Host == "" {
		return rtmpBroadcastConfig{}, errors.New("invalid RTMP(S) endpoint")
	}
	if parsed.Scheme != "rtmp" && parsed.Scheme != "rtmps" {
		return rtmpBroadcastConfig{}, errors.New("endpoint scheme must be rtmp or rtmps")
	}
	if parsed.User != nil {
		return rtmpBroadcastConfig{}, errors.New("endpoint userinfo is not supported")
	}
	cfg.Host = parsed.Host
	return cfg, nil
}

func setRTMPDefaults(cfg *rtmpBroadcastConfig, width, height, fps, bitrate int, audio bool, audioBitrate, sampleRate int) {
	if cfg.Width == 0 {
		cfg.Width = width
	}
	if cfg.Height == 0 {
		cfg.Height = height
	}
	if cfg.FPS == 0 {
		cfg.FPS = fps
	}
	if cfg.VideoBitrateKbps == 0 {
		cfg.VideoBitrateKbps = bitrate
	}
	if cfg.Codec == "" {
		cfg.Codec = "libx264"
	}
	if cfg.Preset == "" {
		cfg.Preset = "veryfast"
	}
	if cfg.KeyframeIntervalSeconds == 0 {
		cfg.KeyframeIntervalSeconds = 2
	}
	cfg.Audio = audio
	if cfg.AudioBitrateKbps == 0 {
		cfg.AudioBitrateKbps = audioBitrate
	}
	if cfg.AudioSampleRate == 0 {
		cfg.AudioSampleRate = sampleRate
	}
}

func validateRTMPDimensions(width, height int) error {
	if width < 160 || width > 3840 || height < 144 || height > 2160 {
		return errors.New("broadcast dimensions must be between 160x144 and 3840x2160")
	}
	if width%2 != 0 || height%2 != 0 {
		return errors.New("broadcast dimensions must be even")
	}
	return nil
}

func validFFmpegAtom(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func rtmpFFmpegArgs(cfg rtmpBroadcastConfig) []string {
	gop := cfg.FPS * cfg.KeyframeIntervalSeconds
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "image2pipe", "-vcodec", "mjpeg", "-framerate", strconv.Itoa(liveBroadcastFPS), "-i", "pipe:0",
	}
	if cfg.Audio {
		args = append(args,
			"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate="+strconv.Itoa(cfg.AudioSampleRate),
		)
	}
	args = append(args,
		"-map", "0:v:0",
		"-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos", cfg.Width, cfg.Height),
		"-r", strconv.Itoa(cfg.FPS),
		"-c:v", cfg.Codec,
		"-preset", cfg.Preset,
		"-b:v", fmt.Sprintf("%dk", cfg.VideoBitrateKbps),
		"-maxrate", fmt.Sprintf("%dk", cfg.VideoBitrateKbps),
		"-bufsize", fmt.Sprintf("%dk", cfg.VideoBitrateKbps*2),
		"-g", strconv.Itoa(gop),
		"-keyint_min", strconv.Itoa(gop),
		"-sc_threshold", "0",
		"-pix_fmt", "yuv420p",
	)
	if cfg.Audio {
		args = append(args,
			"-map", "1:a:0",
			"-c:a", "aac",
			"-b:a", fmt.Sprintf("%dk", cfg.AudioBitrateKbps),
			"-ar", strconv.Itoa(cfg.AudioSampleRate),
			"-ac", "2",
			"-shortest",
		)
	} else {
		args = append(args, "-an")
	}
	args = append(args,
		"-rw_timeout", strconv.Itoa(rtmpWriteTimeoutMicros),
		"-f", "flv",
		cfg.Target,
	)
	return args
}

func newRTMPBroadcastSession(runID string, cfg rtmpBroadcastConfig, source *liveBroadcastSession) *rtmpBroadcastSession {
	now := time.Now().UnixMilli()
	return &rtmpBroadcastSession{
		runID:          runID,
		config:         cfg,
		source:         source,
		processFactory: defaultRTMPProcessFactory,
		maxReconnects:  defaultRTMPReconnectAttempts,
		backoffBase:    defaultRTMPReconnectBase,
		backoffMax:     defaultRTMPReconnectMax,
		status: rtmpBroadcastStatus{
			RunID:                   runID,
			State:                   "starting",
			Provider:                cfg.Provider,
			Host:                    cfg.Host,
			Width:                   cfg.Width,
			Height:                  cfg.Height,
			FPS:                     cfg.FPS,
			VideoBitrateKbps:        cfg.VideoBitrateKbps,
			Codec:                   cfg.Codec,
			Preset:                  cfg.Preset,
			KeyframeIntervalSeconds: cfg.KeyframeIntervalSeconds,
			Audio:                   cfg.Audio,
			AudioBitrateKbps:        cfg.AudioBitrateKbps,
			AudioSampleRate:         cfg.AudioSampleRate,
			StartedAtMS:             now,
			UpdatedAtMS:             now,
		},
		done: make(chan struct{}),
	}
}

func (s *rtmpBroadcastSession) snapshot() rtmpBroadcastStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *rtmpBroadcastSession) setState(state string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.State = state
	if err == nil {
		s.status.LastError = ""
	} else {
		s.status.LastError = sanitizeRTMPError(err, s.config)
	}
	s.status.UpdatedAtMS = time.Now().UnixMilli()
}

func (s *rtmpBroadcastSession) setReconnecting(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.State = "reconnecting"
	s.status.Reconnects++
	s.status.LastError = sanitizeRTMPError(err, s.config)
	s.status.UpdatedAtMS = time.Now().UnixMilli()
}

func (s *rtmpBroadcastSession) stop() {
	s.mu.Lock()
	if s.status.State != "failed" {
		s.status.State = "stopped"
		s.status.LastError = ""
		s.status.UpdatedAtMS = time.Now().UnixMilli()
	}
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *rtmpBroadcastSession) start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	go s.run(ctx)
}

func (s *rtmpBroadcastSession) run(ctx context.Context) {
	defer close(s.done)
	frames, unsubscribe := s.source.subscribe()
	defer unsubscribe()

	failures := 0
	for {
		err := s.runEncoderAttempt(ctx, frames)
		if errors.Is(err, context.Canceled) || errors.Is(err, errRTMPSourceEnded) {
			s.setState("stopped", nil)
			return
		}
		if err == nil {
			err = errors.New("RTMP encoder exited unexpectedly")
		}
		if failures >= s.maxReconnects {
			s.setState("failed", err)
			return
		}
		failures++
		s.setReconnecting(err)
		delay := rtmpReconnectDelay(failures, s.backoffBase, s.backoffMax)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			s.setState("stopped", nil)
			return
		case <-timer.C:
		}
	}
}

func (s *rtmpBroadcastSession) runEncoderAttempt(ctx context.Context, frames <-chan liveEncodedFrame) error {
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	process := s.processFactory(attemptCtx, "ffmpeg", rtmpFFmpegArgs(s.config))
	stdin, err := process.StdinPipe()
	if err != nil {
		return fmt.Errorf("open encoder input: %w", err)
	}
	if err := process.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start encoder: %w", err)
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- process.Wait()
	}()
	s.setState("healthy", nil)

	for {
		select {
		case <-ctx.Done():
			_ = stdin.Close()
			cancel()
			return context.Canceled
		case err := <-waitCh:
			_ = stdin.Close()
			if err == nil {
				return errors.New("encoder process exited")
			}
			return fmt.Errorf("encoder process failed: %w", err)
		case frame, ok := <-frames:
			if !ok {
				_ = stdin.Close()
				cancel()
				return errRTMPSourceEnded
			}
			if len(frame.jpeg) == 0 {
				continue
			}
			if _, err := stdin.Write(frame.jpeg); err != nil {
				cancel()
				_ = stdin.Close()
				return fmt.Errorf("encoder input failed: %w", err)
			}
		}
	}
}

func rtmpReconnectDelay(attempt int, base, max time.Duration) time.Duration {
	if attempt < 1 || base <= 0 {
		return 0
	}
	delay := base
	for i := 1; i < attempt && delay < max; i++ {
		delay *= 2
		if delay > max {
			return max
		}
	}
	if max > 0 && delay > max {
		return max
	}
	return delay
}

func sanitizeRTMPError(err error, cfg rtmpBroadcastConfig) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	for _, secret := range []string{cfg.Target, cfg.StreamKey, cfg.Endpoint} {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	if len(text) > 512 {
		text = text[:512]
	}
	return text
}

func (s *replayServer) startRTMPBroadcast(runID string, cfg rtmpBroadcastConfig) (*rtmpBroadcastSession, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, errors.New("run id is required")
	}

	s.rtmpMu.Lock()
	if existing := s.rtmpSessions[runID]; existing != nil {
		state := existing.snapshot().State
		if state != "stopped" && state != "failed" {
			s.rtmpMu.Unlock()
			return nil, errors.New("broadcast is already active")
		}
	}
	source := s.liveSession(runID)
	session := newRTMPBroadcastSession(runID, cfg, source)
	s.rtmpSessions[runID] = session
	s.rtmpMu.Unlock()

	session.start(context.Background())
	return session, nil
}

func (s *replayServer) rtmpBroadcastIfPresent(runID string) *rtmpBroadcastSession {
	s.rtmpMu.Lock()
	defer s.rtmpMu.Unlock()
	return s.rtmpSessions[strings.TrimSpace(runID)]
}

func (s *replayServer) stopRTMPBroadcasts() {
	s.rtmpMu.Lock()
	sessions := make([]*rtmpBroadcastSession, 0, len(s.rtmpSessions))
	for _, session := range s.rtmpSessions {
		sessions = append(sessions, session)
	}
	s.rtmpMu.Unlock()

	for _, session := range sessions {
		session.stop()
	}
	deadline := time.Now().Add(rtmpStopTimeout)
	for _, session := range sessions {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return
		}
		timer := time.NewTimer(remaining)
		select {
		case <-session.done:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			return
		}
	}
}

func (s *replayServer) handleRTMPBroadcastStart(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run id is required"})
		return
	}

	var req rtmpBroadcastRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, rtmpRequestLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid broadcast configuration"})
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid broadcast configuration"})
		return
	}

	cfg, err := normalizeRTMPBroadcastRequest(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	session, err := s.startRTMPBroadcast(runID, cfg)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, session.snapshot())
}

func (s *replayServer) handleRTMPBroadcastStatus(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run id is required"})
		return
	}
	if session := s.rtmpBroadcastIfPresent(runID); session != nil {
		writeJSON(w, http.StatusOK, session.snapshot())
		return
	}
	writeJSON(w, http.StatusOK, rtmpBroadcastStatus{RunID: runID, State: "stopped"})
}

func (s *replayServer) handleRTMPBroadcastStop(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run id is required"})
		return
	}
	session := s.rtmpBroadcastIfPresent(runID)
	if session == nil {
		writeJSON(w, http.StatusOK, rtmpBroadcastStatus{RunID: runID, State: "stopped"})
		return
	}
	session.stop()
	writeJSON(w, http.StatusOK, session.snapshot())
}
