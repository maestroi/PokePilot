package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log"
	"os"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/media/compositor"
	mediaencode "github.com/maestroi/pokepilot/media/encode"
	redrenderstate "github.com/maestroi/pokepilot/red/renderstate"
	protocol "github.com/maestroi/pokepilot/renderstate"
)

const (
	classicFrameWidth  = 160
	classicFrameHeight = 144
)

// renderAttemptSemanticSegments replays the authoritative recording once and
// selects a presentation frame on every callback. Supported RenderState samples
// use the public headless semantic renderer; transient/unsupported samples use
// the classic framebuffer already produced by that same replay callback.
func (s *replayServer) renderAttemptSemanticSegments(
	ctx context.Context,
	attempt *preparedReplayAttempt,
	onStart func(replayVideoSegment),
	onReady func(),
) error {
	if attempt == nil || attempt.Parsed == nil {
		return fmt.Errorf("nil prepared semantic replay attempt")
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
		return fmt.Errorf("wait for semantic render slot: %w", err)
	}
	defer release()

	replayROM, err := os.ReadFile(attempt.Semantic.ReplayROMPath)
	if err != nil {
		return fmt.Errorf("read semantic replay ROM: %w", err)
	}
	var producer *redrenderstate.Producer
	if s.semanticRenderer != nil {
		producer, err = redrenderstate.New(replayROM)
		if err != nil {
			// A recording without the current rich semantic adapter is still a
			// valid replay. Semantic mode degrades to Classic rather than failing.
			log.Printf("pokereplay: semantic adapter unavailable attempt=%d; classic fallback: %v", attempt.Recording.Attempt, err)
			producer = nil
		}
	}

	emu, err := gomeboy.New(
		gomeboy.WithROMBytes(replayROM),
		gomeboy.Headless(),
		gomeboy.WithModel(attempt.Parsed.Model),
	)
	if err != nil {
		return fmt.Errorf("create semantic replay emulator: %w", err)
	}
	defer emu.Close()

	var encoder *mediaencode.RawVideoEncoder
	encoderSegment := -1
	var semanticFrame *image.RGBA
	semanticSupported := false
	defer func() {
		if encoder != nil {
			_ = encoder.Close()
		}
	}()

	err = emu.ReplayRecordingFrames(attempt.Parsed, func(frame uint64, raw gomeboy.Frame) error {
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

		// Browser semantic replay is sampled every six source frames. Headless
		// video uses the same cadence and holds the latest deterministic scene
		// between samples. Segment starts force a sample so resumed work never
		// depends on a previous segment being re-encoded.
		sample := relative%semanticReplaySampleEveryFrames == 0 ||
			relative == segment.StartFrame ||
			frame == attempt.Parsed.EndFrame
		if sample {
			semanticFrame = nil
			semanticSupported = false
			if producer != nil && s.semanticRenderer != nil {
				state, snapshotErr := producer.Snapshot(emu, protocol.FrameMeta{
					Epoch: uint64(attempt.Recording.Attempt + 1),
					Frame: frame,
					Cycle: emu.Cycle(),
				})
				if snapshotErr == nil && s.semanticRenderer.Supports(state) {
					rendered, renderErr := s.semanticRenderer.Render(state, compositor.SemanticRenderOptions{
						Width: compositor.SemanticVideoWidth, Height: compositor.SemanticVideoHeight,
						AtMS: replayFrameTimeMS(relative),
					})
					if renderErr == nil {
						semanticFrame = rendered
						semanticSupported = true
					}
				}
			}
		}

		if segment.Cached {
			return nil
		}
		if encoder == nil {
			if onStart != nil {
				onStart(*segment)
			}
			encoder, err = mediaencode.StartRawVideo(ctx, segment.LocalPath, mediaencode.RawVideoOptions{
				Binary:       "ffmpeg",
				InputWidth:   compositor.SemanticVideoWidth,
				InputHeight:  compositor.SemanticVideoHeight,
				OutputWidth:  compositor.OutputWidth,
				OutputHeight: compositor.OutputHeight,
				FramesPerSec: farm.GameBoyFramesPerSecond,
				VAAPI:        s.vaapi,
				VAAPIDevice:  vaapiDevice(),
			})
			if err != nil {
				return err
			}
			encoderSegment = index
		}
		if encoderSegment != index {
			return fmt.Errorf("semantic segment encoder advanced from %d to %d without closing", encoderSegment, index)
		}

		presentation := image.Image(semanticFrame)
		if !semanticSupported || semanticFrame == nil {
			fallback, fallbackErr := compositor.RenderClassicRGB(
				raw.RGB,
				classicFrameWidth, classicFrameHeight,
				compositor.SemanticVideoWidth, compositor.SemanticVideoHeight,
			)
			if fallbackErr != nil {
				return fallbackErr
			}
			presentation = fallback
		}
		if err := encoder.WriteImage(presentation); err != nil {
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
		encoderSegment = -1

		if err := probeReplayVideo(ctx, segment.LocalPath, replaySegmentWindowDuration(*segment)); err != nil {
			return fmt.Errorf("validate semantic replay segment %d: %w", segment.Index, err)
		}
		file, err := os.Open(segment.LocalPath)
		if err != nil {
			return err
		}
		_, putErr := s.store.PutObjectReader(ctx, segment.CacheKey, "video/mp4", file)
		closeErr := file.Close()
		if putErr != nil {
			return fmt.Errorf("cache semantic replay segment %d: %w", segment.Index, putErr)
		}
		if closeErr != nil {
			return closeErr
		}
		if err := os.Remove(segment.LocalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove uploaded semantic replay segment %d: %w", segment.Index, err)
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
			return fmt.Errorf("semantic replay segment %d did not complete", segment.Index)
		}
	}
	return nil
}
