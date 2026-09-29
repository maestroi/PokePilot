package encode

import (
	"context"
	"fmt"
	"image"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type RawVideoOptions struct {
	Binary       string
	InputWidth   int
	InputHeight  int
	OutputWidth  int
	OutputHeight int
	FramesPerSec float64
	VAAPI        bool
	VAAPIDevice  string
}

type RawVideoEncoder struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	output *tailWriter
	opts   RawVideoOptions
	buf    []byte
	closed bool
}

func StartRawVideo(ctx context.Context, destination string, options RawVideoOptions) (*RawVideoEncoder, error) {
	if options.InputWidth <= 0 || options.InputHeight <= 0 {
		return nil, fmt.Errorf("raw video input dimensions are required")
	}
	if options.OutputWidth <= 0 {
		options.OutputWidth = options.InputWidth
	}
	if options.OutputHeight <= 0 {
		options.OutputHeight = options.InputHeight
	}
	if options.FramesPerSec <= 0 {
		return nil, fmt.Errorf("raw video frame rate is required")
	}
	binary := strings.TrimSpace(options.Binary)
	if binary == "" {
		binary = "ffmpeg"
	}
	args := rawVideoArgs(destination, options)
	output := &tailWriter{limit: DefaultMaxOutputBytes}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = output
	cmd.Stderr = output
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("raw video encoder stdin: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start raw video encoder: %w", err)
	}
	return &RawVideoEncoder{
		cmd: cmd, stdin: stdin, output: output, opts: options,
		buf: make([]byte, options.InputWidth*options.InputHeight*3),
	}, nil
}

func rawVideoArgs(destination string, options RawVideoOptions) []string {
	fps := strconv.FormatFloat(options.FramesPerSec, 'f', 6, 64)
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if options.VAAPI {
		device := strings.TrimSpace(options.VAAPIDevice)
		if device == "" {
			device = "/dev/dri/renderD128"
		}
		args = append(args, "-init_hw_device", "vaapi=va:"+device, "-filter_hw_device", "va")
	}
	args = append(args,
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"-s:v", fmt.Sprintf("%dx%d", options.InputWidth, options.InputHeight),
		"-r", fps,
		"-i", "-",
		"-an",
	)
	var filters []string
	if options.OutputWidth != options.InputWidth || options.OutputHeight != options.InputHeight {
		filters = append(filters, fmt.Sprintf("scale=%d:%d:flags=neighbor", options.OutputWidth, options.OutputHeight))
	}
	if options.VAAPI {
		filters = append(filters, "format=nv12", "hwupload")
	}
	if len(filters) > 0 {
		args = append(args, "-vf", strings.Join(filters, ","))
	}
	if options.VAAPI {
		args = append(args, "-c:v", "h264_vaapi")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p")
	}
	return append(args, "-movflags", "+faststart", destination)
}

func (e *RawVideoEncoder) WriteImage(frame image.Image) error {
	if e == nil || e.closed || e.stdin == nil {
		return fmt.Errorf("raw video encoder is closed")
	}
	bounds := frame.Bounds()
	if bounds.Dx() != e.opts.InputWidth || bounds.Dy() != e.opts.InputHeight {
		return fmt.Errorf("raw video frame is %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), e.opts.InputWidth, e.opts.InputHeight)
	}
	if rgba, ok := frame.(*image.RGBA); ok && bounds.Min.X == 0 && bounds.Min.Y == 0 {
		j := 0
		for y := 0; y < e.opts.InputHeight; y++ {
			offset := rgba.PixOffset(0, y)
			row := rgba.Pix[offset : offset+e.opts.InputWidth*4]
			for x := 0; x < len(row); x += 4 {
				e.buf[j] = row[x]
				e.buf[j+1] = row[x+1]
				e.buf[j+2] = row[x+2]
				j += 3
			}
		}
	} else {
		j := 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, _ := frame.At(x, y).RGBA()
				e.buf[j] = uint8(r >> 8)
				e.buf[j+1] = uint8(g >> 8)
				e.buf[j+2] = uint8(b >> 8)
				j += 3
			}
		}
	}
	data := e.buf
	for len(data) > 0 {
		n, err := e.stdin.Write(data)
		if err != nil {
			return fmt.Errorf("write raw video frame: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("write raw video frame: short write")
		}
		data = data[n:]
	}
	return nil
}

func (e *RawVideoEncoder) Close() error {
	if e == nil || e.closed {
		return nil
	}
	e.closed = true
	closeErr := e.stdin.Close()
	waitErr := e.cmd.Wait()
	if closeErr != nil {
		return fmt.Errorf("close raw video encoder: %w", closeErr)
	}
	if waitErr != nil {
		return fmt.Errorf("raw video encoder: %w: %s", waitErr, strings.TrimSpace(e.output.String()))
	}
	return nil
}
