package encode

import (
	"strings"
	"testing"
)

func TestRawVideoArgsScaleAndSelectSoftwareEncoder(t *testing.T) {
	args := rawVideoArgs("/tmp/out.mp4", RawVideoOptions{
		InputWidth: 640, InputHeight: 360,
		OutputWidth: 1280, OutputHeight: 720,
		FramesPerSec: 59.7275,
	})
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-f rawvideo", "-pix_fmt rgb24", "-s:v 640x360",
		"scale=1280:720:flags=neighbor", "-c:v libx264", "-pix_fmt yuv420p",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "hwupload") {
		t.Fatalf("software args unexpectedly use hardware upload: %s", joined)
	}
}

func TestRawVideoArgsUseVAAPIWithoutSoftwarePixelFormat(t *testing.T) {
	args := rawVideoArgs("/tmp/out.mp4", RawVideoOptions{
		InputWidth: 640, InputHeight: 360,
		OutputWidth: 1280, OutputHeight: 720,
		FramesPerSec: 60,
		VAAPI: true, VAAPIDevice: "/dev/dri/test",
	})
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"vaapi=va:/dev/dri/test", "scale=1280:720:flags=neighbor,format=nv12,hwupload", "-c:v h264_vaapi",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %s", want, joined)
		}
	}
}
