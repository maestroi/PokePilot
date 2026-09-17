package main

import (
	"net/http"
	"strings"

	"github.com/maestroi/pokepilot/sites"
)

const publicCORSMethods = "GET, HEAD, OPTIONS"
const adminCORSMethods = "GET, HEAD, POST, PATCH, DELETE, OPTIONS"

func publicCORS(next http.Handler) http.Handler {
	return corsOrigins(func(origin string) bool {
		return sites.FromEnv().AllowsPublicOrigin(origin)
	}, publicCORSMethods, next)
}

func adminCORS(next http.Handler) http.Handler {
	return corsOrigins(func(origin string) bool {
		return sites.FromEnv().AllowsAdminOrigin(origin)
	}, adminCORSMethods, next)
}

func corsOrigins(allow func(string) bool, methods string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		origin := strings.TrimSpace(req.Header.Get("Origin"))
		allowed := origin != "" && allow(origin)
		if allowed {
			res.Header().Set("Access-Control-Allow-Origin", origin)
			res.Header().Add("Vary", "Origin")
			res.Header().Set("Access-Control-Allow-Methods", methods)
			res.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Authorization")
			res.Header().Set("Access-Control-Max-Age", "600")
		}
		if req.Method == http.MethodOptions && origin != "" {
			if allowed {
				res.WriteHeader(http.StatusNoContent)
				return
			}
			res.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(res, req)
	})
}
