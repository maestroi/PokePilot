// Package thumbnail renders versioned YouTube thumbnail images from trusted
// packaging concepts. Artwork is optional; all text and branding are code-owned.
package thumbnail

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"math"
	"strings"

	"github.com/maestroi/pokepilot/media/packaging"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

const (
	SchemaVersion       = 1
	TemplateVersion     = "pokepilot-card-v1"
	RendererVersion     = "jpeg-compositor-v1"
	Width               = 1280
	Height              = 720
	MaxImageBytes       = 2 << 20 // YouTube's 2MB thumbnail upload ceiling.
	MaxArtworkBytes     = 5 << 20
	MaxArtworkDimension = 4096
	MaxVariants         = 3
)

type Spec struct {
	Version     int    `json:"version"`
	Template    string `json:"template"`
	Renderer    string `json:"renderer"`
	PackageHash string `json:"package_hash"`
	ConceptID   string `json:"concept_id"`
	Variant     int    `json:"variant"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Provider    string `json:"provider"`
	AssetSHA256 string `json:"asset_sha256,omitempty"`
	Hash        string `json:"hash"`
}
type Rendered struct {
	JPEG   []byte
	SHA256 string
	Size   int
	Width  int
	Height int
}

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func Seal(spec Spec) Spec {
	spec.Hash = ""
	data, _ := json.Marshal(spec)
	spec.Hash = Digest(data)
	return spec
}
func findConcept(pkg packaging.Package, id string) (packaging.ThumbnailConcept, bool) {
	for _, c := range pkg.Thumbnails {
		if c.ID == id {
			return c, true
		}
	}
	return packaging.ThumbnailConcept{}, false
}
func NewSpec(pkg packaging.Package, id string, variant int, provider, assetSHA string) (Spec, error) {
	if err := packaging.VerifyChecksum(pkg); err != nil {
		return Spec{}, err
	}
	c, ok := findConcept(pkg, id)
	if !ok || c.Evidence.EventID == "" || c.TitleID == "" || c.Subject == "" || c.Text == "" {
		return Spec{}, errors.New("thumbnail: missing evidence-backed concept")
	}
	if variant < 1 || variant > MaxVariants {
		return Spec{}, errors.New("thumbnail: variant out of range")
	}
	if len(provider) > 120 || strings.ContainsAny(provider, "\x00\r\n") || provider == "" {
		return Spec{}, errors.New("thumbnail: provider identity missing or invalid")
	}
	if assetSHA != "" && !validHash(assetSHA) {
		return Spec{}, errors.New("thumbnail: artwork hash invalid")
	}
	if provider == "none" && assetSHA != "" {
		return Spec{}, errors.New("thumbnail: artwork requires a provider")
	}
	s := Spec{Version: SchemaVersion, Template: TemplateVersion, Renderer: RendererVersion,
		PackageHash: pkg.Hash, ConceptID: id, Variant: variant, Width: Width, Height: Height,
		Provider: provider, AssetSHA256: assetSHA}
	return Seal(s), nil
}
func validHash(s string) bool { return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == "" }
func Validate(spec Spec, pkg packaging.Package) error {
	if spec.Version != SchemaVersion || spec.Template != TemplateVersion || spec.Renderer != RendererVersion ||
		spec.Width != Width || spec.Height != Height || spec.PackageHash != pkg.Hash {
		return errors.New("thumbnail: unknown template or package identity")
	}
	expected, err := NewSpec(pkg, spec.ConceptID, spec.Variant, spec.Provider, spec.AssetSHA256)
	if err != nil {
		return err
	}
	if spec.Hash != expected.Hash {
		return errors.New("thumbnail: render spec hash mismatch")
	}
	return nil
}

// DecodeArtwork checks the header BEFORE allocating image pixel memory. Only
// PNG and JPEG inputs are supported (never SVG/URLs/GIF).
func DecodeArtwork(data []byte) (image.Image, error) {
	if len(data) == 0 || len(data) > MaxArtworkBytes {
		return nil, errors.New("thumbnail: artwork exceeds byte limit")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("thumbnail: artwork is not a supported image")
	}
	if (format != "png" && format != "jpeg") || cfg.Width < 320 || cfg.Height < 180 ||
		cfg.Width > MaxArtworkDimension || cfg.Height > MaxArtworkDimension ||
		int64(cfg.Width)*int64(cfg.Height) > 12_000_000 {
		return nil, errors.New("thumbnail: artwork dimensions or format unsupported")
	}
	img, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format {
		return nil, errors.New("thumbnail: cannot decode artwork")
	}
	return img, nil
}

type theme struct {
	top    color.RGBA
	bottom color.RGBA
	accent color.RGBA
	trim   color.RGBA
}

var themes = []theme{
	{color.RGBA{20, 35, 59, 255}, color.RGBA{7, 14, 31, 255}, color.RGBA{251, 192, 65, 255}, color.RGBA{73, 121, 165, 255}},
	{color.RGBA{32, 25, 62, 255}, color.RGBA{11, 17, 37, 255}, color.RGBA{124, 224, 234, 255}, color.RGBA{117, 91, 184, 255}},
	{color.RGBA{50, 31, 29, 255}, color.RGBA{13, 17, 33, 255}, color.RGBA{251, 159, 80, 255}, color.RGBA{138, 86, 79, 255}},
}

func box(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	if x1 < x0 || y1 < y0 {
		return
	}
	draw.Draw(img, image.Rect(x0, y0, x1, y1), &image.Uniform{C: c}, image.Point{}, draw.Src)
}
func circle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := max(0, cy-r); y < min(Height, cy+r); y++ {
		dy := y - cy
		dx := int(math.Sqrt(float64(r*r - dy*dy)))
		box(img, max(0, cx-dx), y, min(Width, cx+dx), y+1, c)
	}
}
func backplate(img *image.RGBA, v int, t theme) {
	for y := 0; y < Height; y++ {
		f := float64(y) / float64(Height-1)
		blend := func(a, b uint8) uint8 { return uint8(float64(a)*(1-f) + float64(b)*f) }
		col := color.RGBA{blend(t.top.R, t.bottom.R), blend(t.top.G, t.bottom.G), blend(t.top.B, t.bottom.B), 255}
		box(img, 0, y, Width, y+1, col)
	}
	// Retro game-card motifs; artwork can replace the right panel without
	// changing the title safe area on the left.
	for i := 0; i < 13; i++ {
		x := 730 + (i*71+v*37)%520
		y := 55 + (i*137+v*61)%580
		box(img, x, y, x+48, y+48, t.trim)
		box(img, x+5, y+5, x+43, y+43, t.bottom)
	}
	circle(img, 975, 344, 250, t.trim)
	circle(img, 975, 344, 220, t.bottom)
	circle(img, 975, 344, 155, t.accent)
	circle(img, 975, 344, 132, t.top)
	box(img, 735, 325, 1215, 354, t.trim)
	circle(img, 975, 344, 58, t.trim)
	circle(img, 975, 344, 45, t.bottom)
}
func photo(img *image.RGBA, src image.Image) {
	// Fill only the artwork panel. Deterministically center-crop and preserve
	// source aspect ratio: generated art never covers brand/claims.
	b := src.Bounds()
	target := image.Rect(665, 0, Width, Height)
	tw, th := target.Dx(), target.Dy()
	scale := math.Max(float64(tw)/float64(b.Dx()), float64(th)/float64(b.Dy()))
	sw := int(math.Ceil(float64(tw) / scale))
	sh := int(math.Ceil(float64(th) / scale))
	x0 := b.Min.X + (b.Dx()-sw)/2
	y0 := b.Min.Y + (b.Dy()-sh)/2
	xdraw.CatmullRom.Scale(img, target, src, image.Rect(x0, y0, x0+sw, y0+sh), draw.Src, nil)
	// Left panel is an opaque text-safe band; no generated text shows through.
	box(img, 0, 0, 665, Height, color.RGBA{11, 20, 36, 255})
}
func textFace(ttf []byte, size float64) (font.Face, error) {
	ft, err := opentype.Parse(ttf)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(ft, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}
func text(img draw.Image, x, y int, value string, face font.Face, col color.Color) {
	d := &font.Drawer{Dst: img, Src: image.NewUniform(col), Face: face, Dot: fixed.P(x, y)}
	d.DrawString(value)
}
func fits(value string, face font.Face, maxWidth int) bool {
	return (&font.Drawer{Face: face}).MeasureString(value).Ceil() <= maxWidth
}
func wrap(value string, face font.Face, maxWidth, maxLines int) []string {
	words := strings.Fields(value)
	lines := make([]string, 0, maxLines)
	for _, word := range words {
		if len(lines) == 0 {
			lines = append(lines, "")
		}
		last := len(lines) - 1
		candidate := strings.TrimSpace(lines[last] + " " + word)
		if fits(candidate, face, maxWidth) {
			lines[last] = candidate
			continue
		}
		if len(lines) >= maxLines {
			break
		}
		lines = append(lines, word)
	}
	// Never render text beyond the safe area; truncate by rune only where
	// text from a trusted concept is unexpectedly long.
	for i, line := range lines {
		for !fits(line, face, maxWidth) && len(line) > 0 {
			line = string([]rune(line)[:len([]rune(line))-1])
		}
		lines[i] = line
	}
	return lines
}
func renderJpeg(img image.Image) ([]byte, error) {
	for _, q := range []int{89, 84, 78, 70, 62, 54} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
		if buf.Len() <= MaxImageBytes {
			return buf.Bytes(), nil
		}
	}
	return nil, errors.New("thumbnail: output exceeds upload ceiling")
}

// Render owns all public claims, logo, layout and typography. Generated
// artwork is merely a decorative input with no authority over the composition.
func Render(pkg packaging.Package, spec Spec, asset []byte) (Rendered, error) {
	if err := Validate(spec, pkg); err != nil {
		return Rendered{}, err
	}
	if spec.AssetSHA256 != Digest(asset) && spec.AssetSHA256 != "" {
		return Rendered{}, errors.New("thumbnail: artwork checksum mismatch")
	}
	if spec.AssetSHA256 == "" && len(asset) > 0 {
		return Rendered{}, errors.New("thumbnail: unbound artwork")
	}
	concept, _ := findConcept(pkg, spec.ConceptID)
	var art image.Image
	var err error
	if len(asset) > 0 {
		art, err = DecodeArtwork(asset)
		if err != nil {
			return Rendered{}, err
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	t := themes[(spec.Variant-1)%len(themes)]
	backplate(img, spec.Variant, t)
	if art != nil {
		photo(img, art)
	}
	// Bottom, top and content-safe zones are fixed across all variants.
	box(img, 0, 0, Width, 12, t.accent)
	box(img, 0, Height-18, Width, Height, t.accent)
	box(img, 52, 68, 63, 124, t.accent)
	logoFace, err := textFace(gobold.TTF, 35)
	if err != nil {
		return Rendered{}, err
	}
	defer logoFace.Close()
	headFace, err := textFace(gobold.TTF, 76)
	if err != nil {
		return Rendered{}, err
	}
	defer headFace.Close()
	footerFace, err := textFace(goregular.TTF, 27)
	if err != nil {
		return Rendered{}, err
	}
	defer footerFace.Close()
	white := color.RGBA{247, 250, 255, 255}
	text(img, 84, 110, "POKEPILOT", logoFace, white)
	box(img, 84, 142, 314, 148, t.accent)
	// 76pt headlines wrap to at most three lines, within a 550px safe column.
	lines := wrap(strings.ToUpper(concept.Text), headFace, 540, 3)
	if len(lines) == 0 || lines[0] == "" {
		return Rendered{}, errors.New("thumbnail: missing title")
	}
	for i, line := range lines {
		text(img, 64, 286+i*92, line, headFace, white)
	}
	text(img, 64, 654, "AUTONOMOUS RUN  /  "+strings.ToUpper(string(pkg.Style)), footerFace, white)
	out, err := renderJpeg(img)
	if err != nil {
		return Rendered{}, err
	}
	return Rendered{JPEG: out, SHA256: Digest(out), Size: len(out), Width: Width, Height: Height}, nil
}
