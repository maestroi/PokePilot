package compositor

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	protocol "github.com/maestroi/pokepilot/renderstate"
)

func semanticFixtureStates(t *testing.T) map[string]protocol.RenderState {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate semantic renderer test")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "renderstate", "testdata", "modern-scenes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var states map[string]protocol.RenderState
	if err := json.Unmarshal(data, &states); err != nil {
		t.Fatal(err)
	}
	return states
}

func TestHeadlessSemanticRendererUsesSharedSceneContract(t *testing.T) {
	renderer, err := NewPublicSemanticRenderer()
	if err != nil {
		t.Fatal(err)
	}
	states := semanticFixtureStates(t)
	want := map[string]SemanticSceneKind{
		"overworld": SemanticSceneOverworld,
		"dialogue":  SemanticSceneDialogue,
		"menu":      SemanticSceneMenu,
		"battle":    SemanticSceneBattle,
	}
	for name, kind := range want {
		state := states[name]
		if got := SemanticScene(state); got != kind {
			t.Fatalf("%s scene=%q want %q", name, got, kind)
		}
		if !renderer.Supports(state) {
			t.Fatalf("%s fixture is not supported", name)
		}
		frame, err := renderer.Render(state, SemanticRenderOptions{Width: SemanticVideoWidth, Height: SemanticVideoHeight, AtMS: 250})
		if err != nil {
			t.Fatalf("%s render: %v", name, err)
		}
		if got := frame.Bounds().Size(); got.X != SemanticVideoWidth || got.Y != SemanticVideoHeight {
			t.Fatalf("%s frame size=%v", name, got)
		}
	}
}

func TestHeadlessSemanticRendererIsDeterministicAtExplicitTimestamp(t *testing.T) {
	renderer, err := NewPublicSemanticRenderer()
	if err != nil {
		t.Fatal(err)
	}
	state := semanticFixtureStates(t)["overworld"]
	a, err := renderer.Render(state, SemanticRenderOptions{Width: 640, Height: 360, AtMS: 240})
	if err != nil {
		t.Fatal(err)
	}
	b, err := renderer.Render(state, SemanticRenderOptions{Width: 640, Height: 360, AtMS: 240})
	if err != nil {
		t.Fatal(err)
	}
	ha := sha256.Sum256(a.Pix)
	hb := sha256.Sum256(b.Pix)
	if ha != hb {
		t.Fatal("same semantic state/timestamp produced different pixels")
	}
	c, err := renderer.Render(state, SemanticRenderOptions{Width: 640, Height: 360, AtMS: 760})
	if err != nil {
		t.Fatal(err)
	}
	hc := sha256.Sum256(c.Pix)
	if ha == hc {
		t.Fatal("animated water did not respond to explicit presentation timestamp")
	}
}

func TestHeadlessSemanticRendererLoadsBrowserTinyTownAtlas(t *testing.T) {
	renderer, err := NewPublicSemanticRenderer()
	if err != nil {
		t.Fatal(err)
	}
	img := renderer.semanticAssetImage("/theme-assets/kenney-tiny-town/tilemap.png#tile=1,0,16")
	if img == nil || img.Bounds().Dx() <= 16 || img.Bounds().Dy() <= 16 {
		t.Fatalf("public Tiny Town atlas unavailable: %v", img)
	}
	if renderer.theme.ID != SemanticThemeID || renderer.theme.Distribution != "public" || renderer.theme.AllowGameArtFallbacks {
		t.Fatalf("unsafe or unexpected headless theme: id=%q distribution=%q game_art=%v", renderer.theme.ID, renderer.theme.Distribution, renderer.theme.AllowGameArtFallbacks)
	}
}

func TestHeadlessSemanticRendererRejectsUnsupportedScene(t *testing.T) {
	renderer, err := NewPublicSemanticRenderer()
	if err != nil {
		t.Fatal(err)
	}
	state := semanticFixtureStates(t)["overworld"]
	state.Scene = protocol.SceneTransition
	if _, err := renderer.Render(state, SemanticRenderOptions{}); !errors.Is(err, ErrUnsupportedSemanticScene) {
		t.Fatalf("unsupported scene err=%v", err)
	}
}

func TestClassicRGBFallbackProducesVideoSurface(t *testing.T) {
	rgb := bytes.Repeat([]byte{0x20, 0x80, 0xe0}, 160*144)
	frame, err := RenderClassicRGB(rgb, 160, 144, SemanticVideoWidth, SemanticVideoHeight)
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.Bounds().Size(); got.X != SemanticVideoWidth || got.Y != SemanticVideoHeight {
		t.Fatalf("fallback size=%v", got)
	}
	// 160:144 scales to 400x360, so the 120px side bars remain black.
	if got := frame.RGBAAt(10, 180); got.R != 0 || got.G != 0 || got.B != 0 {
		t.Fatalf("fallback side bar=%v, want black", got)
	}
	if got := frame.RGBAAt(320, 180); got.R == 0 && got.G == 0 && got.B == 0 {
		t.Fatalf("fallback center unexpectedly black: %v", got)
	}
}
