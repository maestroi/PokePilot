package thumbnail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/media/packaging"
	"github.com/maestroi/pokepilot/media/producer"
)

func testPackage(t *testing.T) packaging.Package {
	t.Helper()
	tl := farm.MediaTimeline{
		Run:     farm.MediaRunSummary{RunID: "thumbnail-run", Goal: "win the badges", Status: "done"},
		Attempt: 1, EndFrame: 120000, FramesPerSecond: 60,
		Events: []farm.MediaEvent{
			{Type: "gym_battle", Frame: 30000, StartFrame: 24000, Evidence: "gym"},
			{Type: "badge_acquired", Frame: 33000, Evidence: "badge"},
			{Type: "blackout", Frame: 60000, Evidence: "blackout"},
			{Type: "failure_recovered", Frame: 73000, Evidence: "recovered"},
			{Type: "run_finished", Frame: 100000, Evidence: "finished"},
		},
	}.Normalized()
	in := producer.Input{RunID: "thumbnail-run", Timelines: []farm.MediaTimeline{tl}}
	a, err := producer.Assess(context.Background(), in, nil, producer.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := packaging.Generate(context.Background(), packaging.Request{Assessment: a, Timelines: []farm.MediaTimeline{tl}})
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}
func TestDeterministicBrandedVariants(t *testing.T) {
	pkg := testPackage(t)
	if len(pkg.Thumbnails) < 2 {
		t.Fatal("missing concept candidates")
	}
	first, err := NewSpec(pkg, pkg.Thumbnails[0].ID, 1, "none", "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Render(pkg, first, nil)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Render(pkg, first, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.JPEG, again.JPEG) || out.SHA256 != again.SHA256 {
		t.Fatal("deterministic re-render changed bytes")
	}
	if out.Size == 0 || out.Size > MaxImageBytes {
		t.Fatalf("image size %d exceeds YouTube limit", out.Size)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(out.JPEG))
	if err != nil || format != "jpeg" || cfg.Width != 1280 || cfg.Height != 720 {
		t.Fatalf("wrong image format/dimensions: %+v %q %v", cfg, format, err)
	}
	second, err := NewSpec(pkg, pkg.Thumbnails[0].ID, 2, "none", "")
	if err != nil {
		t.Fatal(err)
	}
	diff, err := Render(pkg, second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(out.JPEG, diff.JPEG) {
		t.Fatal("variants are not visually distinct")
	}
	if err := Validate(first, pkg); err != nil {
		t.Fatal(err)
	}
}
func TestIdentityAndSpecTamper(t *testing.T) {
	pkg := testPackage(t)
	if _, err := NewSpec(pkg, "missing", 1, "none", ""); err == nil {
		t.Fatal("unknown concept accepted")
	}
	if _, err := NewSpec(pkg, pkg.Thumbnails[0].ID, 4, "none", ""); err == nil {
		t.Fatal("oversized variant accepted")
	}
	spec, err := NewSpec(pkg, pkg.Thumbnails[0].ID, 1, "none", "")
	if err != nil {
		t.Fatal(err)
	}
	spec.Width = 1920
	if _, err := Render(pkg, spec, nil); err == nil {
		t.Fatal("unsupported image dimensions accepted")
	}
	spec.Width = 1280
	spec.PackageHash = strings.Repeat("a", 64)
	if err := Validate(spec, pkg); err == nil {
		t.Fatal("stale package reference accepted")
	}
	spec.PackageHash = pkg.Hash
	spec.Hash = strings.Repeat("a", 64)
	if err := Validate(spec, pkg); err == nil {
		t.Fatal("tampered render identity accepted")
	}
}
func assetBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	for y := 0; y < 360; y++ {
		for x := 0; x < 640; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 50, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestGeneratedArtworkIsBoundAndSafe(t *testing.T) {
	pkg := testPackage(t)
	asset := assetBytes(t)
	spec, err := NewSpec(pkg, pkg.Thumbnails[0].ID, 1, "images:test-model", Digest(asset))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Render(pkg, spec, asset)
	if err != nil {
		t.Fatal(err)
	}
	if out.Size > MaxImageBytes {
		t.Fatal("artwork exceeded output limit")
	}
	changed := append([]byte(nil), asset...)
	changed[len(changed)-1] ^= 1
	if _, err := Render(pkg, spec, changed); err == nil {
		t.Fatal("changed artwork accepted")
	}
	if _, err := DecodeArtwork([]byte("<svg><text>BAD</text></svg>")); err == nil {
		t.Fatal("unsafe SVG accepted")
	}
	if _, err := DecodeArtwork(bytes.Repeat([]byte{'x'}, MaxArtworkBytes+1)); err == nil {
		t.Fatal("oversized artwork accepted")
	}
	if _, err := NewSpec(pkg, pkg.Thumbnails[0].ID, 1, "none", Digest(asset)); err == nil {
		t.Fatal("unproven artwork provider accepted")
	}
	if _, err := jpeg.Decode(bytes.NewReader(out.JPEG)); err != nil {
		t.Fatal(err)
	}
}
func TestImageProviderAndFailClosed(t *testing.T) {
	pkg := testPackage(t)
	asset := assetBytes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("wrong provider request")
		}
		var payload struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.Model != "image-model" || !strings.Contains(payload.Prompt, "No words") {
			t.Errorf("unsafe prompt fields: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(asset)}}})
	}))
	defer server.Close()
	provider := ChatImageProvider{Endpoint: server.URL, ModelID: "image-model", APIKey: "test-key"}
	out, err := provider.Generate(context.Background(), pkg.Thumbnails[0], 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(asset, out) {
		t.Fatal("provider returned wrong artifact")
	}
	provider.Endpoint = "https://username:secret@example.com/images"
	if _, err := provider.Generate(context.Background(), pkg.Thumbnails[0], 1); err == nil {
		t.Fatal("URL embedded credentials accepted")
	}
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "api_secret=private", 500) }))
	defer broken.Close()
	provider.Endpoint = broken.URL
	if _, err := provider.Generate(context.Background(), pkg.Thumbnails[0], 1); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatalf("unsafe provider error leaked: %v", err)
	}
}
