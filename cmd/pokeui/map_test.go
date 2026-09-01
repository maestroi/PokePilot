package main

import (
	"bytes"
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
	for _, want := range []string{`id="detail-map-panel"`, `id="detail-map"`, `class="map-legend"`} {
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
