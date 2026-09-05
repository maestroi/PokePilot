package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
)

func TestReplayRenderCachesMP4AndServesRanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake stream helper is a POSIX shell script")
	}
	recordingBytes := []byte("recording")
	sum := sha256.Sum256(recordingBytes)
	recordingSHA := hex.EncodeToString(sum[:])
	cacheKey := "runs/run-1/attempt-1/replay-" + recordingSHA[:12] + ".mp4"
	var mu sync.Mutex
	var cached []byte

	s3srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pokepilot/runs/run-1/attempt-1/run.gbrun":
			_, _ = w.Write(recordingBytes)
		case r.Method == http.MethodHead && r.URL.Path == "/pokepilot/"+cacheKey:
			mu.Lock()
			size := len(cached)
			mu.Unlock()
			if size == 0 {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Length", "8")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/pokepilot/"+cacheKey:
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			cached = append([]byte(nil), body...)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/pokepilot/"+cacheKey:
			mu.Lock()
			body := append([]byte(nil), cached...)
			mu.Unlock()
			if len(body) == 0 {
				http.NotFound(w, r)
				return
			}
			if r.Header.Get("Range") == "bytes=0-3" {
				w.Header().Set("Content-Range", "bytes 0-3/8")
				w.Header().Set("Accept-Ranges", "bytes")
				w.Header().Set("Content-Type", "video/mp4")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(body[:4])
				return
			}
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write(body)
		default:
			t.Fatalf("unexpected S3 request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer s3srv.Close()

	store, err := artifactstore.NewS3(artifactstore.S3Config{
		Endpoint: s3srv.URL, Bucket: "pokepilot", Region: "us-east-1",
		AccessKey: "test", SecretKey: "secret", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/runs/run-1/artifacts" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(artifactList{
			RunID: "run-1", Attempt: 1,
			Artifacts: []artifactRef{{
				Name: "run.gbrun", MediaType: "application/octet-stream", SHA256: recordingSHA,
				Store: "s3", Bucket: "pokepilot", ObjectKey: "runs/run-1/attempt-1/run.gbrun",
				Size: int64(len(recordingBytes)), Replayable: true,
			}},
		})
	}))
	defer wall.Close()

	dir := t.TempDir()
	rom := filepath.Join(dir, "pokemon-red.gb")
	if err := os.WriteFile(rom, []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	stream := filepath.Join(dir, "fake-gomeboy-stream")
	script := `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-output" ]; then
    shift
    out="$1"
  fi
  shift
done
printf 'fake-mp4' > "$out"
`
	if err := os.WriteFile(stream, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	replay := newReplayServer(wall.URL, rom, stream, store)
	srv := httptest.NewServer(replay.handler())
	defer srv.Close()

	res, err := http.Post(srv.URL+"/v1/runs/run-1/replay/render", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("render status=%d, want 202", res.StatusCode)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		res, err = http.Get(srv.URL + "/v1/runs/run-1/replay/status")
		if err != nil {
			t.Fatal(err)
		}
		var status replayStatus
		err = json.NewDecoder(res.Body).Decode(&status)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if status.State == "ready" {
			if status.ObjectKey != cacheKey {
				t.Fatalf("cache key=%q, want %q", status.ObjectKey, cacheKey)
			}
			break
		}
		if status.State == "error" {
			t.Fatalf("render failed: %s", status.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("render did not become ready: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	gotCache := string(cached)
	mu.Unlock()
	if gotCache != "fake-mp4" {
		t.Fatalf("cached bytes=%q", gotCache)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/v1/runs/run-1/replay/video", nil)
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
	if res.StatusCode != http.StatusPartialContent || string(body) != "fake" || !strings.HasPrefix(res.Header.Get("Content-Range"), "bytes 0-3") {
		t.Fatalf("video status=%d range=%q body=%q", res.StatusCode, res.Header.Get("Content-Range"), body)
	}
}
