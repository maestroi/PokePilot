package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorLoadsThumbnailFramePolicy(t *testing.T) {
	page := string(operatorIndexPage())
	if !strings.Contains(page, `<script src="/frame_policy.js"></script>`) {
		t.Fatal("operator page does not load frame_policy.js")
	}

	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer wall.Close()
	srv := httptest.NewServer(handler(wall.URL))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/frame_policy.js")
	if err != nil {
		t.Fatalf("GET frame policy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("frame policy status=%d, want 200", resp.StatusCode)
	}
}

func TestThumbnailFramePolicyKeepsSelectedRunFast(t *testing.T) {
	src := string(framePolicyJS)
	for _, want := range []string{
		"const thumbnailMs = 500",
		"document.documentElement.dataset.selectedRun",
		"runID !== selected",
		`url.pathname === "/frame"`,
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("frame policy missing %q", want)
		}
	}
}
