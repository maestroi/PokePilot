package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func vueBuilt(t *testing.T, target string) bool {
	t.Helper()
	return vueFileExists(target, target+".html")
}

func vueFileExists(target, name string) bool {
	_, err := fs.ReadFile(vueWebAssets, "ui/vue/"+target+"/"+name)
	return err == nil
}

func TestVueRootRequiresFrontendAndLegacyIsGone(t *testing.T) {
	legacy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte("legacy-root"))
			return
		}
		http.NotFound(w, r)
	})
	h := withVuePreview(legacy, "operator")

	root := httptest.NewRecorder()
	h.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/", nil))
	if vueBuilt(t, "operator") {
		if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), "id=\"app\"") {
			t.Fatalf("Vue root = %d %q", root.Code, root.Body.String())
		}
		if got := root.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Vue HTML cache = %q", got)
		}
	} else if root.Code != http.StatusServiceUnavailable || strings.Contains(root.Body.String(), "legacy-root") {
		t.Fatalf("missing frontend = %d %q", root.Code, root.Body.String())
	}

	fallback := httptest.NewRecorder()
	h.ServeHTTP(fallback, httptest.NewRequest(http.MethodGet, "/legacy/", nil))
	if fallback.Code != http.StatusNotFound {
		t.Fatalf("legacy route = %d, want 404", fallback.Code)
	}
}

func TestVueTargetsStaySeparated(t *testing.T) {
	if !vueBuilt(t, "operator") || !vueBuilt(t, "spectator") {
		t.Skip("frontend bundles not built in this Go-only checkout")
	}

	for _, tc := range []struct {
		target string
		own    string
		other  string
	}{
		{target: "operator", own: "operator.html", other: "spectator.html"},
		{target: "spectator", own: "spectator.html", other: "operator.html"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			own := httptest.NewRecorder()
			if !serveVueFile(own, httptest.NewRequest(http.MethodGet, "/", nil), tc.target, tc.own) {
				t.Fatalf("%s entry was not served", tc.target)
			}
			other := httptest.NewRecorder()
			if serveVueFile(other, httptest.NewRequest(http.MethodGet, "/", nil), tc.target, tc.other) {
				t.Fatalf("%s surface served %s entry", tc.target, tc.other)
			}
		})
	}
}

func TestVueNextRedirectsToRoot(t *testing.T) {
	h := withVuePreview(http.NotFoundHandler(), "operator")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/next/?debug=1", nil))
	if res.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d", res.Code)
	}
	if got := res.Header().Get("Location"); got != "/?debug=1" {
		t.Fatalf("location = %q", got)
	}
}

func TestVueNextRunQueryRedirectsToRunPage(t *testing.T) {
	h := withVuePreview(http.NotFoundHandler(), "spectator")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/next/?run=abc", nil))
	if res.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d", res.Code)
	}
	if got := res.Header().Get("Location"); got != "/runs/abc" {
		t.Fatalf("location = %q", got)
	}
}

func TestVueSpectatorCanonicalizesRunQuery(t *testing.T) {
	h := withVuePreview(http.NotFoundHandler(), "spectator")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/?run=run-9", nil))
	if res.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d", res.Code)
	}
	if got := res.Header().Get("Location"); got != "/runs/run-9" {
		t.Fatalf("location = %q", got)
	}
}

func TestVueSpectatorServesPublicPages(t *testing.T) {
	if !vueFileExists("spectator", "spectator.html") {
		t.Skip("spectator frontend bundle not built in this Go-only checkout")
	}
	h := withVuePreview(http.NotFoundHandler(), "spectator")
	pages := []string{"/"}
	if vueFileExists("spectator", "world.html") {
		pages = append(pages, "/world", "/explore")
	}
	if vueFileExists("spectator", "replays.html") {
		pages = append(pages, "/replays")
	}
	pages = append(pages, "/runs/run-live")
	for _, page := range pages {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, page, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("%s = %d: %s", page, res.Code, res.Body.String())
		}
		if !strings.Contains(res.Body.String(), "id=\"app\"") {
			t.Fatalf("%s did not serve Vue entry: %q", page, res.Body.String())
		}
	}
}

