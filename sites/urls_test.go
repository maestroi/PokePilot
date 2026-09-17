package sites

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestProductionURLsUseRomPilotHosts(t *testing.T) {
	cfg := Production()
	if cfg.PublicBase() != ProductionPublicURL {
		t.Fatalf("public = %q", cfg.PublicBase())
	}
	if cfg.AdminBase() != ProductionAdminURL {
		t.Fatalf("admin = %q", cfg.AdminBase())
	}
	if cfg.APIBase() != ProductionAPIURL {
		t.Fatalf("api = %q", cfg.APIBase())
	}

	runURL := cfg.RunPageURL("run-123")
	if runURL != "https://rompilot.app/runs/run-123" {
		t.Fatalf("run page = %q", runURL)
	}
	if strings.Contains(runURL, LegacyPublicHost) || strings.Contains(runURL, "maestroi.cc") {
		t.Fatalf("public run URL still names the legacy host: %q", runURL)
	}
	if cfg.ExploreURL() != "https://rompilot.app/explore" {
		t.Fatalf("explore = %q", cfg.ExploreURL())
	}
	if !strings.HasPrefix(cfg.AdminBase(), "https://admin.rompilot.app") {
		t.Fatalf("admin base = %q", cfg.AdminBase())
	}
}

func TestFromEnvPrefersSpecificVariablesAndKeepsAliases(t *testing.T) {
	t.Setenv(EnvPublicBaseURL, "https://rompilot.app/")
	t.Setenv(EnvSpectatorURL, "https://pokemon.maestroi.cc")
	t.Setenv(EnvAdminBaseURL, "https://admin.rompilot.app")
	t.Setenv(EnvRunBaseURL, "https://pokemon.labstack.cc")
	t.Setenv(EnvAPIBaseURL, "https://api.rompilot.app")

	cfg := FromEnv()
	if cfg.PublicBase() != ProductionPublicURL {
		t.Fatalf("public alias lost to legacy spectator URL: %q", cfg.PublicBase())
	}
	if cfg.AdminBase() != ProductionAdminURL {
		t.Fatalf("admin alias lost to legacy run URL: %q", cfg.AdminBase())
	}
	if cfg.SpectatorURL() != ProductionPublicURL {
		t.Fatalf("spectator alias = %q", cfg.SpectatorURL())
	}
}

func TestFromEnvFallsBackToLegacyAliases(t *testing.T) {
	t.Setenv(EnvPublicBaseURL, "")
	t.Setenv(EnvAdminBaseURL, "")
	t.Setenv(EnvAPIBaseURL, "")
	t.Setenv(EnvSpectatorURL, "https://rompilot.app")
	t.Setenv(EnvRunBaseURL, "https://admin.rompilot.app")

	cfg := FromEnv()
	if cfg.PublicBase() != ProductionPublicURL || cfg.AdminBase() != ProductionAdminURL {
		t.Fatalf("alias fallback = %+v", cfg)
	}
	if cfg.APIBase() != ProductionPublicURL {
		t.Fatalf("api fallback to public = %q", cfg.APIBase())
	}
}

func TestEmptyConfigStaysLocal(t *testing.T) {
	t.Setenv(EnvPublicBaseURL, "")
	t.Setenv(EnvAdminBaseURL, "")
	t.Setenv(EnvAPIBaseURL, "")
	t.Setenv(EnvSpectatorURL, "")
	t.Setenv(EnvRunBaseURL, "")

	cfg := FromEnv()
	if cfg.PublicBase() != "" || cfg.AdminBase() != "" || cfg.APIBase() != "" {
		t.Fatalf("empty env invented production hosts: %+v", cfg)
	}
	if got := cfg.RunPageURL("run-1"); got != "/runs/run-1" {
		t.Fatalf("local run path = %q", got)
	}
	if got := cfg.LiveHTTPURL("/v1/watch"); got != "/v1/watch" {
		t.Fatalf("local live HTTP = %q", got)
	}
	if got := cfg.LiveWebSocketURL("/v1/watch/live"); got != "/v1/watch/live" {
		t.Fatalf("local live WS path = %q", got)
	}
	if len(cfg.PublicOrigins()) != 0 || len(cfg.AdminOrigins()) != 0 {
		t.Fatalf("local CORS invented origins public=%v admin=%v", cfg.PublicOrigins(), cfg.AdminOrigins())
	}
}

