// Package sites is the single source of canonical external RomPilot URLs.
//
// Production hostnames are not assumed when the corresponding environment
// variables are empty, so local development keeps working on localhost ports.
package sites

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	EnvPublicBaseURL = "POKEPILOT_PUBLIC_BASE_URL"
	EnvAdminBaseURL  = "POKEPILOT_ADMIN_BASE_URL"
	EnvAPIBaseURL    = "POKEPILOT_API_BASE_URL"
	// EnvSpectatorURL is the compatibility alias for EnvPublicBaseURL.
	EnvSpectatorURL = "POKEPILOT_SPECTATOR_URL"
	// EnvRunBaseURL is the compatibility alias for EnvAdminBaseURL. Issue
	// adapters historically used it for private run/debug links.
	EnvRunBaseURL = "POKEPILOT_RUN_BASE_URL"

	ProductionPublicURL = "https://rompilot.app"
	ProductionAdminURL  = "https://admin.rompilot.app"
	ProductionAPIURL    = "https://api.rompilot.app"

	ProductionPublicHost = "rompilot.app"
	ProductionAdminHost  = "admin.rompilot.app"
	ProductionAPIHost    = "api.rompilot.app"
	WWWPublicHost        = "www.rompilot.app"
	LegacyPublicHost     = "pokemon.maestroi.cc"
	LegacyAdminHost      = "pokemon.labstack.cc"
)

// Config holds the configured external bases. Empty fields mean "not set":
// callers must keep using relative/local URLs instead of inventing production
// hosts.
type Config struct {
	Public string
	Admin  string
	API    string
}

// FromEnv reads the canonical URL variables, including the older spectator
// and run-base aliases. Later, more specific variables win.
func FromEnv() Config {
	return Config{
		Public: firstBase(os.Getenv(EnvPublicBaseURL), os.Getenv(EnvSpectatorURL)),
		Admin:  firstBase(os.Getenv(EnvAdminBaseURL), os.Getenv(EnvRunBaseURL)),
		API:    NormalizeBase(os.Getenv(EnvAPIBaseURL)),
	}
}

// Production is the canonical public deployment. Tests use it to lock the
// generated hosts; processes only get these values when the environment
// actually configures them (or a caller passes this struct explicitly).
func Production() Config {
	return Config{
		Public: ProductionPublicURL,
		Admin:  ProductionAdminURL,
		API:    ProductionAPIURL,
	}
}

func firstBase(values ...string) string {
	for _, raw := range values {
		if base := NormalizeBase(raw); base != "" {
			return base
		}
	}
	return ""
}

