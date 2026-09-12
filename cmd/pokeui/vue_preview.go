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
// pokeui compilation. The committed placeholder keeps Go-only development and
// tests compilable before the frontend has been built.
//
//go:embed ui/vue
var vueWebAssets embed.FS

const vuePreviewPrefix = "/next/"

// withVuePreview keeps the legacy handler authoritative at every existing URL
// while reserving /next/ for the incremental Vue migration. Target selects the
// private operator or public spectator build subtree; the two trees are never
// exposed through the same HTTP surface.
func withVuePreview(next http.Handler, target string) http.Handler {
	preview := http.NewServeMux()
	mountVuePreview(preview, target)
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path == strings.TrimSuffix(vuePreviewPrefix, "/") {
			http.Redirect(res, req, vuePreviewPrefix, http.StatusTemporaryRedirect)
			return
		}
		if strings.HasPrefix(req.URL.Path, vuePreviewPrefix) {
			preview.ServeHTTP(res, req)
			return
		}
		next.ServeHTTP(res, req)
	})
}

func mountVuePreview(mux *http.ServeMux, target string) {
	entry := target + ".html"
	mux.HandleFunc("GET "+vuePreviewPrefix+"{$}", func(res http.ResponseWriter, req *http.Request) {
		serveVuePreviewFile(res, req, target, entry)
	})
	mux.HandleFunc("GET "+vuePreviewPrefix, func(res http.ResponseWriter, req *http.Request) {
		name := strings.TrimPrefix(req.URL.Path, vuePreviewPrefix)
		if name == "" {
			serveVuePreviewFile(res, req, target, entry)
			return
		}
		serveVuePreviewFile(res, req, target, name)
	})
}

func serveVuePreviewFile(res http.ResponseWriter, req *http.Request, target, name string) {
	clean := strings.TrimPrefix(path.Clean("/"+name), "/")
	if clean == "" || clean == "." || strings.HasPrefix(clean, "../") {
		http.NotFound(res, req)
		return
	}

	assetPath := path.Join("ui/vue", target, clean)
	data, err := fs.ReadFile(vueWebAssets, assetPath)
	if err != nil {
		if clean == target+".html" && errors.Is(err, fs.ErrNotExist) {
			res.Header().Set("Cache-Control", "no-store")
			http.Error(res, "Vue preview assets are not built; run `cd web && npm run build`", http.StatusServiceUnavailable)
			return
		}
		http.NotFound(res, req)
		return
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
}
