package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestVuePokemonAssetProxyServesWhitelistedArtwork(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 1, 2, 3}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if req.URL.Path != "/sprites/master/sprites/pokemon/versions/generation-i/red-blue/17.png" {
			t.Fatalf("upstream path = %q", req.URL.Path)
		}
		res.Header().Set("Content-Type", "image/png")
		_, _ = res.Write(png)
	}))
	t.Cleanup(upstream.Close)

	oldRoot, oldClient := pokemonAssetUpstreamRoot, pokemonAssetClient
	pokemonAssetUpstreamRoot = upstream.URL + "/sprites/master/"
	pokemonAssetClient = upstream.Client()
	t.Cleanup(func() {
		pokemonAssetUpstreamRoot = oldRoot
		pokemonAssetClient = oldClient
	})

	h := withVuePreview(http.NotFoundHandler(), "spectator")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/poke-assets/sprites/pokemon/versions/generation-i/red-blue/17.png", nil))

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Code, res.Body.String())
	}
	if !bytes.Equal(res.Body.Bytes(), png) {
		t.Fatalf("body = %x, want %x", res.Body.Bytes(), png)
	}
	if got := res.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := res.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=86400") {
		t.Fatalf("Cache-Control = %q", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls.Load())
	}
}

func TestPokemonAssetProxyRejectsUnapprovedPaths(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(res, "should not be reached")
	}))
	t.Cleanup(upstream.Close)

	oldRoot, oldClient := pokemonAssetUpstreamRoot, pokemonAssetClient
	pokemonAssetUpstreamRoot = upstream.URL + "/"
	pokemonAssetClient = upstream.Client()
	t.Cleanup(func() {
		pokemonAssetUpstreamRoot = oldRoot
		pokemonAssetClient = oldClient
	})

	for _, path := range []string{
		"/poke-assets/../../README.md",
		"/poke-assets/sprites/pokemon/9999.png",
		"/poke-assets/sprites/badges/9.png",
		"/poke-assets/sprites/items/not-an-image.svg",
	} {
		res := httptest.NewRecorder()
		servePokemonAsset(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, res.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid asset paths reached upstream %d time(s)", calls.Load())
	}
}
