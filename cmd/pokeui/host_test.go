package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/sites"
)

func TestOperatorUIConfigUsesConfiguredPublicURL(t *testing.T) {
	t.Setenv(sites.EnvPublicBaseURL, "https://rompilot.app")
	t.Setenv(sites.EnvAdminBaseURL, "https://admin.rompilot.app")
	t.Setenv(sites.EnvAPIBaseURL, "https://api.rompilot.app")
	t.Setenv(sites.EnvSpectatorURL, "https://pokemon.maestroi.cc")

	ui := httptest.NewServer(handler("http://wall.example"))
	t.Cleanup(ui.Close)

	res, err := http.Get(ui.URL + "/v1/ui-config")
	if err != nil {
		t.Fatalf("ui-config: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var got map[string]string
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["spectator_url"] != "https://rompilot.app" {
		t.Fatalf("spectator_url = %q", got["spectator_url"])
	}
	if got["public_base_url"] != "https://rompilot.app" {
		t.Fatalf("public_base_url = %q", got["public_base_url"])
	}
	if got["admin_base_url"] != "https://admin.rompilot.app" {
		t.Fatalf("admin_base_url = %q", got["admin_base_url"])
	}
	if got["api_base_url"] != "https://api.rompilot.app" {
		t.Fatalf("api_base_url = %q", got["api_base_url"])
	}
	for _, value := range got {
		if strings.Contains(value, "pokemon.maestroi.cc") || strings.Contains(value, "maestroi.cc") {
			t.Fatalf("ui-config still names the legacy host: %v", got)
		}
	}
}

func TestOperatorUIConfigStaysEmptyLocally(t *testing.T) {
	t.Setenv(sites.EnvPublicBaseURL, "")
	t.Setenv(sites.EnvAdminBaseURL, "")
	t.Setenv(sites.EnvAPIBaseURL, "")
	t.Setenv(sites.EnvSpectatorURL, "")
	t.Setenv(sites.EnvRunBaseURL, "")

	got := operatorUIConfig()
	if got["spectator_url"] != "" || got["public_base_url"] != "" || got["admin_base_url"] != "" || got["api_base_url"] != "" {
		t.Fatalf("local ui-config invented hosts: %v", got)
	}
}

func TestLegacyPublicHostRedirectsToRomPilot(t *testing.T) {
	t.Setenv(sites.EnvPublicBaseURL, "https://rompilot.app")
	h := withExternalHosts(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("legacy host reached the inner handler")
	}))
	req := httptest.NewRequest(http.MethodGet, "http://pokemon.maestroi.cc/runs/123?x=1", nil)
	req.Host = "pokemon.maestroi.cc"
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d", res.Code)
	}
	if got := res.Header().Get("Location"); got != "https://rompilot.app/runs/123?x=1" {
		t.Fatalf("location = %q", got)
	}
}

func TestAPIHostHidesHTMLAndKeepsPublicAPI(t *testing.T) {
	t.Setenv(sites.EnvPublicBaseURL, "https://rompilot.app")
	t.Setenv(sites.EnvAPIBaseURL, "https://api.rompilot.app")
	inner := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		_, _ = io.WriteString(res, "ok:"+req.URL.Path)
	})
	h := withExternalHosts(inner)

	html := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://api.rompilot.app/", nil)
	req.Host = "api.rompilot.app"
	h.ServeHTTP(html, req)
	if html.Code != http.StatusNotFound {
		t.Fatalf("API HTML = %d", html.Code)
	}

	watch := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "https://api.rompilot.app/v1/watch", nil)
	req.Host = "api.rompilot.app"
	h.ServeHTTP(watch, req)
	if watch.Code != http.StatusOK || watch.Body.String() != "ok:/v1/watch" {
		t.Fatalf("API watch = %d %q", watch.Code, watch.Body.String())
	}
}

func TestPublicCORSAllowsOnlyConfiguredOrigins(t *testing.T) {
	t.Setenv(sites.EnvPublicBaseURL, "https://rompilot.app")
	t.Setenv(sites.EnvAPIBaseURL, "https://api.rompilot.app")
	t.Setenv(sites.EnvAdminBaseURL, "https://admin.rompilot.app")
	h := publicCORS(http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(http.StatusOK)
	}))

	allowed := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://api.rompilot.app/v1/watch", nil)
	req.Header.Set("Origin", "https://rompilot.app")
	h.ServeHTTP(allowed, req)
	if got := allowed.Header().Get("Access-Control-Allow-Origin"); got != "https://rompilot.app" {
		t.Fatalf("allowed origin = %q", got)
	}

	denied := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "https://api.rompilot.app/v1/watch", nil)
	req.Header.Set("Origin", "https://admin.rompilot.app")
	h.ServeHTTP(denied, req)
	if got := denied.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("admin origin authorized public CORS: %q", got)
	}

	preflight := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodOptions, "https://api.rompilot.app/v1/watch", nil)
	req.Header.Set("Origin", "https://evil.example")
	h.ServeHTTP(preflight, req)
	if preflight.Code != http.StatusForbidden {
		t.Fatalf("unknown origin preflight = %d", preflight.Code)
	}
}

func TestAdminCORSDoesNotAllowPublicOrigin(t *testing.T) {
	t.Setenv(sites.EnvPublicBaseURL, "https://rompilot.app")
	t.Setenv(sites.EnvAdminBaseURL, "https://admin.rompilot.app")
	h := adminCORS(http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(http.StatusOK)
	}))

	ok := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "https://admin.rompilot.app/v1/specs", nil)
	req.Header.Set("Origin", "https://admin.rompilot.app")
	h.ServeHTTP(ok, req)
	if got := ok.Header().Get("Access-Control-Allow-Origin"); got != "https://admin.rompilot.app" {
		t.Fatalf("admin origin = %q", got)
	}
	if ok.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("admin CORS must not share credentials with other hosts")
	}

	denied := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "https://admin.rompilot.app/v1/specs", nil)
	req.Header.Set("Origin", "https://rompilot.app")
	h.ServeHTTP(denied, req)
	if got := denied.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("public origin authorized admin CORS: %q", got)
	}
}
