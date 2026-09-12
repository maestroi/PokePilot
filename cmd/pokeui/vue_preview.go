package main

import (
	"embed"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

// vueWebAssets is populated by `cd web && npm run build` before production
// pokeui compilation. A committed placeholder keeps ordinary Go-only builds
// valid; in that case root transparently falls back to the legacy UI.
//
//go:embed ui/vue
var vueWebAssets embed.FS

// withVuePreview retains the historic helper name from the incremental
// migration, but Vue is now the primary root UI. The old console remains at
// /legacy/ as a temporary safety hatch while the cutover is validated.
func withVuePreview(next http.Handler, target string) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/":
			if serveVueFile(res, req, target, target+".html") {
				return
			}
			next.ServeHTTP(res, req)
			return
		case req.Method == http.MethodGet && req.URL.Path == "/legacy":
			http.Redirect(res, req, "/legacy/", http.StatusTemporaryRedirect)
			return
		case strings.HasPrefix(req.URL.Path, "/legacy/"):
			clone := req.Clone(req.Context())
			urlCopy := *req.URL
			urlCopy.Path = "/" + strings.TrimPrefix(req.URL.Path, "/legacy/")
			clone.URL = &urlCopy
			next.ServeHTTP(res, clone)
			return
		case req.Method == http.MethodGet && (req.URL.Path == "/next" || req.URL.Path == "/next/"):
			destination := "/"
			if req.URL.RawQuery != "" {
				destination += "?" + req.URL.RawQuery
			}
			http.Redirect(res, req, destination, http.StatusTemporaryRedirect)
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
