package artifactstore

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetObjectPreservesRangeResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/pokepilot/runs/run-1/replay.mp4" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Range"); got != "bytes=2-5" {
			t.Fatalf("Range = %q", got)
		}
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "AWS4-HMAC-SHA256 ") {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 2-5/8")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("2345"))
	}))
	defer srv.Close()

	store, err := NewS3(S3Config{
		Endpoint: srv.URL, Bucket: "pokepilot", Region: "us-east-1",
		AccessKey: "test", SecretKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	obj, err := store.GetObject(context.Background(), "runs/run-1/replay.mp4", "bytes=2-5")
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Body.Close()
	body, err := io.ReadAll(obj.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "2345" {
		t.Fatalf("body = %q", body)
	}
	if obj.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d", obj.StatusCode)
	}
	if obj.ContentRange != "bytes 2-5/8" {
		t.Fatalf("Content-Range = %q", obj.ContentRange)
	}
}

func TestHeadObjectNotFoundIsClassified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("method = %s, want HEAD", r.Method)
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	store, err := NewS3(S3Config{
		Endpoint: srv.URL, Bucket: "pokepilot", Region: "us-east-1",
		AccessKey: "test", SecretKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.HeadObject(context.Background(), "missing")
	if !IsNotFound(err) {
		t.Fatalf("HeadObject error = %v, want classified not found", err)
	}
}
