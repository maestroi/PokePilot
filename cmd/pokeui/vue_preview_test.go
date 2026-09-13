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
	_, err := fs.ReadFile(vueWebAssets, "ui/vue/"+target+"/"+target+".html")
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
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/next/?run=abc", nil))
	if res.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d", res.Code)
	}
	if got := res.Header().Get("Location"); got != "/?run=abc" {
		t.Fatalf("location = %q", got)
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
