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
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-protocol_whitelist", "file,http,https,tcp,tls,crypto",
		"-f", "concat", "-safe", "0", "-i", manifestPath,
		"-c", "copy", "-movflags", "+faststart", destination,
	}
	output, err := run(ctx, runner, "ffmpeg", args...)
	if err != nil {
		for _, url := range urls {
			output = strings.ReplaceAll(output, url, "[segment-url]")
		}
		return fmt.Errorf("concat remote media segments: %w: %s", err, strings.TrimSpace(output))
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