func TestVueBuildProvenanceEndpoint(t *testing.T) {
	oldVersion, oldPR, oldTitle := version, buildPR, buildTitleB64
	defer func() { version, buildPR, buildTitleB64 = oldVersion, oldPR, oldTitle }()
	version = "0123456789abcdef"
	buildPR = "270"
	buildTitleB64 = ""

	h := withVuePreview(http.NotFoundHandler(), "operator")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/build", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Code, res.Body.String())
	}
	var got buildProvenance
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode build provenance: %v", err)
	}
	if got.Version != version || got.PRNumber != "270" || got.PRURL == "" || got.CommitURL == "" {
		t.Fatalf("build provenance = %#v", got)
	}
}

func TestVueSpectatorServesWorldExplorer(t *testing.T) {
	if !vueFileExists("spectator", "world.html") {
		t.Skip("spectator frontend bundle not built in this Go-only checkout")
	}
	h := withVuePreview(http.NotFoundHandler(), "spectator")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/world?map=ROUTE_13&x=49&y=8&debug=1", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("world route = %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "id=\"app\"") {
		t.Fatalf("world route did not serve Vue entry: %q", res.Body.String())
	}
}

func TestVueEmbedIncludesUnderscorePrefixedAssets(t *testing.T) {
	_, err := fs.ReadFile(vueWebAssets, "ui/vue/_underscore_embed_probe.txt")
	if err != nil {
		t.Fatalf("go:embed dropped underscore-prefixed Vue assets: %v", err)
	}
}

func TestVueSpectatorHTMLAssetsAreEmbedded(t *testing.T) {
	if !vueBuilt(t, "spectator") {
		t.Skip("spectator frontend bundle not built in this Go-only checkout")
	}

	h := withVuePreview(http.NotFoundHandler(), "spectator")
	pages := []string{"/"}
	if vueFileExists("spectator", "world.html") {
		pages = append(pages, "/world")
	}
	if vueFileExists("spectator", "replays.html") {
		pages = append(pages, "/replays")
	}
	seen := map[string]bool{}
	for _, page := range pages {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, page, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("%s = %d: %s", page, res.Code, res.Body.String())
		}
		for _, asset := range htmlAssetRefs(res.Body.String()) {
			if seen[asset] {
				continue
			}
			seen[asset] = true
			assetRes := httptest.NewRecorder()
			h.ServeHTTP(assetRes, httptest.NewRequest(http.MethodGet, asset, nil))
			if assetRes.Code != http.StatusOK {
				t.Fatalf("%s referenced %s, got %d", page, asset, assetRes.Code)
			}
		}
	}
}

func htmlAssetRefs(html string) []string {
	refs := make([]string, 0, 8)
	for _, prefix := range []string{`src="`, `href="`} {
		rest := html
		for {
			start := strings.Index(rest, prefix)
			if start < 0 {
				break
			}
			rest = rest[start+len(prefix):]
			end := strings.Index(rest, `"`)
			if end < 0 {
				break
			}
			ref := rest[:end]
			rest = rest[end+1:]
			if strings.HasPrefix(ref, "/assets/") {
				refs = append(refs, ref)
			}
		}
	}
	return refs
}


func TestVueSpectatorServesGen1RenderAssets(t *testing.T) {
	if !vueFileExists("spectator", "gen1/red/maps/PalletTown.blk") {
		t.Skip("spectator Gen 1 render assets not built in this Go-only checkout")
	}
	h := withVuePreview(http.NotFoundHandler(), "spectator")
	for _, asset := range []string{
		"/gen1/red/maps/PalletTown.blk",
		"/gen1/red/blocksets/overworld.bst",
		"/gen1/red/tilesets/overworld.png",
	} {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, asset, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("%s = %d: %s", asset, res.Code, res.Body.String())
		}
		if res.Body.Len() == 0 {
			t.Fatalf("%s served an empty body", asset)
		}
	}
}
