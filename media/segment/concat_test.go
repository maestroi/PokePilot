package segment

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

type captureRunner struct {
	probes map[string]string // ffprobe output by input; missing → output
	binary string
	args   []string
	output string
	err    error
}

func (r *captureRunner) Run(_ context.Context, binary string, args ...string) (string, error) {
	if binary == "ffprobe" && r.probes != nil {
		return r.probes[args[len(args)-1]], nil
	}
	r.binary = binary
	r.args = append([]string(nil), args...)
	return r.output, r.err
}

func TestConcatFilesBuildsReusableFFmpegPlan(t *testing.T) {
	runner := &captureRunner{}
	dir := t.TempDir()
	if err := ConcatFiles(context.Background(), dir, []string{"/tmp/a.mp4", "/tmp/b's.mp4"}, "/tmp/out.mp4", runner); err != nil {
		t.Fatal(err)
	}
	if runner.binary != "ffmpeg" || !strings.Contains(strings.Join(runner.args, " "), "-f concat") {
		t.Fatalf("runner=%q args=%v", runner.binary, runner.args)
	}
	data, err := os.ReadFile(dir + "/segments.ffconcat")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "file '/tmp/a.mp4'") || !strings.Contains(text, "b'\\''s.mp4") {
		t.Fatalf("manifest=%q", text)
	}
}

func TestConcatURLsRedactsSignedInputsFromErrors(t *testing.T) {
	url := "https://example.invalid/part.mp4?secret=abc"
	runner := &captureRunner{output: "failed reading " + url, err: errors.New("exit 1")}
	err := ConcatURLs(context.Background(), t.TempDir(), []string{url}, "/tmp/out.mp4", runner)
	if err == nil {
		t.Fatal("expected concat failure")
	}
	if strings.Contains(err.Error(), "secret=abc") || !strings.Contains(err.Error(), "[segment-url]") {
		t.Fatalf("error was not redacted: %v", err)
	}
}

func TestConcatRejectsMismatchedStreamsBeforeCopying(t *testing.T) {
	runner := &captureRunner{probes: map[string]string{
		"/tmp/card.mp4": "h264,High,1280,720,yuv420p,1/23891",
		"/tmp/seg.mp4":  "h264,High,1280,720,yuv420p,1/594647",
	}}
	err := ConcatFiles(context.Background(), t.TempDir(), []string{"/tmp/card.mp4", "/tmp/seg.mp4"}, "/tmp/out.mp4", runner)
	if err == nil || !strings.Contains(err.Error(), "1/594647") {
		t.Fatalf("err = %v, want time_base mismatch", err)
	}
	if runner.binary == "ffmpeg" {
		t.Fatal("ffmpeg ran despite mismatched inputs")
	}
}
