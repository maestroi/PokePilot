package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
)

const (
	replaySegmentPlanVersion    = 1
	defaultReplaySegmentSeconds = 300
	replaySegmentSecondsEnv     = "POKEPILOT_REPLAY_SEGMENT_SECONDS"
	minReplaySegmentSeconds     = 10
	maxReplaySegmentSeconds     = 3600
	replaySegmentURLTTL          = 12 * time.Hour
)

type replayVideoSegment struct {
	Attempt    int
	Index      int
	StartFrame uint64 // relative to recording start, inclusive
	EndFrame   uint64 // relative to recording start, inclusive
	CacheKey   string
	LocalPath  string
	Cached     bool
}

type preparedReplayAttempt struct {
	Recording replayRecording
	Semantic  semanticReplaySegment
	Parsed    *gomeboy.Recording
	Segments  []replayVideoSegment
}

func replaySegmentFrames() uint64 {
	seconds := defaultReplaySegmentSeconds
	if raw := strings.TrimSpace(os.Getenv(replaySegmentSecondsEnv)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= minReplaySegmentSeconds && parsed <= maxReplaySegmentSeconds {
			seconds = parsed
		} else {
			log.Printf("pokereplay: ignoring invalid %s=%q (want %d..%d seconds)", replaySegmentSecondsEnv, raw, minReplaySegmentSeconds, maxReplaySegmentSeconds)
		}
	}
	frames := uint64(float64(seconds)*farm.GameBoyFramesPerSecond + 0.5)
	if frames == 0 {
		return 1
	}
	return frames
}

func planReplayVideoSegments(attempt int, recording *gomeboy.Recording, maxFrames uint64) []replayVideoSegment {
	if recording == nil {
		return nil
	}
	if maxFrames == 0 {
		maxFrames = 1
	}
	totalFrames := recording.DurationFrames() + 1 // restored initial frame + every stepped frame
	segments := make([]replayVideoSegment, 0, (totalFrames+maxFrames-1)/maxFrames)
	for start := uint64(0); start < totalFrames; start += maxFrames {
		end := start + maxFrames - 1
		if end >= totalFrames {
			end = totalFrames - 1
		}
		segments = append(segments, replayVideoSegment{
			Attempt:    attempt,
			Index:      len(segments),
			StartFrame: start,
			EndFrame:   end,
		})
	}
	return segments
}

func (s *replayServer) replayVideoSegmentCacheKey(runID string, recording replayRecording, mode replayMode, maxFrames uint64, segment replayVideoSegment) string {
	base := s.replayCacheKeyForMode(runID, []replayRecording{recording}, mode)
	stem := strings.TrimSuffix(path.Base(base), path.Ext(base))
	return path.Join(
		path.Dir(base),
		fmt.Sprintf("segments-v%d-f%d", replaySegmentPlanVersion, maxFrames),
		stem,
		fmt.Sprintf("%05d-f%d-%d.mp4", segment.Index, segment.StartFrame, segment.EndFrame),
	)
}

func (s *replayServer) prepareReplayAttempt(runID string, recording replayRecording, semantic semanticReplaySegment, dir string, maxFrames uint64) (preparedReplayAttempt, error) {
	parsed, err := gomeboy.LoadRecording(semantic.RecordingPath)
	if err != nil {
		return preparedReplayAttempt{}, fmt.Errorf("load recording: %w", err)
	}
	segments := planReplayVideoSegments(recording.Attempt, parsed, maxFrames)
	for i := range segments {
		segments[i].CacheKey = s.replayVideoSegmentCacheKey(runID, recording, replayModeRaw, maxFrames, segments[i])
		segments[i].LocalPath = pathJoinOS(dir, fmt.Sprintf("attempt-%03d-part-%05d.mp4", recording.Attempt, segments[i].Index))
	}
	return preparedReplayAttempt{
		Recording: recording,
		Semantic:  semantic,
		Parsed:    parsed,
		Segments:  segments,
	}, nil
}

func (s *replayServer) applyReplaySegmentMode(runID string, mode replayMode, maxFrames uint64, attempt *preparedReplayAttempt) {
	if attempt == nil {
		return
	}
	for i := range attempt.Segments {
		attempt.Segments[i].CacheKey = s.replayVideoSegmentCacheKey(runID, attempt.Recording, mode, maxFrames, attempt.Segments[i])
	}
}

