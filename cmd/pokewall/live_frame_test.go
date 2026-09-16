package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchRunnerBufferedFrameOptsIntoPlaybackQueue(t *testing.T) {
	var buffered string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buffered = r.URL.Query().Get("buffered")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("frame"))
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")
	data, err := fetchRunnerBufferedFrame([]string{addr})
	if err != nil {
		t.Fatal(err)
	}
	if buffered != "1" {
		t.Fatalf("buffered query = %q, want 1", buffered)
	}
	if string(data) != "frame" {
		t.Fatalf("frame = %q, want frame", data)
	}
}
