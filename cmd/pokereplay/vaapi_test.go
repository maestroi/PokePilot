package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCartridgeForRecordingAppliesPatchBytes(t *testing.T) {
	base := []byte{0x10, 0xb1, 0x20, 0xb1}
	want := []byte{0x10, 0x83, 0x20, 0x83}
	sum := sha256.Sum256(want)
	got, err := cartridgeForRecording(base, map[string]string{
		"starter":         "mewtwo",
		"rom_patch_bytes": "0x1:b1>83,0x3:b1>83",
	}, hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("cartridgeForRecording = %x, want %x", got, want)
	}
}

func TestCartridgeForRecordingKeepsVanillaROM(t *testing.T) {
	base := []byte{0x10, 0xb1, 0x20}
	sum := sha256.Sum256(base)
	got, err := cartridgeForRecording(base, map[string]string{"starter": "squirtle"}, hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(base) {
		t.Fatalf("vanilla cartridge mutated: %x", got)
	}
}

func TestPrepareStreamROMFallsBackForNonRecordingBytes(t *testing.T) {
	dir := t.TempDir()
	rom := filepath.Join(dir, "pokemon-red.gb")
	if err := os.WriteFile(rom, []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	recording := filepath.Join(dir, "run.gbrun")
	if err := os.WriteFile(recording, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newReplayServer("", rom, "", nil)
	got, err := s.prepareStreamROM(dir, recording)
	if err != nil {
		t.Fatal(err)
	}
	if got != rom {
		t.Fatalf("prepareStreamROM = %q, want vanilla %q", got, rom)
	}
}

func TestPrepareStreamROMWritesDerivedCartridge(t *testing.T) {
	dir := t.TempDir()
	base := []byte{0x10, 0xb1, 0x20, 0xb1}
	derived := []byte{0x10, 0x83, 0x20, 0x83}
	rom := filepath.Join(dir, "pokemon-red.gb")
	if err := os.WriteFile(rom, base, 0o644); err != nil {
		t.Fatal(err)
	}
	recording := filepath.Join(dir, "run.gbrun")
	if err := os.WriteFile(recording, []byte("not parsed here"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newReplayServer("", rom, "", nil)
	s.deriveROM = func(baseROM []byte, metadata map[string]string, wantSHA string) ([]byte, error) {
		t.Fatal("deriveROM should not run for unparseable recordings")
		return nil, nil
	}
	got, err := s.prepareStreamROM(dir, recording)
	if err != nil {
		t.Fatal(err)
	}
	if got != rom {
		t.Fatalf("unparseable recording used %q", got)
	}

	s.deriveROM = func(baseROM []byte, metadata map[string]string, wantSHA string) ([]byte, error) {
		if string(baseROM) != string(base) {
			t.Fatalf("base ROM = %x", baseROM)
		}
		if metadata["rom_patch_bytes"] != "0x1:b1>83,0x3:b1>83" {
			t.Fatalf("metadata = %#v", metadata)
		}
		if wantSHA != hex.EncodeToString(sha256Sum(derived)) {
			t.Fatalf("wantSHA = %q", wantSHA)
		}
		return append([]byte(nil), derived...), nil
	}
	s.parseRecording = func(data []byte) (replayIdentity, error) {
		return replayIdentity{
			ROMSHA256: hex.EncodeToString(sha256Sum(derived)),
			Metadata:  map[string]string{"starter": "mewtwo", "rom_patch_bytes": "0x1:b1>83,0x3:b1>83"},
		}, nil
	}
	got, err = s.prepareStreamROM(dir, recording)
	if err != nil {
		t.Fatal(err)
	}
	if got == rom {
		t.Fatal("expected a derived ROM path, got the vanilla ROM")
	}
	body, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(derived) {
		t.Fatalf("derived ROM = %x, want %x", body, derived)
	}
}

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

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
	got := s.streamArgs("/rom/pokemon_red.gb", "/tmp/run.gbrun", "/tmp/replay.mp4")
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
	got := s.streamArgs("/rom/x.gb", "/tmp/run.gbrun", "/tmp/replay.mp4")
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
