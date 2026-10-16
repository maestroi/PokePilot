package segment

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/media/editplan"
	"github.com/maestroi/pokepilot/media/encode"
)

const EditEncoderVersion = "libx264-30fps-v1"

type EditChapter struct {
	StartMS int64  `json:"start_ms"`
	Title   string `json:"title"`
}

type EditResult struct {
	DurationMS int64         `json:"duration_ms"`
	Chapters   []EditChapter `json:"chapters,omitempty"`
}

// RenderEdit transcodes verified, typed segments from an existing composited
// full-replay MP4. No gameplay is replayed, and the original video is intact.
// Input must be a trusted local path or a presigned S3 URL obtained by the
// replay worker, NEVER model-provided media/command text.
func RenderEdit(ctx context.Context, source string, plan editplan.Plan, workDir, destination string, runner encode.Runner) (EditResult, error) {
	if source == "" || workDir == "" || destination == "" {
		return EditResult{}, fmt.Errorf("edit render: missing source or destination")
	}
	if plan.RenderVersion != editplan.RenderVersion || !editplan.ValidProfile(plan.Profile) {
		return EditResult{}, fmt.Errorf("edit render: unsupported renderer/profile")
	}
	if runner == nil {
		runner = encode.NewExecRunner(0)
	}
	width, height := 1280, 720
	if plan.Profile == editplan.Short {
		width, height = 1080, 1920
	}
	var files []string
	var result EditResult
	for i, s := range plan.Segments {
		if s.Action == editplan.Cut {
			continue
		}
		// Recheck the schema at the encoder boundary. This function should
		// only receive Validate-approved plans; these checks defend a
		// future caller that accidentally skips that step.
		if s.Action != editplan.Keep && s.Action != editplan.Card && s.Action != editplan.Pad {
			return EditResult{}, fmt.Errorf("edit render: unsupported action")
		}
		if s.Action == editplan.Keep && (s.Speed != 1 && s.Speed != 2 && s.Speed != 4 && s.Speed != 8 && s.Speed != 16 ||
			s.EndMS <= s.StartMS) {
			return EditResult{}, fmt.Errorf("edit render: invalid speed/window")
		}
		if (s.Action == editplan.Card || s.Action == editplan.Pad) && (s.DurationMS < 250 || s.DurationMS > 15000) {
			return EditResult{}, fmt.Errorf("edit render: invalid generated duration")
		}
		clipMS := s.DurationMS
		if s.Action == editplan.Keep {
			clipMS = int64(math.Ceil(float64(s.EndMS-s.StartMS) / float64(s.Speed)))
		}
		if clipMS <= 0 {
			return EditResult{}, fmt.Errorf("edit render: invalid output duration")
		}
		clip := filepath.Join(workDir, fmt.Sprintf("edit-%04d.mp4", i))
		args := []string{"-hide_banner", "-loglevel", "error", "-y"}
		if s.Action == editplan.Keep {
			start, err := editplan.OffsetMS(plan.Sources, s.Attempt, s.StartMS)
			if err != nil {
				return EditResult{}, err
			}
			args = append(args, "-ss", seconds(start), "-t", seconds(s.EndMS-s.StartMS), "-i", source)
		} else {
			color := "0x071018"
			if s.Action == editplan.Pad {
				color = "black"
			}
			args = append(args, "-f", "lavfi", "-i", fmt.Sprintf("color=c=%s:s=%dx%d:r=30:d=%s", color, width, height, seconds(clipMS)))
		}
		filter := "setpts=(PTS-STARTPTS)"
		if s.Action == editplan.Keep {
			filter = fmt.Sprintf("setpts=(PTS-STARTPTS)/%d", s.Speed)
		}
		filter += ",fps=30"
		if s.Action == editplan.Keep {
			filter += fmt.Sprintf(",scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black", width, height, width, height)
		}
		filter += ",format=yuv420p"
		// Text is supplied ONLY through a bounded local text file. It cannot
		// inject filter instructions or shell flags, and is never interpolated
		// into an ffmpeg expression.
		for _, entry := range []struct {
			value   string
			size, y string
		}{
			{s.Overlay, "32", "60"},
			{s.Caption, "29", "h-145"},
		} {
			if entry.value == "" {
				continue
			}
			if len(entry.value) > 240 || strings.ContainsRune(entry.value, 0) {
				return EditResult{}, fmt.Errorf("edit render: oversized text overlay")
			}
			textPath := filepath.Join(workDir, fmt.Sprintf("edit-%04d-caption-%d.txt", i, len(filter)))
			if err := os.WriteFile(textPath, []byte(entry.value), 0600); err != nil {
				return EditResult{}, err
			}
			filter += fmt.Sprintf(",drawtext=textfile=%s:fontsize=%s:fontcolor=white:box=1:boxcolor=black@0.75:boxborderw=14:x=(w-text_w)/2:y=%s", escapeFilterPath(textPath), entry.size, entry.y)
		}
		if s.Transition == "fade" && clipMS >= 800 {
			fadeSeconds := 0.3
			filter += fmt.Sprintf(",fade=t=in:st=0:d=%.3f,fade=t=out:st=%.3f:d=%.3f", fadeSeconds, float64(clipMS)/1000-fadeSeconds, fadeSeconds)
		}
		args = append(args, "-vf", filter, "-an", "-c:v", "libx264", "-preset", "veryfast", "-crf", "20",
			"-pix_fmt", "yuv420p", "-r", "30", "-movflags", "+faststart", clip)
		output, err := runner.Run(ctx, "ffmpeg", args...)
		if err != nil {
			// The presigned source URL is an internal credential and can
			// appear in an encoder error. Redact it before returning.
			output = strings.ReplaceAll(output, source, "[source-video]")
			return EditResult{}, fmt.Errorf("edit render segment %d: %w: %s", i, err, strings.TrimSpace(output))
		}
		if s.Chapter != "" {
			result.Chapters = append(result.Chapters, EditChapter{StartMS: result.DurationMS, Title: s.Chapter})
		}
		result.DurationMS += clipMS
		files = append(files, clip)
	}
	if len(files) == 0 {
		return EditResult{}, fmt.Errorf("edit render: no playable segments")
	}
	if err := ConcatFiles(ctx, workDir, files, destination, runner); err != nil {
		return EditResult{}, err
	}
	return result, nil
}

func seconds(ms int64) string { return strconv.FormatFloat(float64(ms)/1000, 'f', 3, 64) }
func escapeFilterPath(s string) string {
	// Paths are generated by the replay worker under a private temp dir.
	// Backslashes and FFmpeg option separators still require escaping.
	r := strings.NewReplacer("\\", "\\\\", ":", "\\:", ",", "\\,", "'", "\\'", "[", "\\[", "]", "\\]")
	return r.Replace(s)
}
