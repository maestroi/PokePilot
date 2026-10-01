package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/media/compositor"
)

type versionedReplayCompositor struct{ version string }

func (c versionedReplayCompositor) Version() string { return c.version }
func (versionedReplayCompositor) Compose(context.Context, compositor.Scene) error {
	return nil
}

func testReplayRecording() replayRecording {
	return replayRecording{
		Attempt: 1,
		Artifact: artifactRef{
			SHA256:    strings.Repeat("a", 64),
			ObjectKey: "runs/run-1/attempt-1/run.gbrun",
		},
		Timeline: artifactRef{
			SHA256:    strings.Repeat("b", 64),
			ObjectKey: "runs/run-1/attempt-1/media-timeline.json",
		},
	}
}

func TestReplayCacheKeyUsesCanonicalMaterialIdentity(t *testing.T) {
	recording := testReplayRecording()
	replay := newReplayServer("http://wall.invalid", "", "", nil)
	maxFrames := uint64(12345)

	broadcast := replay.replayCacheKeyForModeWithSegmentFrames("run-1", []replayRecording{recording}, replayModeBroadcast, maxFrames)
	if !strings.Contains(broadcast, "replay-v1-") || !strings.HasSuffix(broadcast, ".mp4") {
		t.Fatalf("broadcast key=%q, want canonical identity-v1 key", broadcast)
	}
	if got := replay.replayCacheKeyForModeWithSegmentFrames("run-1", []replayRecording{recording}, replayModeBroadcast, maxFrames); got != broadcast {
		t.Fatalf("identical request changed key: got %q want %q", got, broadcast)
	}

	changedTimeline := recording
	changedTimeline.Timeline.SHA256 = strings.Repeat("c", 64)
	if got := replay.replayCacheKeyForModeWithSegmentFrames("run-1", []replayRecording{changedTimeline}, replayModeBroadcast, maxFrames); got == broadcast {
		t.Fatalf("timeline content did not change identity: %q", got)
	}

	replay.compositor = versionedReplayCompositor{version: "broadcast-layout-v2"}
	if got := replay.replayCacheKeyForModeWithSegmentFrames("run-1", []replayRecording{recording}, replayModeBroadcast, maxFrames); got == broadcast {
		t.Fatalf("layout/compositor version did not change identity: %q", got)
	}
	replay.compositor = compositor.NewFFmpeg("ffmpeg", nil)
	replay.vaapi = true
	if got := replay.replayCacheKeyForModeWithSegmentFrames("run-1", []replayRecording{recording}, replayModeBroadcast, maxFrames); got == broadcast {
		t.Fatalf("encoder policy did not change identity: %q", got)
	}
	replay.vaapi = false
	if got := replay.replayCacheKeyForModeWithSegmentFrames("run-1", []replayRecording{recording}, replayModeBroadcast, maxFrames+1); got == broadcast {
		t.Fatalf("segment policy did not change identity: %q", got)
	}

	raw := replay.replayCacheKeyForModeWithSegmentFrames("run-1", []replayRecording{recording}, replayModeRaw, maxFrames)
	semantic := replay.replayCacheKeyForModeWithSegmentFrames("run-1", []replayRecording{recording}, replayModeSemantic, maxFrames)
	if raw == broadcast || semantic == broadcast || raw == semantic {
		t.Fatalf("render profiles aliased: raw=%q broadcast=%q semantic=%q", raw, broadcast, semantic)
	}
}

func TestLegacyReplayKeyRemainsLookupOnly(t *testing.T) {
	recordings := []replayRecording{testReplayRecording()}
	replay := newReplayServer("http://wall.invalid", "", "", nil)

	legacyRaw := replaySetCacheKey("run-1", recordings)
	if got := replay.legacyReplayCacheKeyForMode("run-1", recordings, replayModeRaw); got != legacyRaw {
		t.Fatalf("legacy raw key=%q want=%q", got, legacyRaw)
	}
	if canonical := replay.replayCacheKeyForModeWithSegmentFrames("run-1", recordings, replayModeRaw, 12345); canonical == legacyRaw {
		t.Fatalf("canonical identity silently aliases legacy raw key %q", canonical)
	}

	legacyBroadcast := replaySetCacheKeyForMode("run-1", recordings, replayModeBroadcast, broadcastRendererVersion)
	if got := replay.legacyReplayCacheKeyForMode("run-1", recordings, replayModeBroadcast); got != legacyBroadcast {
		t.Fatalf("legacy broadcast key=%q want=%q", got, legacyBroadcast)
	}
}

func TestReplayStatusExposesLegacyArtifactForMigration(t *testing.T) {
	recordings := []replayRecording{testReplayRecording()}
	legacyKey := replaySetCacheKeyForMode("run-1", recordings, replayModeBroadcast, broadcastRendererVersion)

	s3srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/pokepilot/"+legacyKey {
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer s3srv.Close()
	store, err := artifactstore.NewS3(artifactstore.S3Config{
		Endpoint: s3srv.URL, Bucket: "pokepilot", Region: "us-east-1",
		AccessKey: "test", SecretKey: "secret", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	wall := httptest.NewServer(http.NotFoundHandler())
	defer wall.Close()
	replay := newReplayServer(wall.URL, "", "", store)

	status := replay.replayStatus(context.Background(), "run-1", recordings, replayModeBroadcast)
	if status.State != "ready" || status.ObjectKey != legacyKey || !status.LegacyIdentity {
		t.Fatalf("legacy status=%+v", status)
	}
	if canonical := replay.replayCacheKeyForMode("run-1", recordings, replayModeBroadcast); canonical == status.ObjectKey {
		t.Fatalf("legacy artifact reported as canonical: %+v", status)
	}
}

func TestParseReplayModeDefaultsToBroadcastAndKeepsRawFallback(t *testing.T) {
	if got, err := parseReplayMode(""); err != nil || got != replayModeBroadcast {
		t.Fatalf("default mode=%q err=%v", got, err)
	}
	if got, err := parseReplayMode("semantic"); err != nil || got != replayModeSemantic {
		t.Fatalf("semantic mode=%q err=%v", got, err)
	}
	if got, err := parseReplayMode("raw"); err != nil || got != replayModeRaw {
		t.Fatalf("raw mode=%q err=%v", got, err)
	}
	if _, err := parseReplayMode("unknown"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}