func (s *replayServer) probeReplaySegmentCache(ctx context.Context, attempts []preparedReplayAttempt) (int, error) {
	ready := 0
	for ai := range attempts {
		for si := range attempts[ai].Segments {
			segment := &attempts[ai].Segments[si]
			obj, err := s.store.HeadObject(ctx, segment.CacheKey)
			if err == nil {
				if obj.Size <= 0 {
					continue
				}
				url, err := s.store.PresignGetObject(segment.CacheKey, replaySegmentURLTTL)
				if err != nil {
					return ready, err
				}
				if err := probeReplayVideo(ctx, url, replaySegmentWindowDuration(*segment)); err != nil {
					// A malformed/truncated cached object is derived state. Treat it as
					// missing so the deterministic source recording regenerates only
					// this segment under the same cache key.
					log.Printf("pokereplay: cached segment invalid key=%s attempt=%d segment=%d: %v", segment.CacheKey, segment.Attempt, segment.Index, err)
					continue
				}
				segment.Cached = true
				ready++
				continue
			}
			if !artifactstore.IsNotFound(err) {
				return ready, err
			}
		}
	}
	return ready, nil
}

type replayVideoProbe struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func probeReplayVideo(ctx context.Context, source string, expected time.Duration) error {
	cmd := exec.CommandContext(ctx,
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_type:format=duration",
		"-of", "json",
		source,
	)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ffprobe rejected segment: %w", err)
	}
	var probe replayVideoProbe
	if err := json.Unmarshal(output, &probe); err != nil {
		return fmt.Errorf("decode ffprobe output: %w", err)
	}
	if len(probe.Streams) == 0 || probe.Streams[0].CodecType != "video" {
		return fmt.Errorf("segment has no video stream")
	}
	durationSeconds, err := strconv.ParseFloat(strings.TrimSpace(probe.Format.Duration), 64)
	if err != nil || durationSeconds <= 0 || math.IsNaN(durationSeconds) || math.IsInf(durationSeconds, 0) {
		return fmt.Errorf("segment has invalid duration %q", probe.Format.Duration)
	}
	if expected > 0 {
		got := time.Duration(durationSeconds * float64(time.Second))
		delta := got - expected
		if delta < 0 {
			delta = -delta
		}
		if delta > time.Second {
			return fmt.Errorf("segment duration %s differs from expected %s", got.Round(time.Millisecond), expected.Round(time.Millisecond))
		}
	}
	return nil
}

type replayRGBEncoder struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	output *replayOutputTail
	closed bool
}

func (s *replayServer) startReplayRGBEncoder(ctx context.Context, destination string) (*replayRGBEncoder, error) {
	cmd := exec.CommandContext(ctx, s.streamBinary, s.streamRGBArgs(destination)...)
	output := &replayOutputTail{}
	cmd.Stdout = output
	cmd.Stderr = output
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("gomeboy segment encoder stdin: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start gomeboy segment encoder: %w", err)
	}
	return &replayRGBEncoder{cmd: cmd, stdin: stdin, output: output}, nil
}

