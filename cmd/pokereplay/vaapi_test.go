package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRewriteFFmpegVAAPI(t *testing.T) {
	in := []string{
		"-hide_banner", "-loglevel", "warning",
		"-f", "rawvideo", "-pixel_format", "rgb24",
		"-video_size", "160x144", "-framerate", "59.727501",
		"-i", "pipe:0", "-an",
		"-vf", "scale=640:576:flags=neighbor",
		"-c:v", "h264_vaapi",
		"-pix_fmt", "yuv420p",
		"-f", "mp4",
		"/tmp/replay.mp4",
	}
	got := rewriteFFmpegVAAPI(in, "/dev/dri/renderD128")
	want := []string{
		"-hide_banner", "-loglevel", "warning",
		"-init_hw_device", "vaapi=va:/dev/dri/renderD128",
		"-filter_hw_device", "va",
		"-f", "rawvideo", "-pixel_format", "rgb24",
		"-video_size", "160x144", "-framerate", "59.727501",
		"-i", "pipe:0", "-an",
		"-vf", "scale=640:576:flags=neighbor,format=nv12,hwupload",
		"-c:v", "h264_vaapi",
		"-f", "mp4",
		"/tmp/replay.mp4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rewriteFFmpegVAAPI mismatch\ngot  %#v\nwant %#v", got, want)
	}
}

func TestStreamArgsUseVAAPIWhenEnabled(t *testing.T) {
	s := &replayServer{
		romPath:      "/rom/pokemon_red.gb",
		streamBinary: "/usr/local/bin/gomeboy-stream",
		vaapi:        true,
		ffmpegVAAPI:  "/usr/local/bin/ffmpeg-vaapi",
	}
	got := s.streamArgs("/tmp/run.gbrun", "/tmp/replay.mp4")
	want := []string{
		"-rom", "/rom/pokemon_red.gb",
		"-recording", "/tmp/run.gbrun",
		"-output", "/tmp/replay.mp4",
		"-format", "mp4",
		"-codec", "h264_vaapi",
		"-preset", "",
		"-ffmpeg", "/usr/local/bin/ffmpeg-vaapi",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("streamArgs mismatch\ngot  %#v\nwant %#v", got, want)
	}
}

func TestStreamArgsStaySoftwareWhenVAAPIOff(t *testing.T) {
	s := &replayServer{romPath: "/rom/x.gb", streamBinary: "gomeboy-stream"}
	got := s.streamArgs("/tmp/run.gbrun", "/tmp/replay.mp4")
	want := []string{
		"-rom", "/rom/x.gb",
		"-recording", "/tmp/run.gbrun",
		"-output", "/tmp/replay.mp4",
		"-format", "mp4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("streamArgs mismatch\ngot  %#v\nwant %#v", got, want)
	}
}

func TestDetectVAAPIUsesRenderNode(t *testing.T) {
	dir := t.TempDir()
	device := filepath.Join(dir, "renderD128")
	if err := os.WriteFile(device, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POKEPILOT_REPLAY_ENCODER", "")
	t.Setenv("POKEPILOT_VAAPI_DEVICE", device)
	if !detectVAAPI() {
		t.Fatal("detectVAAPI() = false, want true when render node exists")
	}
}

func TestVAAPIExplainForcedOff(t *testing.T) {
	on, encoder, reason := vaapiExplain("off", "/dev/dri/renderD128", true)
	if on || encoder != "libx264" {
		t.Fatalf("forced off: on=%t encoder=%q", on, encoder)
	}
	if !strings.Contains(reason, "POKEPILOT_REPLAY_ENCODER=off") {
		t.Fatalf("reason=%q", reason)
	}
}

func TestVAAPIExplainAutoWithDevice(t *testing.T) {
	on, encoder, reason := vaapiExplain("", "/dev/dri/renderD128", true)
	if !on || encoder != "h264_vaapi" {
		t.Fatalf("auto+device: on=%t encoder=%q", on, encoder)
	}
	if !strings.Contains(reason, "/dev/dri/renderD128") {
		t.Fatalf("reason=%q", reason)
	}
}

func TestVAAPIExplainAutoWithoutDevice(t *testing.T) {
	on, encoder, reason := vaapiExplain("auto", "/dev/dri/renderD128", false)
	if on || encoder != "libx264" {
		t.Fatalf("auto/no device: on=%t encoder=%q", on, encoder)
	}
	if !strings.Contains(reason, "not found") {
		t.Fatalf("reason=%q", reason)
	}
}

func TestDetectVAAPIHonorsOff(t *testing.T) {
	dir := t.TempDir()
	device := filepath.Join(dir, "renderD128")
	if err := os.WriteFile(device, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POKEPILOT_REPLAY_ENCODER", "off")
	t.Setenv("POKEPILOT_VAAPI_DEVICE", device)
	if detectVAAPI() {
		t.Fatal("detectVAAPI() = true, want false when encoder=off")
	}
}
