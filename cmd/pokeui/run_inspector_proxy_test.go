package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunInspectorRoutesUseWallForMetadataAndReplayForMedia(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/runs/run-1/debug":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"run":{"run_id":"run-1"},"summary":{}}`)
		case "/v1/runs/run-1/artifacts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"run_id":"run-1","artifacts":[]}`)
		case "/v1/media/render-jobs":
			if got := r.URL.Query().Get("limit"); got != "5" {
				t.Fatalf("render job limit=%q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jobs":[{"id":"render-1","run_id":"run-1","state":"rendering","segments_total":4,"segments_done":2}],"total":1,"states":{"rendering":1}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer wall.Close()

	replay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/runs/run-1/replay/video":
			if got := r.Header.Get("Range"); got != "bytes=0-3" {
				t.Fatalf("replay Range=%q", got)
			}
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Range", "bytes 0-3/8")
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "fake")
		case "/v1/runs/run-1/live/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"run_id":"run-1","state":"live","target_fps":20}`)
		case "/v1/runs/run-1/live/stream.mjpeg":
			w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=test")
			w.Header().Set("X-PokePilot-Live-FPS", "20")
			_, _ = io.WriteString(w, "--test--\r\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer replay.Close()

	ui := httptest.NewServer(handlerWithServices(wall.URL, replay.URL, ""))
	defer ui.Close()

	res, err := http.Get(ui.URL + "/v1/runs/run-1/debug")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("debug status=%d", res.StatusCode)
	}

	res, err = http.Get(ui.URL + "/v1/media/render-jobs?limit=5")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), `"segments_done":2`) {
		t.Fatalf("render jobs status=%d body=%q", res.StatusCode, body)
	}

	req, err := http.NewRequest(http.MethodGet, ui.URL+"/v1/runs/run-1/replay/video", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-3")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusPartialContent || string(body) != "fake" || res.Header.Get("Content-Range") != "bytes 0-3/8" {
		t.Fatalf("video status=%d range=%q body=%q", res.StatusCode, res.Header.Get("Content-Range"), body)
	}

	res, err = http.Get(ui.URL + "/v1/runs/run-1/live/status")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), `"state":"live"`) {
		t.Fatalf("live status=%d body=%q", res.StatusCode, body)
	}

	res, err = http.Get(ui.URL + "/v1/runs/run-1/live/stream.mjpeg")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("X-PokePilot-Live-FPS") != "20" || string(body) != "--test--\r\n" {
		t.Fatalf("live stream status=%d fps=%q body=%q", res.StatusCode, res.Header.Get("X-PokePilot-Live-FPS"), body)
	}
}

func TestRunInspectorReplayIsExplicitlyDisabledWithoutService(t *testing.T) {
	wall := httptest.NewServer(http.NotFoundHandler())
	defer wall.Close()
	ui := httptest.NewServer(handlerWithServices(wall.URL, "", ""))
	defer ui.Close()

	res, err := http.Get(ui.URL + "/v1/runs/run-1/replay/status")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", res.StatusCode)
	}
}
