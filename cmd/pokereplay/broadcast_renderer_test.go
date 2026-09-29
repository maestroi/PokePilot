package main

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/media/compositor"
)

func TestReplayCacheKeyVersionsBroadcastWithoutChangingRaw(t *testing.T) {
	recordings := []replayRecording{{
		Attempt: 1,
		Artifact: artifactRef{
			SHA256:    strings.Repeat("a", 64),
			ObjectKey: "runs/run-1/attempt-1/run.gbrun",
		},
	}}
	raw := replaySetCacheKey("run-1", recordings)
	if got := replaySetCacheKeyForMode("run-1", recordings, replayModeRaw, broadcastRendererVersion); got != raw {
		t.Fatalf("raw key=%q, want legacy %q", got, raw)
	}
	broadcast := replaySetCacheKeyForMode("run-1", recordings, replayModeBroadcast, broadcastRendererVersion)
	if broadcast == raw || !strings.Contains(broadcast, broadcastRendererVersion) {
		t.Fatalf("broadcast key=%q raw=%q", broadcast, raw)
	}
	if got := replaySetCacheKeyForMode("run-1", recordings, replayModeBroadcast, "broadcast-1280x720-v2"); got == broadcast {
		t.Fatalf("renderer version did not change cache key: %q", got)
	}
	semantic := replaySetCacheKeyForMode("run-1", recordings, replayModeSemantic, compositor.PublicSemanticRendererVersion())
	if semantic == raw || semantic == broadcast || !strings.Contains(semantic, "semantic-renderstate") || !strings.Contains(semantic, "kenney-tiny-town") {
		t.Fatalf("semantic key=%q broadcast=%q raw=%q", semantic, broadcast, raw)
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