func (e *replayRGBEncoder) Write(frame gomeboy.Frame) error {
	if e == nil || e.closed || e.stdin == nil {
		return fmt.Errorf("replay segment encoder is closed")
	}
	data := frame.RGB
	for len(data) > 0 {
		n, err := e.stdin.Write(data)
		if err != nil {
			return fmt.Errorf("write replay segment frame: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("write replay segment frame: short write")
		}
		data = data[n:]
	}
	return nil
}

func (e *replayRGBEncoder) Close() error {
	if e == nil || e.closed {
		return nil
	}
	e.closed = true
	closeErr := e.stdin.Close()
	waitErr := e.cmd.Wait()
	if closeErr != nil {
		return fmt.Errorf("close replay segment encoder: %w", closeErr)
	}
	if waitErr != nil {
		return fmt.Errorf("replay segment encoder: %w: %s", waitErr, strings.TrimSpace(e.output.String()))
	}
	return nil
}

func (s *replayServer) renderAttemptVideoSegments(
	ctx context.Context,
	runID string,
	mode replayMode,
	attempt *preparedReplayAttempt,
	onStart func(replayVideoSegment),
	onReady func(),
) error {
	if attempt == nil || attempt.Parsed == nil {
		return fmt.Errorf("nil prepared replay attempt")
	}
	missing := false
	for _, segment := range attempt.Segments {
		if !segment.Cached {
			missing = true
			break
		}
	}
	if !missing {
		return nil
	}

	release, err := acquireReplayRender(ctx)
	if err != nil {
		return fmt.Errorf("wait for replay render slot: %w", err)
	}
	defer release()

	emu, err := gomeboy.New(
		gomeboy.WithROM(attempt.Semantic.ReplayROMPath),
		gomeboy.Headless(),
		gomeboy.WithModel(attempt.Parsed.Model),
	)
	if err != nil {
		return fmt.Errorf("create replay emulator: %w", err)
	}
	defer emu.Close()

	var timeline farm.MediaTimeline
	if mode == replayModeBroadcast {
		if s.compositor == nil {
			return fmt.Errorf("broadcast compositor is not configured")
		}
		timeline, err = s.mediaTimelineOrEmpty(ctx, runID, attempt.Recording.Attempt)
		if err != nil {
			return fmt.Errorf("media timeline: %w", err)
		}
	}

	var encoder *replayRGBEncoder
	var encoderSegment int = -1
	defer func() {
		if encoder != nil {
			_ = encoder.Close()
		}
	}()

	err = emu.ReplayRecordingFrames(attempt.Parsed, func(frame uint64, image gomeboy.Frame) error {
		if frame < attempt.Parsed.StartFrame {
			return nil
		}
		relative := frame - attempt.Parsed.StartFrame
		if len(attempt.Segments) == 0 || relative > attempt.Segments[len(attempt.Segments)-1].EndFrame {
			return nil
		}
		index := 0
		for index+1 < len(attempt.Segments) && relative > attempt.Segments[index].EndFrame {
			index++
		}
		segment := &attempt.Segments[index]
		if segment.Cached {
			return nil
		}

		if encoder == nil {
			if onStart != nil {
				onStart(*segment)
			}
			rawPath := segment.LocalPath
			if mode == replayModeBroadcast {
				rawPath += ".raw.mp4"
			}
			encoder, err = s.startReplayRGBEncoder(ctx, rawPath)
			if err != nil {
				return err
			}
			encoderSegment = index
		}
		if encoderSegment != index {
			return fmt.Errorf("replay segment encoder advanced from %d to %d without closing", encoderSegment, index)
		}
		if err := encoder.Write(image); err != nil {
			return err
		}
		if relative != segment.EndFrame {
			return nil
		}
		if err := encoder.Close(); err != nil {
			encoder = nil
			return err
		}
		encoder = nil

		if mode == replayModeBroadcast {
			rawPath := segment.LocalPath + ".raw.mp4"
			startMS := replayFrameTimeMS(segment.StartFrame)
			endMS := replayFrameTimeMS(segment.EndFrame + 1)
			if err := s.compositor.Compose(ctx, broadcastScene{
				RunID:       runID,
				Attempt:     attempt.Recording.Attempt,
				RawVideo:    rawPath,
				Destination: segment.LocalPath,
				Timeline:    timeline,
				StartMS:     startMS,
				EndMS:       endMS,
				VAAPI:       s.vaapi,
				VAAPIDevice: vaapiDevice(),
			}); err != nil {
				return fmt.Errorf("broadcast segment %d: %w", segment.Index, err)
			}
			_ = os.Remove(rawPath)
		}

		if err := probeReplayVideo(ctx, segment.LocalPath, replaySegmentWindowDuration(*segment)); err != nil {
			return fmt.Errorf("validate replay segment %d: %w", segment.Index, err)
		}
		file, err := os.Open(segment.LocalPath)
		if err != nil {
			return err
		}
		_, putErr := s.store.PutObjectReader(ctx, segment.CacheKey, "video/mp4", file)
		closeErr := file.Close()
		if putErr != nil {
			return fmt.Errorf("cache replay segment %d: %w", segment.Index, putErr)
		}
		if closeErr != nil {
			return closeErr
		}
		if err := os.Remove(segment.LocalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove uploaded replay segment %d: %w", segment.Index, err)
		}
		segment.Cached = true
		if onReady != nil {
			onReady()
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, segment := range attempt.Segments {
		if !segment.Cached {
			return fmt.Errorf("replay segment %d did not complete", segment.Index)
		}
	}
	return nil
}

func replayVideoSegmentURLs(s *replayServer, attempts []preparedReplayAttempt) ([]string, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("replay segment storage is not configured")
	}
	var urls []string
	for ai := range attempts {
		for si := range attempts[ai].Segments {
			segment := &attempts[ai].Segments[si]
			if !segment.Cached {
				return nil, fmt.Errorf("replay segment attempt=%d segment=%d is not cached", segment.Attempt, segment.Index)
			}
			url, err := s.store.PresignGetObject(segment.CacheKey, replaySegmentURLTTL)
			if err != nil {
				return nil, fmt.Errorf("presign replay segment attempt=%d segment=%d: %w", segment.Attempt, segment.Index, err)
			}
			urls = append(urls, url)
		}
	}
	return urls, nil
}

func concatReplaySegmentURLs(ctx context.Context, dir string, urls []string, destination string) error {
	if len(urls) == 0 {
		return fmt.Errorf("concat replay segments: no segment URLs")
	}
	var manifest strings.Builder
	manifest.WriteString("ffconcat version 1.0\n")
	for _, url := range urls {
		escaped := strings.ReplaceAll(url, "'", "'\\''")
		fmt.Fprintf(&manifest, "file '%s'\n", escaped)
	}
	manifestPath := pathJoinOS(dir, "segments-remote.ffconcat")
	if err := os.WriteFile(manifestPath, []byte(manifest.String()), 0o600); err != nil {
		return fmt.Errorf("write remote replay concat manifest: %w", err)
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-protocol_whitelist", "file,http,https,tcp,tls,crypto",
		"-f", "concat", "-safe", "0", "-i", manifestPath,
		"-c", "copy", "-movflags", "+faststart", destination,
	}
	output := &replayOutputTail{}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		detail := output.String()
		for _, url := range urls {
			detail = strings.ReplaceAll(detail, url, "[segment-url]")
		}
		return fmt.Errorf("concat remote replay segments: %w: %s", err, strings.TrimSpace(detail))
	}
	return nil
}

func totalReplayVideoSegments(attempts []preparedReplayAttempt) int {
	total := 0
	for _, attempt := range attempts {
		total += len(attempt.Segments)
	}
	return total
}

func replaySegmentWindowDuration(segment replayVideoSegment) time.Duration {
	start := replayFrameTimeMS(segment.StartFrame)
	end := replayFrameTimeMS(segment.EndFrame + 1)
	if end <= start {
		return 0
	}
	return time.Duration(end-start) * time.Millisecond
}

func replayAttemptsDuration(attempts []preparedReplayAttempt) time.Duration {
	var total time.Duration
	for _, attempt := range attempts {
		for _, segment := range attempt.Segments {
			total += replaySegmentWindowDuration(segment)
		}
	}
	return total
}

func (s *replayServer) renderLegacyReplay(
	ctx context.Context,
	runID string,
	recordings []replayRecording,
	semantic []semanticReplaySegment,
	dir string,
	mode replayMode,
	onReady func(done, total int),
) (int64, error) {
	if len(recordings) == 0 || len(recordings) != len(semantic) {
		return 0, fmt.Errorf("legacy replay inputs do not match")
	}
	videos := make([]string, len(recordings))
	for index, recording := range recordings {
		video, err := s.renderCachedSegment(ctx, runID, recording, semantic[index], dir, index, mode)
		if err != nil {
			return 0, fmt.Errorf("attempt %d: %w", recording.Attempt, err)
		}
		videos[index] = video
		if onReady != nil {
			onReady(index+1, len(recordings))
		}
	}
	if len(videos) == 1 {
		info, err := os.Stat(videos[0])
		if err != nil {
			return 0, err
		}
		return info.Size(), nil
	}
	videoPath := pathJoinOS(dir, "replay-legacy.mp4")
	if err := concatReplaySegments(ctx, dir, videos, videoPath); err != nil {
		return 0, err
	}
	file, err := os.Open(videoPath)
	if err != nil {
		return 0, err
	}
	obj, putErr := s.store.PutObjectReader(ctx, s.replayCacheKeyForMode(runID, recordings, mode), "video/mp4", file)
	closeErr := file.Close()
	if putErr != nil {
		return 0, putErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	return obj.Size, nil
}
