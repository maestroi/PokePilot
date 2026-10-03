package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/game"
)

func TestFetchStoredROMUsesGameObjectAndRejectsUndetectableBytes(t *testing.T) {
	var gotPath string
	s3srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		_, _ = w.Write([]byte("not-a-cartridge"))
	}))
	defer s3srv.Close()

	store, err := artifactstore.NewS3(artifactstore.S3Config{
		Endpoint: s3srv.URL, Bucket: "pokepilot", Region: "us-east-1",
		AccessKey: "test", SecretKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	lib := &replayROMLibrary{
		paths:    map[game.GameID]string{},
		cacheDir: t.TempDir(),
	}
	_, err = lib.fetchStored(context.Background(), store, map[string]string{"game": "pokemon-blue"})
	if err == nil || !strings.Contains(err.Error(), "pokemon-blue") {
		t.Fatalf("err = %v, want the requested game", err)
	}
	if !strings.Contains(gotPath, "roms/pokemon-blue") {
		t.Fatalf("store path = %q, want roms/pokemon-blue", gotPath)
	}
	if _, ok := lib.paths[game.GameID("pokemon-blue")]; ok {
		t.Fatal("undetectable store object was remembered as pokemon-blue")
	}
}
