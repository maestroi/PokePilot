package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSpectatorVueCSPAllowsPokemonSpriteHost(t *testing.T) {
	h := spectatorSecurityHeaders(withVuePreview(http.NotFoundHandler(), "spectator"))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/build", nil))

	got := res.Header().Get("Content-Security-Policy")
	want := "img-src 'self' data: blob: " + pokemonSpriteImageOrigin
	if !strings.Contains(got, want) {
		t.Fatalf("spectator CSP = %q, want %q", got, want)
	}
	if strings.Contains(got, "img-src https:") {
		t.Fatalf("spectator CSP = %q, sprite access should stay pinned to the approved host", got)
	}
}

func TestPokemonSpriteCSPRewriteIsIdempotent(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; script-src 'self'")
	allowPokemonSpriteImages(headers)
	allowPokemonSpriteImages(headers)

	got := headers.Get("Content-Security-Policy")
	if strings.Count(got, pokemonSpriteImageOrigin) != 1 {
		t.Fatalf("sprite origin count in CSP = %d: %q", strings.Count(got, pokemonSpriteImageOrigin), got)
	}
}