// NormalizeBase trims space and a trailing slash. Invalid absolute URLs
// become empty so callers cannot emit half-formed hosts.
func NormalizeBase(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		return ""
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return ""
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func (c Config) PublicBase() string { return NormalizeBase(c.Public) }
func (c Config) AdminBase() string  { return NormalizeBase(c.Admin) }

// APIBase is the external API origin. When unset it falls back to the public
// site, because the spectator process is the public API in the current farm.
func (c Config) APIBase() string {
	if base := NormalizeBase(c.API); base != "" {
		return base
	}
	return c.PublicBase()
}

func (c Config) SpectatorURL() string { return c.PublicBase() }

// RunPageURL is the public spectator page for one run.
func (c Config) RunPageURL(runID string) string {
	return joinPath(c.PublicBase(), RunPath(runID))
}

// ExploreURL is the public world/map explorer.
func (c Config) ExploreURL() string {
	return joinPath(c.PublicBase(), "/explore")
}

// LiveHTTPURL is an absolute live/telemetry HTTP endpoint when an API or
// public base is configured; otherwise it returns the relative path so local
// same-origin clients keep working.
func (c Config) LiveHTTPURL(path string) string {
	return joinPath(c.APIBase(), path)
}

// LiveWebSocketURL converts the live HTTP endpoint to ws/wss. https always
// becomes wss so mixed-content upgrades cannot happen.
func (c Config) LiveWebSocketURL(path string) string {
	return toWebSocketURL(c.LiveHTTPURL(path))
}

// PublicOrigins is the exact CORS allowlist for the public spectator API.
// Local/empty configuration yields no extra origins; browsers then rely on
// same-origin requests to the spectator process.
func (c Config) PublicOrigins() []string {
	return uniqueBases(c.PublicBase(), c.APIBase())
}

// AdminOrigins is the exact CORS allowlist for the private operator API.
// The public site is never included.
func (c Config) AdminOrigins() []string {
	return uniqueBases(c.AdminBase())
}

func (c Config) AllowsPublicOrigin(origin string) bool {
	return containsBase(c.PublicOrigins(), origin)
}

func (c Config) AllowsAdminOrigin(origin string) bool {
	return containsBase(c.AdminOrigins(), origin)
}

// LegacyPublicRedirect returns a permanent public URL when the request host
// is the retired spectator hostname or www. Path and query are preserved.
func (c Config) LegacyPublicRedirect(host, path, rawQuery string) (string, bool) {
	base := c.PublicBase()
	if base == "" {
		return "", false
	}
	switch hostname(host) {
	case LegacyPublicHost, WWWPublicHost:
	default:
		return "", false
	}
	if path == "" {
		path = "/"
	}
	target := joinPath(base, path)
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	return target, true
}

func (c Config) IsAPIHost(host string) bool {
	apiHost := hostnameFromBase(c.APIBase())
	if apiHost == "" || apiHost == hostnameFromBase(c.PublicBase()) {
		return false
	}
	return hostname(host) == apiHost
}

func (c Config) IsLegacyPublicHost(host string) bool {
	return hostname(host) == LegacyPublicHost
}

// RunPath is the public path for one run. Empty IDs stay on the homepage.
func RunPath(runID string) string {
	id := strings.TrimSpace(runID)
	if id == "" {
		return "/"
	}
	return "/runs/" + url.PathEscape(id)
}

// CanonicalRunRedirect maps the historical `/?run=` spectator links onto
// `/runs/:id` while preserving any extra query string.
func CanonicalRunRedirect(path string, query url.Values) (string, bool) {
	if path != "/" && path != "" {
		return "", false
	}
	runID := strings.TrimSpace(query.Get("run"))
	if runID == "" {
		return "", false
	}
	next := url.Values{}
	for key, values := range query {
		if key == "run" {
			continue
		}
		for _, value := range values {
			next.Add(key, value)
		}
	}
	target := RunPath(runID)
	if encoded := next.Encode(); encoded != "" {
		target += "?" + encoded
	}
	return target, true
}

// LocalSpectatorBase maps the local operator port onto the spectator port.
// Production hosts are never invented here.
func LocalSpectatorBase(current string) string {
	parsed, err := url.Parse(strings.TrimSpace(current))
	if err != nil || parsed.Host == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil || port != "18080" {
		return ""
	}
	parsed.Host = net.JoinHostPort(host, "18081")
	parsed.Path = "/"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func toWebSocketURL(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "/") {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	case "wss", "ws":
	default:
		return ""
	}
	return parsed.String()
}

func joinPath(base, path string) string {
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if base == "" {
		return path
	}
	if path == "/" {
		return base + "/"
	}
	return base + path
}

func uniqueBases(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		base := NormalizeBase(raw)
		if base == "" {
			continue
		}
		if _, ok := seen[base]; ok {
			continue
		}
		seen[base] = struct{}{}
		out = append(out, base)
	}
	return out
}

func containsBase(bases []string, origin string) bool {
	want := NormalizeBase(origin)
	if want == "" {
		return false
	}
	for _, base := range bases {
		if base == want {
			return true
		}
	}
	return false
}

func hostname(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return strings.ToLower(h)
	}
	return strings.ToLower(host)
}

func hostnameFromBase(base string) string {
	parsed, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return hostname(parsed.Host)
}

// RequestHost is the HTTP host without a port, used by Traefik-fronted tests.
func RequestHost(req *http.Request) string {
	if req == nil {
		return ""
	}
	if host := req.Header.Get("X-Forwarded-Host"); host != "" {
		return hostname(host)
	}
	return hostname(req.Host)
}
