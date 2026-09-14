package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestBuildPortableReproContainsExactCheckpointPairOnly(t *testing.T) {
	manifest := sampleManifest("run-42-attempt-3-objective-key")
	artifacts := []artifactMeta{
		{Name: "round-066-frame-000002-fail.state", Size: 5, SHA256: strings.Repeat("a", 64), Data: []byte("state")},
		{Name: "round-066-frame-000002-fail.knowledge-v4.json", Size: 9, SHA256: strings.Repeat("b", 64), Data: []byte("knowledge")},
		{Name: "round-066-frame-000002-fail.failure-repro.json", Size: 2, SHA256: strings.Repeat("c", 64), Data: []byte("{}")},
		{Name: "final.png", Size: 12, SHA256: strings.Repeat("d", 64), Data: []byte("must-not-leak")},
	}
	blob, ok, err := buildPortableRepro(41, manifest, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected portable repro")
	}
	zr, err := zip.NewReader(bytes.NewReader(blob.data), int64(len(blob.data)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	var repro farm.PortableReproManifest
	for _, f := range zr.File {
		names = append(names, f.Name)
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if f.Name == "repro.json" {
			if err := json.Unmarshal(data, &repro); err != nil {
				t.Fatal(err)
			}
		}
		if bytes.Contains(data, []byte("must-not-leak")) {
			t.Fatalf("unsafe artifact leaked through %s", f.Name)
		}
	}
	sort.Strings(names)
	want := []string{
		"repro.json",
		"round-066-frame-000002-fail.failure-repro.json",
		"round-066-frame-000002-fail.knowledge-v4.json",
		"round-066-frame-000002-fail.state",
	}
	sort.Strings(want)
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("zip files=%v want=%v", names, want)
	}
	if repro.IssueNumber != 41 || repro.RunID != "run-42" || repro.Attempt != 3 {
		t.Fatalf("manifest=%+v", repro)
	}
	if repro.Checkpoint.Name != "round-066-frame-000002-fail.state" || repro.Knowledge.Name != "round-066-frame-000002-fail.knowledge-v4.json" {
		t.Fatalf("checkpoint pair=%+v / %+v", repro.Checkpoint, repro.Knowledge)
	}
	if err := farm.ValidatePortableReproManifest(repro); err != nil {
		t.Fatal(err)
	}
}

func TestPublishPortableReproCreatesReleaseAndAssetIdempotently(t *testing.T) {
	var mu sync.Mutex
	var releaseCreated int
	var uploaded int
	var uploadedName string
	var uploadedBytes []byte
	var assets []githubReleaseAsset

	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/o/r/releases/tags/"+reproReleaseTag, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if releaseCreated == 0 {
			testHTTPError(w, http.StatusNotFound, "not found")
			return
		}
		_ = json.NewEncoder(w).Encode(githubRelease{
			ID: 7, TagName: reproReleaseTag,
			UploadURL: "http://" + r.Host + "/upload{?name,label}",
		})
	})
	mux.HandleFunc("POST /repos/o/r/releases", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("auth=%q", got)
		}
		mu.Lock()
		releaseCreated++
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(githubRelease{
			ID: 7, TagName: reproReleaseTag,
			UploadURL: "http://" + r.Host + "/upload{?name,label}",
		})
	})
	mux.HandleFunc("GET /repos/o/r/releases/7/assets", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(assets)
	})
	mux.HandleFunc("POST /upload", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		defer mu.Unlock()
		uploaded++
		uploadedName = r.URL.Query().Get("name")
		uploadedBytes = append([]byte(nil), data...)
		asset := githubReleaseAsset{
			ID: 9, Name: uploadedName, Size: int64(len(data)),
			BrowserDownloadURL: "https://github.test/o/r/releases/download/" + reproReleaseTag + "/" + url.PathEscape(uploadedName),
		}
		assets = append(assets, asset)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(asset)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := newGitHubClient(server.URL, "https://github.test", "o/r", "secret", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	blob := portableReproBlob{
		asset: portableReproAsset{Name: "repro-issue-41-abc.zip", SHA256: strings.Repeat("e", 64), Size: 3},
		data:  []byte("zip"),
	}
	for i := 0; i < 2; i++ {
		asset, err := client.publishPortableRepro(context.Background(), blob)
		if err != nil {
			t.Fatal(err)
		}
		if asset.URL == "" || asset.Name != blob.asset.Name {
			t.Fatalf("asset=%+v", asset)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if releaseCreated != 1 || uploaded != 1 {
		t.Fatalf("releaseCreated=%d uploaded=%d", releaseCreated, uploaded)
	}
	if uploadedName != blob.asset.Name || string(uploadedBytes) != "zip" {
		t.Fatalf("upload name=%q bytes=%q", uploadedName, uploadedBytes)
	}
}

func TestRenderPortableReproUsesOfflineBundleCommand(t *testing.T) {
	asset := portableReproAsset{
		Name:   "repro-issue-41-deadbeef.zip",
		URL:    "https://github.test/o/r/releases/download/farm-repros/repro-issue-41-deadbeef.zip",
		SHA256: strings.Repeat("f", 64), Size: 123,
	}
	body := renderPortableReproAsset(asset)
	for _, want := range []string{
		"Portable repro bundle",
		asset.URL,
		asset.SHA256,
		"no ROM",
		"go run ./cmd/pokerepro -bundle '" + asset.URL + "' -play",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}
