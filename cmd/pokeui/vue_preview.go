package main

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

const pokemonSpriteImageOrigin = "https://raw.githubusercontent.com"

// vueWebAssets is populated by `cd web && npm run build` before production
// pokeui compilation. A committed placeholder keeps ordinary Go-only builds
// valid, but the browser surface now requires built Vue assets at runtime.
//
//go:embed ui/vue
var vueWebAssets embed.FS

// withVuePreview retains its historic name for compatibility with existing
// callers, but the migration is complete: Vue owns root and the legacy console
// is no longer exposed as a browser fallback.
func withVuePreview(next http.Handler, target string) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if target == "spectator" {
			allowPokemonSpriteImages(res.Header())
		}
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/":
			if serveVueFile(res, req, target, target+".html") {
				return
			}
			res.Header().Set("Cache-Control", "no-store")
			http.Error(res, "frontend assets unavailable; run the web build before starting pokeui", http.StatusServiceUnavailable)
			return
		case req.Method == http.MethodGet && target == "spectator" && (req.URL.Path == "/replays" || strings.HasPrefix(req.URL.Path, "/replays/")):
			if serveVueFile(res, req, target, "replays.html") {
				return
			}
			res.Header().Set("Cache-Control", "no-store")
			http.Error(res, "replay library assets unavailable; run the spectator web build before starting pokeui", http.StatusServiceUnavailable)
			return
		case req.Method == http.MethodGet && (req.URL.Path == "/next" || req.URL.Path == "/next/"):
			destination := "/"
			if req.URL.RawQuery != "" {
				destination += "?" + req.URL.RawQuery
			}
			http.Redirect(res, req, destination, http.StatusPermanentRedirect)
			return
		case req.Method == http.MethodGet && req.URL.Path == "/v1/build":
			res.Header().Set("Content-Type", "application/json")
			res.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(res).Encode(currentBuildProvenance())
			return
		case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/poke-assets/"):
			servePokemonAsset(res, req)
			return
		case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/assets/"):
			name := strings.TrimPrefix(req.URL.Path, "/")
			if !serveVueFile(res, req, target, name) {
				http.NotFound(res, req)
			}
			return
		default:
			next.ServeHTTP(res, req)
		}
	})
}

func allowPokemonSpriteImages(headers http.Header) {
	const imageDirective = "img-src 'self' data: blob:"
	csp := headers.Get("Content-Security-Policy")
	if csp == "" || strings.Contains(csp, pokemonSpriteImageOrigin) || !strings.Contains(csp, imageDirective) {
		return
	}
	headers.Set("Content-Security-Policy", strings.Replace(csp, imageDirective, imageDirective+" "+pokemonSpriteImageOrigin, 1))
}

func serveVueFile(res http.ResponseWriter, req *http.Request, target, name string) bool {
	clean := strings.TrimPrefix(path.Clean("/"+name), "/")
	if clean == "" || clean == "." || strings.HasPrefix(clean, "../") {
		return false
	}

	assetPath := path.Join("ui/vue", target, clean)
	data, err := fs.ReadFile(vueWebAssets, assetPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false
		}
		return false
	}

	if contentType := mime.TypeByExtension(path.Ext(clean)); contentType != "" {
		res.Header().Set("Content-Type", contentType)
	}
	if strings.HasSuffix(clean, ".html") {
		res.Header().Set("Cache-Control", "no-store")
	} else if strings.HasPrefix(clean, "assets/") {
		res.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		res.Header().Set("Cache-Control", "no-cache")
	}
	res.WriteHeader(http.StatusOK)
	_, _ = res.Write(data)
	return true
}
