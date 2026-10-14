package compositor

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/media/segment"
)

// A reel stream-copies cards next to broadcast segments, so both must carry
// identical video stream parameters.
func TestCardConcatsWithBroadcastSegment(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	ctx := context.Background()
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.mp4")
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=green:size=160x144:rate=59.7275", "-t", "1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", raw).CombinedOutput(); err != nil {
		t.Fatalf("raw: %v %s", err, out)
	}
	c := NewFFmpeg("ffmpeg", nil)
	seg := filepath.Join(dir, "seg.mp4")
	timeline := farm.MediaTimeline{Run: farm.MediaRunSummary{RunID: "run"}, Attempt: 1, FramesPerSecond: farm.GameBoyFramesPerSecond}
	if err := c.Compose(ctx, Scene{RunID: "run", Attempt: 1, RawVideo: raw, Destination: seg, Timeline: timeline}); err != nil {
		t.Fatal(err)
	}
	card := filepath.Join(dir, "card.mp4")
	if err := c.ComposeCard(ctx, Card{Title: "Run complete", Lines: []string{"Badges: 8"}, DurationMS: 2000, Destination: card}); err != nil {
		t.Fatal(err)
	}
	if a, b := streamParams(t, seg), streamParams(t, card); a != b {
		t.Fatalf("segment %q != card %q", a, b)
	}
	reel := filepath.Join(dir, "reel.mp4")
	if err := segment.ConcatURLs(ctx, dir, []string{card, seg, card}, reel, nil); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", reel).Output()
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64); d < 4.8 || d > 5.3 {
		t.Fatalf("reel duration = %.3fs, want ~5s", d)
	}
}

func streamParams(t *testing.T, file string) string {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name,profile,width,height,pix_fmt,r_frame_rate", "-of", "csv=p=0", file).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
