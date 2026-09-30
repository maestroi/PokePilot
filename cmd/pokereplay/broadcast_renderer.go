package main

import (
	"fmt"
	"path"
	"strings"

	"github.com/maestroi/pokepilot/farm"
	mediaartifact "github.com/maestroi/pokepilot/media/artifact"
	"github.com/maestroi/pokepilot/media/compositor"
	protocol "github.com/maestroi/pokepilot/renderstate"
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
	return s.replayCacheKeyForModeWithSegmentFrames(runID, recordings, mode, replaySegmentFrames())
}

func (s *replayServer) replayCacheKeyForModeWithSegmentFrames(runID string, recordings []replayRecording, mode replayMode, maxFrames uint64) string {
	identity := s.replayIdentityInput(runID, recordings, mode, maxFrames)
	base := replaySetCacheKey(runID, recordings)
	return path.Join(path.Dir(base), "replay-"+mediaartifact.Token(identity)+".mp4")
}

func (s *replayServer) replayIdentityInput(runID string, recordings []replayRecording, mode replayMode, maxFrames uint64) mediaartifact.IdentityInput {
	input := mediaartifact.IdentityInput{Profile: s.replayProfileForMode(mode, maxFrames)}
	for _, recording := range recordings {
		input.Sources = append(input.Sources, mediaartifact.Source{
			Attempt:    recording.Attempt,
			SHA256:     recording.Artifact.SHA256,
			FallbackID: recording.Artifact.ObjectKey,
		})
		switch mode {
		case replayModeBroadcast:
			timeline := mediaartifact.Content{
				Attempt:    recording.Attempt,
				Version:    fmt.Sprintf("media-timeline-v%d", farm.MediaTimelineVersion),
				SHA256:     recording.Timeline.SHA256,
				FallbackID: recording.Timeline.ObjectKey,
			}
			if strings.TrimSpace(timeline.SHA256) == "" && strings.TrimSpace(timeline.FallbackID) == "" {
				empty := farm.MediaTimeline{
					Run:             farm.MediaRunSummary{RunID: runID},
					Attempt:         recording.Attempt,
					FramesPerSecond: farm.GameBoyFramesPerSecond,
				}.Normalized()
				if artifact, err := farm.NewMediaTimelineArtifact(empty); err == nil {
					timeline.SHA256 = artifact.SHA256
				}
			}
			input.Timelines = append(input.Timelines, timeline)
		case replayModeSemantic:
			input.RenderStates = append(input.RenderStates, mediaartifact.Content{
				Attempt: recording.Attempt,
				Version: fmt.Sprintf(
					"renderstate-v%d-replay-timeline-v%d-sample-f%d",
					protocol.SchemaVersion,
					protocol.ReplayTimelineVersion,
					semanticReplaySampleEveryFrames,
				),
			})
		}
	}
	return input
}

func (s *replayServer) replayProfileForMode(mode replayMode, maxFrames uint64) mediaartifact.Profile {
	name := mediaartifact.ProfileReplayFull
	if mode == replayModeRaw {
		name = mediaartifact.ProfileDebugRaw
	}
	profile, err := mediaartifact.NewProfile(name)
	if err != nil {
		panic(err)
	}
	profile.SegmentPolicy = mediaartifact.Component{
		ID:      "bounded",
		Version: fmt.Sprintf("v%d-f%d", replaySegmentPlanVersion, maxFrames),
	}
	profile.Encoder.Codec = s.encoderName()
	if s.vaapi {
		profile.Encoder.Policy = "h264-vaapi-default-v1"
	} else {
		profile.Encoder.Policy = "libx264-veryfast-crf20-v1"
	}

	switch mode {
	case replayModeBroadcast:
		version := broadcastRendererVersion
		if s.compositor != nil {
			version = s.compositor.Version()
		}
		profile.Presentation.Renderer = mediaartifact.Component{ID: "classic-framebuffer", Version: "gomeboy-rgb-v1"}
		profile.Presentation.Layout = mediaartifact.Component{ID: "pokepilot-broadcast", Version: version}
		profile.OverlayPolicy = fmt.Sprintf("media-timeline-v%d", farm.MediaTimelineVersion)
		profile.EventPolicy = "broadcast-event-cards-v1"
	case replayModeSemantic:
		version := compositor.PublicSemanticRendererVersion()
		if s.semanticRenderer != nil {
			version = s.semanticRenderer.Version()
		}
		profile.Presentation.Renderer = mediaartifact.Component{ID: "renderstate", Version: version}
		profile.Presentation.Theme = mediaartifact.Component{ID: compositor.SemanticThemeID, Version: compositor.PublicSemanticThemeVersion()}
		profile.Presentation.Layout = mediaartifact.Component{ID: "semantic-full-frame", Version: "v1"}
		profile.OverlayPolicy = "none"
		profile.EventPolicy = "semantic-scenes-v1"
	case replayModeRaw:
		profile.Presentation.Renderer = mediaartifact.Component{ID: "classic-framebuffer", Version: "gomeboy-stream-v1"}
		profile.Presentation.Layout = mediaartifact.Component{ID: "raw-frame", Version: "v1"}
		profile.OverlayPolicy = "none"
		profile.EventPolicy = "none"
	default:
		panic(fmt.Sprintf("unsupported replay mode %q", mode))
	}
	return mediaartifact.NormalizeProfile(profile)
}

func (s *replayServer) legacyReplayCacheKeyForMode(runID string, recordings []replayRecording, mode replayMode) string {
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

// replaySetCacheKeyForMode is the pre-profile cache identity. Keep it only for
// explicit backward-compatible lookup of artifacts produced before identity-v1.
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
