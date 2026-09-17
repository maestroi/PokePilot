package main

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/maestroi/pokepilot/sites"
)

func withExternalHosts(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		cfg := sites.FromEnv()
		host := sites.RequestHost(req)
		if target, ok := cfg.LegacyPublicRedirect(host, req.URL.Path, req.URL.RawQuery); ok {
			http.Redirect(res, req, target, http.StatusMovedPermanently)
			return
		}
		if cfg.IsAPIHost(host) && !publicAPIPath(req.URL.Path) {
			http.NotFound(res, req)
			return
		}
		next.ServeHTTP(res, req)
	})
}

func publicAPIPath(path string) bool {
	switch {
	case path == "/v1/watch", strings.HasPrefix(path, "/v1/watch/"):
		return true
	case path == "/frame":
		return true
	case strings.HasPrefix(path, "/maps/"):
		return true
	case path == "/v1/version", path == "/v1/build":
		return true
	default:
		return false
	}
}

func spectatorRunID(path string) (string, bool) {
	const prefix = "/runs/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	id, err := url.PathUnescape(rest)
	if err != nil || strings.TrimSpace(id) == "" {
		return "", false
	}
	return id, true
}

func operatorUIConfig() map[string]string {
	cfg := sites.FromEnv()
	return map[string]string{
		"spectator_url":   cfg.SpectatorURL(),
		"public_base_url": cfg.PublicBase(),
		"admin_base_url":  cfg.AdminBase(),
		"api_base_url":    cfg.APIBase(),
	}
}
