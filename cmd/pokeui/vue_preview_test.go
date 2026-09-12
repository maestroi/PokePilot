package main

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func requireVueBuild(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"ui/vue/operator/operator.html",
		"ui/vue/spectator/spectator.html",
	} {
		if _, err := fs.Stat(vueWebAssets, name); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				t.Skip("Vue assets are not built; run `cd web && npm run build` to exercise embed integration")
			}
			t.Fatalf("stat %s: %v", name, err)
		}
	}
}

func TestVuePreviewServesBuiltEntryAndHashedAsset(t *testing.T) {
	requireVueBuild(t)
	assetPattern := regexp.MustCompile(`(?:src|href)="(/next/assets/[^"]+)"`)

	for _, target := range []string{"operator", "spectator"} {
		t.Run(target, func(t *testing.T) {
			h := withVuePreview(http.NotFoundHandler(), target)
			entryReq := httptest.NewRequest(http.MethodGet, "/next/", nil)
			entryRes := httptest.NewRecorder()
			h.ServeHTTP(entryRes, entryReq)
			if entryRes.Code != http.StatusOK {
				t.Fatalf("GET /next/ = %d, want 200: %s", entryRes.Code, entryRes.Body.String())
			}
			if got := entryRes.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("entry Cache-Control = %q, want no-store", got)
			}
			if !strings.Contains(entryRes.Body.String(), `<div id="app"></div>`) {
				t.Fatal("Vue entry is missing app mount")
			}

			match := assetPattern.FindStringSubmatch(entryRes.Body.String())
			if len(match) != 2 {
				t.Fatalf("Vue entry does not reference a /next/assets bundle: %s", entryRes.Body.String())
			}
			assetReq := httptest.NewRequest(http.MethodGet, match[1], nil)
			assetRes := httptest.NewRecorder()
			h.ServeHTTP(assetRes, assetReq)
			if assetRes.Code != http.StatusOK {
				t.Fatalf("GET %s = %d, want 200", match[1], assetRes.Code)
			}
			if got := assetRes.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
				t.Fatalf("asset Cache-Control = %q, want immutable", got)
			}
		})
	}
}

func TestVuePreviewKeepsOperatorAndSpectatorTreesSeparate(t *testing.T) {
	requireVueBuild(t)

	operator := withVuePreview(http.NotFoundHandler(), "operator")
	res := httptest.NewRecorder()
	operator.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/next/spectator.html", nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("operator GET spectator entry = %d, want 404", res.Code)
	}

	spectator := spectatorSecurityHeaders(withVuePreview(http.NotFoundHandler(), "spectator"))
	res = httptest.NewRecorder()
	spectator.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/next/operator.html", nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("spectator GET operator entry = %d, want 404", res.Code)
	}
	if got := res.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("spectator Vue preview is missing spectator security headers")
	}
}

func TestVuePreviewLeavesLegacyRoutesAlone(t *testing.T) {
	legacy := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("X-Legacy", "yes")
		res.WriteHeader(http.StatusNoContent)
	})
	h := withVuePreview(legacy, "operator")

	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil))
	if res.Code != http.StatusNoContent || res.Header().Get("X-Legacy") != "yes" {
		t.Fatalf("legacy route was intercepted: code=%d header=%q", res.Code, res.Header().Get("X-Legacy"))
	}
}
