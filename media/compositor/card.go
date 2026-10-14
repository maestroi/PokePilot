package compositor

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/media/encode"
	"golang.org/x/image/draw"
)

// Card is a still full-frame title card (a reel's intro or outro).
type Card struct {
	Title       string
	Lines       []string
	DurationMS  int64
	Destination string
	VAAPI       bool
	VAAPIDevice string
}

// ComposeCard encodes a card with the same frame size, rate, pixel format and
// codec settings as Compose, so a reel can stream-copy cards and broadcast
// segments into one file.
func (c *FFmpeg) ComposeCard(ctx context.Context, card Card) error {
	if card.DurationMS <= 0 {
		return fmt.Errorf("card duration is required")
	}
	binary := strings.TrimSpace(c.Binary)
	if binary == "" {
		binary = "ffmpeg"
	}
	dir, err := os.MkdirTemp(filepath.Dir(card.Destination), ".pokepilot-card-*")
	if err != nil {
		return fmt.Errorf("create card render dir: %w", err)
	}
	defer os.RemoveAll(dir)
	still := filepath.Join(dir, "card.png")
	if err := writeScaledPNG(still, drawCard(card)); err != nil {
		return err
	}

	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	filter := "format=yuv420p"
	if card.VAAPI {
		device := strings.TrimSpace(card.VAAPIDevice)
		if device == "" {
			device = defaultVAAPIDevice
		}
		args = append(args, "-init_hw_device", "vaapi=va:"+device, "-filter_hw_device", "va")
		filter = "format=nv12,hwupload"
	}
	args = append(args,
		"-loop", "1", "-framerate", strconv.FormatFloat(farm.GameBoyFramesPerSecond, 'f', 6, 64),
		"-t", strconv.FormatFloat(float64(card.DurationMS)/1000, 'f', 3, 64), "-i", still,
		"-vf", filter, "-an",
	)
	if card.VAAPI {
		args = append(args, "-c:v", "h264_vaapi")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "20")
	}
	args = append(args, "-movflags", "+faststart", card.Destination)

	runner := c.Runner
	if runner == nil {
		runner = encode.NewExecRunner(0)
	}
	if output, err := runner.Run(ctx, binary, args...); err != nil {
		return fmt.Errorf("compose card: %w: %s", err, strings.TrimSpace(output))
	}
	return nil
}

func drawCard(card Card) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, sceneWidth, sceneHeight))
	fillRect(img, img.Bounds(), color.RGBA{R: 7, G: 16, B: 24, A: 255})
	fillRect(img, image.Rect(0, 0, sceneWidth, 6), color.RGBA{R: 232, G: 178, B: 79, A: 255})
	drawText(img, 40, 60, color.RGBA{R: 83, G: 200, B: 219, A: 255}, "POKEPILOT")

	// basicfont has one size; draw the title small and scale it up 4x.
	title := clipText(strings.ToUpper(strings.TrimSpace(card.Title)), 19)
	small := image.NewRGBA(image.Rect(0, 0, 7*len(title)+2, 16))
	drawText(small, 1, 12, color.RGBA{R: 242, G: 197, B: 108, A: 255}, title)
	at := image.Pt(40, 80)
	draw.NearestNeighbor.Scale(img, image.Rect(at.X, at.Y, at.X+small.Bounds().Dx()*4, at.Y+small.Bounds().Dy()*4), small, small.Bounds(), draw.Over, nil)

	y := 180
	for _, line := range card.Lines {
		for _, wrapped := range wrapText(line, 78) {
			if y > sceneHeight-20 {
				return img
			}
			drawText(img, 40, y, color.RGBA{R: 222, G: 232, B: 236, A: 255}, wrapped)
			y += 18
		}
	}
	return img
}
