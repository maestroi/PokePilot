package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func boolPtr(v bool) *bool { return &v }

func TestNormalizeRTMPBroadcastRequestPresetsAndValidation(t *testing.T) {
	twitch, err := normalizeRTMPBroadcastRequest(rtmpBroadcastRequest{
		Provider:  "twitch",
		StreamKey: "live_secret_key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if twitch.Provider != "twitch" || twitch.Host != "live.twitch.tv" {
		t.Fatalf("twitch config=%+v", twitch)
	}
	if twitch.Target != "rtmps://live.twitch.tv/app/live_secret_key" {
		t.Fatalf("twitch target=%q", twitch.Target)
	}
	if twitch.FPS != 30 || twitch.VideoBitrateKbps != 4500 || !twitch.Audio {
		t.Fatalf("twitch preset=%+v", twitch)
	}

	youtube, err := normalizeRTMPBroadcastRequest(rtmpBroadcastRequest{
		Provider:  "youtube",
		StreamKey: "yt-secret",
		Audio:     boolPtr(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	if youtube.Provider != "youtube" || youtube.Host != "a.rtmp.youtube.com" || youtube.Audio {
		t.Fatalf("youtube config=%+v", youtube)
	}

	generic, err := normalizeRTMPBroadcastRequest(rtmpBroadcastRequest{
		Endpoint:         "rtmp://relay.internal/live/custom-token",
		Width:            1920,
		Height:           1080,
		FPS:              25,
		VideoBitrateKbps: 6000,
		Codec:            "libx264",
		Preset:           "faster",
	})
	if err != nil {
		t.Fatal(err)
	}
	if generic.Provider != "generic" || generic.Host != "relay.internal" || generic.Audio {
		t.Fatalf("generic config=%+v", generic)
	}
	if generic.Width != 1920 || generic.Height != 1080 || generic.FPS != 25 || generic.VideoBitrateKbps != 6000 {
		t.Fatalf("generic overrides=%+v", generic)
	}

	for name, req := range map[string]rtmpBroadcastRequest{
		"http scheme": {Endpoint: "https://example.test/live/key"},
		"odd width":   {Endpoint: "rtmp://example.test/live/key", Width: 1279},
		"bad codec":   {Endpoint: "rtmp://example.test/live/key", Codec: "libx264;rm"},
		"missing key": {Provider: "twitch"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeRTMPBroadcastRequest(req); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestRTMPBroadcastRedactsEndpointAndStreamKey(t *testing.T) {
	cfg, err := normalizeRTMPBroadcastRequest(rtmpBroadcastRequest{
		Provider:  "twitch",
		Endpoint:  "rtmps://ingest.example.test/app",
		StreamKey: "super-secret-stream-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := errors.New("publish " + cfg.Target + " failed for key " + cfg.StreamKey + " at " + cfg.Endpoint)
	safe := sanitizeRTMPError(raw, cfg)
	for _, secret := range []string{cfg.Target, cfg.StreamKey, cfg.Endpoint} {
		if strings.Contains(safe, secret) {
			t.Fatalf("sanitized error leaked %q: %q", secret, safe)
		}
	}
	if !strings.Contains(safe, "[redacted]") {
		t.Fatalf("sanitized error=%q", safe)
	}

	status := newRTMPBroadcastSession("run-secret", cfg, newTestLiveSource("run-secret")).snapshot()
	encoded := strings.Join([]string{status.Provider, status.Host, status.LastError}, " ")
	if strings.Contains(encoded, cfg.StreamKey) || strings.Contains(encoded, cfg.Target) || strings.Contains(encoded, cfg.Endpoint) {
		t.Fatalf("status leaked secret target: %+v", status)
	}
}

func TestRTMPFFmpegArgsApplyConfiguredOutput(t *testing.T) {
	cfg, err := normalizeRTMPBroadcastRequest(rtmpBroadcastRequest{
		Provider:                "generic",
		Endpoint:                "rtmps://relay.example.test/live",
		StreamKey:               "secret",
		Width:                   1920,
		Height:                  1080,
		FPS:                     30,
		VideoBitrateKbps:        5500,
		Codec:                   "libx264",
		Preset:                  "fast",
		KeyframeIntervalSeconds: 2,
		Audio:                   boolPtr(true),
		AudioBitrateKbps:        192,
		AudioSampleRate:         48000,
	})
	if err != nil {
		t.Fatal(err)
	}
	args := rtmpFFmpegArgs(cfg)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"scale=1920:1080:flags=lanczos",
		"-r 30",
		"-b:v 5500k",
		"-g 60",
		"-c:a aac",
		"-b:a 192k",
		"-f flv",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("ffmpeg args missing %q: %s", want, joined)
		}
	}
	if got := args[len(args)-1]; got != cfg.Target {
		t.Fatalf("last arg=%q want target", got)
	}
}

type testRTMPWriter struct {
	onWrite func()
	closed  bool
	mu      sync.Mutex
}

func (w *testRTMPWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	closed := w.closed
	w.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}
	if w.onWrite != nil {
		w.onWrite()
	}
	return len(p), nil
}

func (w *testRTMPWriter) Close() error {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	return nil
}

type testRTMPProcess struct {
	ctx      context.Context
	startErr error
	waitCh   chan error
	writer   *testRTMPWriter
}

func newTestRTMPProcess(ctx context.Context, startErr error, failOnWrite error) *testRTMPProcess {
	p := &testRTMPProcess{
		ctx:      ctx,
		startErr: startErr,
		waitCh:   make(chan error, 1),
		writer:   &testRTMPWriter{},
	}
	if failOnWrite != nil {
		var once sync.Once
		p.writer.onWrite = func() {
			once.Do(func() { p.waitCh <- failOnWrite })
		}
	}
	return p
}

func (p *testRTMPProcess) StdinPipe() (io.WriteCloser, error) { return p.writer, nil }
func (p *testRTMPProcess) Start() error                       { return p.startErr }
func (p *testRTMPProcess) Wait() error {
	select {
	case err := <-p.waitCh:
		return err
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

func newTestLiveSource(runID string) *liveBroadcastSession {
	return &liveBroadcastSession{
		runID: runID,
		status: liveBroadcastStatus{
			RunID:     runID,
			State:     "live",
			TargetFPS: liveBroadcastFPS,
		},
		subscribers: make(map[uint64]chan liveEncodedFrame),
		done:        make(chan struct{}),
	}
}

func TestRTMPBroadcastReconnectsWithoutBackpressuringSourceAndStopsCleanly(t *testing.T) {
	cfg, err := normalizeRTMPBroadcastRequest(rtmpBroadcastRequest{
		Endpoint: "rtmp://relay.example.test/live/secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	source := newTestLiveSource("run-reconnect")
	session := newRTMPBroadcastSession("run-reconnect", cfg, source)
	session.backoffBase = time.Millisecond
	session.backoffMax = 2 * time.Millisecond
	session.maxReconnects = 3

	var mu sync.Mutex
	calls := 0
	session.processFactory = func(ctx context.Context, _ string, _ []string) rtmpProcess {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 1 {
			return newTestRTMPProcess(ctx, nil, errors.New("destination unavailable"))
		}
		return newTestRTMPProcess(ctx, nil, nil)
	}

	session.start(context.Background())
	source.publish(liveEncodedFrame{frame: 1, jpeg: []byte{1, 2, 3}})
	waitRTMPCondition(t, 2*time.Second, func() bool {
		mu.Lock()
		count := calls
		mu.Unlock()
		status := session.snapshot()
		return count >= 2 && status.State == "healthy" && status.Reconnects == 1
	})

	// Publishing a burst remains synchronous and bounded: the live source never
	// waits for this broadcaster, even while encoder/network work happens elsewhere.
	for frame := uint64(2); frame <= 100; frame++ {
		source.publish(liveEncodedFrame{frame: frame, jpeg: []byte{byte(frame)}})
	}

	session.stop()
	select {
	case <-session.done:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast did not stop cleanly")
	}
	if status := session.snapshot(); status.State != "stopped" {
		t.Fatalf("final status=%+v", status)
	}
}

func TestRTMPBroadcastPermanentEncoderFailureBecomesFailed(t *testing.T) {
	cfg, err := normalizeRTMPBroadcastRequest(rtmpBroadcastRequest{
		Endpoint: "rtmp://relay.example.test/live/secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	session := newRTMPBroadcastSession("run-failed", cfg, newTestLiveSource("run-failed"))
	session.backoffBase = time.Millisecond
	session.backoffMax = time.Millisecond
	session.maxReconnects = 2

	var mu sync.Mutex
	calls := 0
	session.processFactory = func(ctx context.Context, _ string, _ []string) rtmpProcess {
		mu.Lock()
		calls++
		mu.Unlock()
		return newTestRTMPProcess(ctx, errors.New("ffmpeg unavailable"), nil)
	}

	session.start(context.Background())
	select {
	case <-session.done:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast did not reach permanent failure")
	}

	status := session.snapshot()
	if status.State != "failed" || status.Reconnects != 2 {
		t.Fatalf("status=%+v", status)
	}
	if !strings.Contains(status.LastError, "ffmpeg unavailable") {
		t.Fatalf("last error=%q", status.LastError)
	}
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls != 3 {
		t.Fatalf("encoder attempts=%d want 3 (initial + 2 reconnects)", gotCalls)
	}
}

func waitRTMPCondition(t *testing.T, timeout time.Duration, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for RTMP broadcast condition")
}
