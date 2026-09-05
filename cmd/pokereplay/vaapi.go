package main

import (
	"os"
	"os/exec"
	"strings"
)

const (
	defaultVAAPIDevice = "/dev/dri/renderD128"
	defaultFFmpegVAAPI = "/usr/local/bin/ffmpeg-vaapi"
	vaapiHWFilter      = "format=nv12,hwupload"
)

func vaapiDevice() string {
	if device := strings.TrimSpace(os.Getenv("POKEPILOT_VAAPI_DEVICE")); device != "" {
		return device
	}
	return defaultVAAPIDevice
}

func detectVAAPI() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("POKEPILOT_REPLAY_ENCODER"))) {
	case "off", "libx264", "software":
		return false
	case "vaapi", "on":
		return true
	}
	_, err := os.Stat(vaapiDevice())
	return err == nil
}

func (s *replayServer) streamArgs(recordingPath, videoPath string) []string {
	args := []string{
		"-rom", s.romPath,
		"-recording", recordingPath,
		"-output", videoPath,
		"-format", "mp4",
	}
	if !s.vaapi {
		return args
	}
	ffmpeg := s.ffmpegVAAPI
	if ffmpeg == "" {
		ffmpeg = defaultFFmpegVAAPI
	}
	return append(args,
		"-codec", "h264_vaapi",
		"-preset", "",
		"-ffmpeg", ffmpeg,
	)
}

func rewriteFFmpegVAAPI(args []string, device string) []string {
	if device == "" {
		device = defaultVAAPIDevice
	}
	out := make([]string, 0, len(args)+6)
	inserted := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-pix_fmt":
			if i+1 < len(args) {
				i++
			}
			continue
		case "-vf":
			out = append(out, arg)
			if i+1 < len(args) {
				i++
				out = append(out, appendVAAPIFilter(args[i]))
			}
			continue
		case "-loglevel":
			out = append(out, arg)
			if i+1 < len(args) {
				i++
				out = append(out, args[i])
			}
			if !inserted {
				out = append(out,
					"-init_hw_device", "vaapi=va:"+device,
					"-filter_hw_device", "va",
				)
				inserted = true
			}
			continue
		}
		out = append(out, arg)
	}
	if !inserted {
		out = append([]string{
			"-init_hw_device", "vaapi=va:" + device,
			"-filter_hw_device", "va",
		}, out...)
	}
	return out
}

func appendVAAPIFilter(vf string) string {
	if strings.Contains(vf, "hwupload") {
		return vf
	}
	if vf == "" {
		return vaapiHWFilter
	}
	return vf + "," + vaapiHWFilter
}

func runFFmpegVAAPI(args []string) int {
	cmd := exec.Command("ffmpeg", rewriteFFmpegVAAPI(args, vaapiDevice())...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		return 1
	}
	return 0
}
