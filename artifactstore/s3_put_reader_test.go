package artifactstore

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Uploading a file must leave it open for the caller: net/http closes an
// io.Closer request body, which made every replay segment upload fail with
// "file already closed" when pokereplay closed its own file afterwards.
func TestPutObjectReaderLeavesCallerFileOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer srv.Close()
	store, err := NewS3(S3Config{
		Endpoint: srv.URL, Bucket: "pokepilot", Region: "us-east-1",
		AccessKey: "test", SecretKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "part-00000.mp4")
	if err := os.WriteFile(path, []byte("segment"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutObjectReader(context.Background(), "runs/run-1/part.mp4", "video/mp4", file); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("caller's file was closed by the upload: %v", err)
	}
}
