package segment

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/maestroi/pokepilot/media/encode"
)

func ConcatFiles(ctx context.Context, workDir string, files []string, destination string, runner encode.Runner) error {
	if len(files) == 0 {
		return fmt.Errorf("concat media segments: no input files")
	}
	manifest := buildManifest(files)
	manifestPath := filepath.Join(workDir, "segments.ffconcat")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		return fmt.Errorf("write media concat manifest: %w", err)
	}
	if err := requireMatchingStreams(ctx, runner, files); err != nil {
		return fmt.Errorf("concat media segments: %w", err)
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "concat", "-safe", "0", "-i", manifestPath,
		"-c", "copy", "-movflags", "+faststart", destination,
	}
	output, err := run(ctx, runner, "ffmpeg", args...)
	if err != nil {
		return fmt.Errorf("concat media segments: %w: %s", err, strings.TrimSpace(output))
	}
	return nil
}

func ConcatURLs(ctx context.Context, workDir string, urls []string, destination string, runner encode.Runner) error {
	if len(urls) == 0 {
		return fmt.Errorf("concat media segments: no segment URLs")
	}
	manifest := buildManifest(urls)
	manifestPath := filepath.Join(workDir, "segments-remote.ffconcat")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		return fmt.Errorf("write remote media concat manifest: %w", err)
	}
	if err := requireMatchingStreams(ctx, runner, urls); err != nil {
		return fmt.Errorf("concat remote media segments: %s", redactURLs(err.Error(), urls))
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-protocol_whitelist", "file,http,https,tcp,tls,crypto",
		"-f", "concat", "-safe", "0", "-i", manifestPath,
		"-c", "copy", "-movflags", "+faststart", destination,
	}
	output, err := run(ctx, runner, "ffmpeg", args...)
	if err != nil {
		return fmt.Errorf("concat remote media segments: %w: %s", err, redactURLs(strings.TrimSpace(output), urls))
	}
	return nil
}

func redactURLs(text string, urls []string) string {
	for _, url := range urls {
		text = strings.ReplaceAll(text, url, "[segment-url]")
	}
	return text
}

// requireMatchingStreams rejects inputs whose video streams differ. Concat with
// -c copy keeps the first input's parameters, so a different time_base later
// silently rescales every following input instead of failing.
func requireMatchingStreams(ctx context.Context, runner encode.Runner, inputs []string) error {
	var first string
	for i, input := range inputs {
		output, err := run(ctx, runner, "ffprobe", "-v", "error", "-select_streams", "v:0",
			"-show_entries", "stream=codec_name,profile,width,height,pix_fmt,time_base",
			"-of", "csv=p=0", input)
		if err != nil {
			return fmt.Errorf("probe input %d: %w: %s", i, err, strings.TrimSpace(output))
		}
		params := strings.TrimSpace(output)
		if i == 0 {
			first = params
		} else if params != first {
			return fmt.Errorf("input %d video stream %q does not match input 0 %q", i, params, first)
		}
	}
	return nil
}

func buildManifest(inputs []string) string {
	var manifest strings.Builder
	manifest.WriteString("ffconcat version 1.0\n")
	for _, input := range inputs {
		escaped := strings.ReplaceAll(input, "'", "'\\''")
		fmt.Fprintf(&manifest, "file '%s'\n", escaped)
	}
	return manifest.String()
}

func run(ctx context.Context, runner encode.Runner, binary string, args ...string) (string, error) {
	if runner == nil {
		execRunner := encode.NewExecRunner(0)
		runner = execRunner
	}
	return runner.Run(ctx, binary, args...)
}
