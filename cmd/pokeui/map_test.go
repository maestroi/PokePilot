package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPokeuiServesLiveMapUIAndEmbeddedAssetDirectory(t *testing.T) {
	ui := httptest.NewServer(handler("http://127.0.0.1:1"))
	t.Cleanup(ui.Close)

	res, err := http.Get(ui.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	for _, want := range []string{
		`id="detail-map-panel"`,
		`id="detail-map"`,
		`class="map-legend"`,
		`url.startsWith("/maps/")`,
		`cache: "no-store"`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("index missing %q", want)
		}
	}

	res, err = http.Get(ui.URL + "/ui.js")
	if err != nil {
		t.Fatal(err)
	}
	js, _ := io.ReadAll(res.Body)
	res.Body.Close()
	for _, want := range []string{`/maps/`, `getContext("2d")`, `run.sprites`, `run.trail`} {
		if !bytes.Contains(js, []byte(want)) {
			t.Errorf("ui.js missing %q", want)
		}
	}

	res, err = http.Get(ui.URL + "/maps/README.md")
	if err != nil {
		t.Fatal(err)
	}
	asset, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /maps/README.md = %d, want 200", res.StatusCode)
	}
	if !bytes.Contains(asset, []byte("Semantic map assets")) {
		t.Errorf("embedded map directory returned unexpected body: %s", asset)
	}
}

func TestPokeuiServesGeneratedSemanticMap(t *testing.T) {
	ui := httptest.NewServer(handler("http://127.0.0.1:1"))
	t.Cleanup(ui.Close)

	res, err := http.Get(ui.URL + "/maps/28.json")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /maps/28.json = %d, want 200", res.StatusCode)
	}

	var asset struct {
		ID       int    `json:"id"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Cells    string `json:"cells"`
		Fallback bool   `json:"fallback"`
	}
	if err := json.NewDecoder(res.Body).Decode(&asset); err != nil {
		t.Fatal(err)
	}
	if asset.ID != 0x28 || asset.Width <= 0 || asset.Height <= 0 {
		t.Fatalf("unexpected Oaks Lab asset: id=%#x size=%dx%d", asset.ID, asset.Width, asset.Height)
	}
	if len(asset.Cells) != asset.Width*asset.Height {
		t.Fatalf("cells length = %d, want %d", len(asset.Cells), asset.Width*asset.Height)
	}
	if asset.Fallback {
		t.Fatal("generated semantic map unexpectedly marked fallback")
	}
}

func TestPokeuiServesFallbackMapWhenSemanticAssetIsMissing(t *testing.T) {
	ui := httptest.NewServer(handler("http://127.0.0.1:1"))
	t.Cleanup(ui.Close)

	// Use a deliberately non-generated two-character map key so this test
	// keeps exercising fallback behavior even after real ROM assets are added.
	res, err := http.Get(ui.URL + "/maps/zz.json")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /maps/zz.json = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var asset struct {
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Cells    string `json:"cells"`
		Fallback bool   `json:"fallback"`
	}
	if err := json.NewDecoder(res.Body).Decode(&asset); err != nil {
		t.Fatal(err)
	}
	if asset.Width != fallbackMapSize || asset.Height != fallbackMapSize {
		t.Fatalf("fallback dimensions = %dx%d, want %dx%d", asset.Width, asset.Height, fallbackMapSize, fallbackMapSize)
	}
	if len(asset.Cells) != fallbackMapSize*fallbackMapSize {
		t.Fatalf("fallback cells length = %d, want %d", len(asset.Cells), fallbackMapSize*fallbackMapSize)
	}
	if !asset.Fallback {
		t.Fatal("fallback marker is false")
	}
}
