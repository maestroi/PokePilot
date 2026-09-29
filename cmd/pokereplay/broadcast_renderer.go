package main

import (
	"fmt"
	"path"
	"strings"

	"github.com/maestroi/pokepilot/media/compositor"
)

const broadcastRendererVersion = compositor.BroadcastVersion

type replayMode string

const (
	replayModeBroadcast replayMode = "broadcast"
	replayModeSemantic  replayMode = "semantic"
	replayModeRaw       replayMode = "raw"
)

func parseReplayMode(value string) (replayMode, error) {
	switch replayMode(strings.ToLower(strings.TrimSpace(value))) {
	case "", replayModeBroadcast:
		return replayModeBroadcast, nil
	case replayModeSemantic:
		return replayModeSemantic, nil
	case replayModeRaw:
		return replayModeRaw, nil
	default:
		return "", fmt.Errorf("unsupported replay mode %q (want broadcast, semantic, or raw)", value)
	}
}

// Keep the replay worker wired to package-level media contracts while the
// concrete compositor lives in media/compositor.
type replayCompositor = compositor.Compositor
type broadcastScene = compositor.Scene

func (s *replayServer) replayCacheKeyForMode(runID string, recordings []replayRecording, mode replayMode) string {
	version := broadcastRendererVersion
	switch mode {
	case replayModeSemantic:
		version = compositor.PublicSemanticRendererVersion()
		if s.semanticRenderer != nil {
			version = s.semanticRenderer.Version()
		}
	case replayModeBroadcast:
		if s.compositor != nil {
			version = s.compositor.Version()
		}
	}
	return replaySetCacheKeyForMode(runID, recordings, mode, version)
}

func replaySetCacheKeyForMode(runID string, recordings []replayRecording, mode replayMode, rendererVersion string) string {
	raw := replaySetCacheKey(runID, recordings)
	if mode == replayModeRaw {
		return raw
	}
	version := sanitizeKeySegment(rendererVersion)
	if version == "" {
		version = "broadcast"
	}
	ext := path.Ext(raw)
	return strings.TrimSuffix(raw, ext) + "-" + version + ext
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
