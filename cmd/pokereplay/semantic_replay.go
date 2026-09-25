package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
	redrenderstate "github.com/maestroi/pokepilot/red/renderstate"
	protocol "github.com/maestroi/pokepilot/renderstate"
)

const semanticReplaySampleEveryFrames uint64 = 6

type semanticReplaySegment struct {
	Attempt       int
	RecordingPath string
	ReplayROMPath string
}

func semanticReplayCacheKey(runID string, recordings []replayRecording) string {
	base := replaySetCacheKey(runID, recordings)
	dir := path.Dir(base)
	h := sha256.New()
	fmt.Fprintf(h, "semantic-replay-v%d\n", protocol.ReplayTimelineVersion)
	for _, recording := range recordings {
		fmt.Fprintf(h, "%d:%s:%s\n", recording.Attempt, strings.ToLower(strings.TrimSpace(recording.Artifact.SHA256)), strings.TrimSpace(recording.Artifact.ObjectKey))
	}
	fingerprint := hex.EncodeToString(h.Sum(nil))
	if len(fingerprint) > 12 {
		fingerprint = fingerprint[:12]
	}
	return path.Join(dir, fmt.Sprintf("semantic-replay-v%d-%s.json", protocol.ReplayTimelineVersion, fingerprint))
}

func replayFrameTimeMS(frames uint64) int64 {
	return int64(math.Round(float64(frames) * 1000 / farm.GameBoyFramesPerSecond))
}

func replaySegmentDurationMS(recording *gomeboy.Recording) int64 {
	if recording == nil || recording.EndFrame < recording.StartFrame {
		return 0
	}
	// gomeboy-stream writes the restored initial frame plus every stepped
	// frame, so the encoded segment contains delta+1 video frames.
	return replayFrameTimeMS(recording.EndFrame - recording.StartFrame + 1)
}

func (s *replayServer) renderSemanticReplay(ctx context.Context, runID string, recordings []replayRecording, segments []semanticReplaySegment) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("semantic replay storage is not configured")
	}
	if len(recordings) == 0 || len(segments) != len(recordings) {
		return fmt.Errorf("semantic replay segments do not match recordings")
	}

	semanticROM, err := os.ReadFile(s.romPath)
	if err != nil {
		return fmt.Errorf("read semantic replay ROM: %w", err)
	}
	producer, err := redrenderstate.New(semanticROM)
	if err != nil {
		// The modern semantic renderer currently has a rich Red adapter. Other
		// games keep their existing framebuffer replay until they gain one.
		return err
	}

	builder := protocol.NewReplayTimelineBuilder(runID, farm.GameBoyFramesPerSecond)
	var offsetMS int64
	for _, segment := range segments {
		duration, err := appendSemanticReplaySegment(builder, producer, segment, offsetMS)
		if err != nil {
			return fmt.Errorf("attempt %d semantic replay: %w", segment.Attempt, err)
		}
		offsetMS += duration
	}
	timeline, err := builder.Build(offsetMS)
	if err != nil {
		return err
	}
	if len(timeline.Samples) == 0 {
		return fmt.Errorf("semantic replay produced no samples")
	}
	data, err := json.Marshal(timeline)
	if err != nil {
		return fmt.Errorf("encode semantic replay: %w", err)
	}
	key := semanticReplayCacheKey(runID, recordings)
	obj, err := s.store.PutObject(ctx, key, "application/json", data)
	if err != nil {
		return err
	}
	log.Printf("pokereplay semantic replay ok run=%s key=%s samples=%d layer_sets=%d size=%d", runID, obj.Key, len(timeline.Samples), len(timeline.LayerSets), obj.Size)
	return nil
}

func appendSemanticReplaySegment(builder *protocol.ReplayTimelineBuilder, producer *redrenderstate.Producer, segment semanticReplaySegment, offsetMS int64) (int64, error) {
	recording, err := gomeboy.LoadRecording(segment.RecordingPath)
	if err != nil {
		return 0, err
	}
	replayROM, err := os.ReadFile(segment.ReplayROMPath)
	if err != nil {
		return 0, err
	}
	replay, err := gomeboy.New(
		gomeboy.WithROMBytes(replayROM),
		gomeboy.WithModel(recording.Model),
		gomeboy.Headless(),
		gomeboy.WithoutVideo(),
	)
	if err != nil {
		return 0, err
	}
	defer replay.Close()

	captured := 0
	var lastSnapshotErr error
	err = replay.ReplayRecordingFrames(recording, func(frame uint64, _ gomeboy.Frame) error {
		if frame < recording.StartFrame {
			return nil
		}
		relative := frame - recording.StartFrame
		if relative%semanticReplaySampleEveryFrames != 0 && frame != recording.EndFrame {
			return nil
		}
		state, snapshotErr := producer.Snapshot(replay, protocol.FrameMeta{
			Frame: frame,
			Cycle: replay.Cycle(),
		})
		if snapshotErr != nil {
			// Live rendering already treats undecodable transient map buffers as
			// a compatibility-frame window. Preserve that behavior in replay by
			// skipping only the bad sample rather than invalidating the .gbrun.
			lastSnapshotErr = snapshotErr
			return nil
		}
		state.Clock.CapturedAtUnixMS = 0
		if err := builder.Append(offsetMS+replayFrameTimeMS(relative), segment.Attempt, state); err != nil {
			return err
		}
		captured++
		return nil
	})
	if err != nil {
		return 0, err
	}
	if captured == 0 {
		if lastSnapshotErr != nil {
			return 0, lastSnapshotErr
		}
		return 0, fmt.Errorf("no semantic samples captured")
	}
	return replaySegmentDurationMS(recording), nil
}

func (s *replayServer) handleReplaySemantic(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("id"))
	recordings, err := s.recordings(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "semantic replay storage is not configured"})
		return
	}
	key := semanticReplayCacheKey(runID, recordings)
	obj, err := s.store.GetObject(r.Context(), key, "")
	if err != nil {
		if artifactstore.IsNotFound(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "semantic replay is not available for this recording"})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer obj.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	if obj.ContentLength > 0 {
		w.Header().Set("Content-Length", fmt.Sprint(obj.ContentLength))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, obj.Body)
}
