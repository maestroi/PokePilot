package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestRunPortableBundleMaterializesWithoutWall(t *testing.T) {
	bundle := testPortableBundle(t, false)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(bundle)
	}))
	defer server.Close()

	out := filepath.Join(t.TempDir(), "repro")
	if err := runPortableBundle(context.Background(), server.URL+"/repro.zip", out, false); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"round-066-frame-0000000042-fail.state":             "state-bytes",
		"round-066-frame-0000000042-fail.knowledge-v4.json": "knowledge-bytes",
	} {
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("%s=%q want=%q", name, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "repro.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRunPortableBundleRejectsHashMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.zip")
	if err := os.WriteFile(path, testPortableBundle(t, true), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runPortableBundle(context.Background(), path, filepath.Join(t.TempDir(), "out"), false)
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("err=%v", err)
	}
}

func testPortableBundle(t *testing.T, badHash bool) []byte {
	t.Helper()
	stateName := "round-066-frame-0000000042-fail.state"
	knowledgeName := "round-066-frame-0000000042-fail.knowledge-v4.json"
	state := []byte("state-bytes")
	knowledge := []byte("knowledge-bytes")
	stateSum := sha256.Sum256(state)
	knowledgeSum := sha256.Sum256(knowledge)
	stateSHA := hex.EncodeToString(stateSum[:])
	if badHash {
		stateSHA = strings.Repeat("0", 64)
	}
	manifest := farm.PortableReproManifest{
		Version:     farm.PortableReproVersion,
		IssueNumber: 41,
		RunID:       "run-42",
		Attempt:     3,
		Checkpoint: farm.PortableReproFile{
			Name: stateName, SHA256: stateSHA, Size: int64(len(state)),
		},
		Knowledge: farm.PortableReproFile{
			Name: knowledgeName, SHA256: hex.EncodeToString(knowledgeSum[:]), Size: int64(len(knowledge)),
		},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range map[string][]byte{
		"repro.json":  manifestData,
		stateName:     state,
		knowledgeName: knowledge,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