func TestLiveWebSocketUsesMatchingScheme(t *testing.T) {
	cfg := Production()
	got := cfg.LiveWebSocketURL("/v1/watch/live")
	if got != "wss://api.rompilot.app/v1/watch/live" {
		t.Fatalf("production WS = %q", got)
	}

	httpCfg := Config{Public: "http://localhost:18081"}
	got = httpCfg.LiveWebSocketURL("/v1/watch/live")
	if got != "ws://localhost:18081/v1/watch/live" {
		t.Fatalf("local WS = %q", got)
	}

	httpsCfg := Config{API: "https://api.rompilot.app"}
	if got := httpsCfg.LiveWebSocketURL("/v1/watch"); !strings.HasPrefix(got, "wss://") {
		t.Fatalf("https API must mint wss, got %q", got)
	}
}

func TestLocalSpectatorPortMapping(t *testing.T) {
	got := LocalSpectatorBase("http://127.0.0.1:18080/#live")
	if got != "http://127.0.0.1:18081" {
		t.Fatalf("local spectator = %q", got)
	}
	if got := LocalSpectatorBase("https://admin.rompilot.app/"); got != "" {
		t.Fatalf("production admin must not invent a spectator port: %q", got)
	}
}

func TestLegacyPublicRedirectPreservesPath(t *testing.T) {
	cfg := Production()
	got, ok := cfg.LegacyPublicRedirect("pokemon.maestroi.cc", "/runs/123", "x=1")
	if !ok || got != "https://rompilot.app/runs/123?x=1" {
		t.Fatalf("legacy redirect = %q ok=%t", got, ok)
	}
	got, ok = cfg.LegacyPublicRedirect("www.rompilot.app", "/", "")
	if !ok || got != "https://rompilot.app/" {
		t.Fatalf("www redirect = %q ok=%t", got, ok)
	}
	if _, ok := cfg.LegacyPublicRedirect("rompilot.app", "/", ""); ok {
		t.Fatal("canonical public host should not redirect")
	}
	empty := Config{}
	if _, ok := empty.LegacyPublicRedirect(LegacyPublicHost, "/runs/1", ""); ok {
		t.Fatal("empty config must not invent a production redirect")
	}
}

func TestCanonicalRunQueryRedirect(t *testing.T) {
	query := url.Values{"run": {"run-9"}, "debug": {"1"}}
	got, ok := CanonicalRunRedirect("/", query)
	if !ok || got != "/runs/run-9?debug=1" {
		t.Fatalf("canonical run = %q ok=%t", got, ok)
	}
	if _, ok := CanonicalRunRedirect("/explore", query); ok {
		t.Fatal("non-root paths should keep their own query")
	}
}

func TestCORSAllowlistsAreExactAndSeparated(t *testing.T) {
	cfg := Production()
	public := cfg.PublicOrigins()
	admin := cfg.AdminOrigins()
	if len(public) != 2 || public[0] != ProductionPublicURL || public[1] != ProductionAPIURL {
		t.Fatalf("public origins = %v", public)
	}
	if len(admin) != 1 || admin[0] != ProductionAdminURL {
		t.Fatalf("admin origins = %v", admin)
	}
	if cfg.AllowsAdminOrigin(ProductionPublicURL) {
		t.Fatal("public origin must not authorize admin CORS")
	}
	if cfg.AllowsPublicOrigin(ProductionAdminURL) {
		t.Fatal("admin origin must not authorize public CORS")
	}
	if cfg.AllowsPublicOrigin("https://evil.example") || cfg.AllowsPublicOrigin("*") {
		t.Fatal("wildcard/unknown public origin allowed")
	}
}

func TestAPIHostDetection(t *testing.T) {
	cfg := Production()
	if !cfg.IsAPIHost("api.rompilot.app") || !cfg.IsAPIHost("api.rompilot.app:443") {
		t.Fatal("api host not detected")
	}
	if cfg.IsAPIHost("rompilot.app") || cfg.IsAPIHost("admin.rompilot.app") {
		t.Fatal("non-api hosts classified as API")
	}
	same := Config{Public: ProductionPublicURL, API: ProductionPublicURL}
	if same.IsAPIHost(ProductionAPIHost) {
		t.Fatal("API host detection must stay off when API shares the public origin")
	}
}

func TestRequestHostPrefersForwardedHost(t *testing.T) {
	req := httptestRequest("pokemon.maestroi.cc")
	req.Header.Set("X-Forwarded-Host", "pokemon.maestroi.cc")
	if got := RequestHost(req); got != LegacyPublicHost {
		t.Fatalf("forwarded host = %q", got)
	}
}

func httptestRequest(host string) *http.Request {
	req, err := http.NewRequest(http.MethodGet, "http://"+host+"/runs/1", nil)
	if err != nil {
		panic(err)
	}
	req.Host = host
	return req
}
