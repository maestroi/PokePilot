package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/artifactstore"
)

func TestUploadSkipsExistingAndUnrecognised(t *testing.T) {
	rom, err := os.ReadFile("../../roms/tetris.gb")
	if err != nil {
		t.Skipf("tetris ROM: %v", err)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tetris.gb"), rom, 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a rom"), 0o644)

	objects := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/bucket/")
		switch r.Method {
		case http.MethodHead:
			if _, ok := objects[key]; !ok {
				http.NotFound(w, r)
			}
		case http.MethodPut:
			objects[key], _ = io.ReadAll(r.Body)
		}
	}))
	defer srv.Close()
	store, err := artifactstore.NewS3(artifactstore.S3Config{
		Endpoint: srv.URL, Bucket: "bucket", Region: "us-east-1", AccessKey: "a", SecretKey: "s",
	})
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	for range 2 {
		if err := upload(context.Background(), store, dir, &out); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(objects["roms/tetris"], rom) || len(objects) != 1 {
		t.Fatalf("objects = %v keys, want only roms/tetris with ROM bytes", len(objects))
	}
	for _, want := range []string{"upload  tetris.gb -> roms/tetris", "exists  tetris.gb -> roms/tetris", "skip    notes.txt"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}
