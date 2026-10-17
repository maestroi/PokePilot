package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/maestroi/pokepilot/artifactstore"
)

type fakeBroadcastMusicS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func testBroadcastMusicStore(t *testing.T) (*artifactstore.S3, *fakeBroadcastMusicS3) {
	t.Helper()
	fake := &fakeBroadcastMusicS3{objects: make(map[string][]byte)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/music-test/")
		fake.mu.Lock()
		defer fake.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			body, ok := fake.objects[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case http.MethodPut:
			data, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fake.objects[key] = data
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)
	store, err := artifactstore.NewS3(artifactstore.S3Config{
		Endpoint: srv.URL, Bucket: "music-test", AccessKey: "test-key",
		SecretKey: "test-secret", Region: "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, fake
}

func uploadTestMusic(t *testing.T, server *replayServer, name string, licensed bool, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	path := "/v1/live/broadcast/music?name=" + name
	if licensed {
		path += "&licensed=true"
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	rec := httptest.NewRecorder()
	server.handleBroadcastMusicUpload(rec, req)
	return rec
}

func TestBroadcastMusicUploadPersistsCatalogAndStagesVerifiedBytes(t *testing.T) {
	store, fake := testBroadcastMusicStore(t)
	server := &replayServer{store: store}
	data := []byte("ID3\x04\x00\x00licensed demo audio")
	rec := uploadTestMusic(t, server, "licensed.mp3", true, data)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var track broadcastMusicTrack
	if err := json.Unmarshal(rec.Body.Bytes(), &track); err != nil {
		t.Fatal(err)
	}
	if track.Format != "mp3" || track.Name != "licensed.mp3" || track.Size != int64(len(data)) {
		t.Fatalf("track=%+v", track)
	}
	lib, err := server.loadMusicLibrary(context.Background())
	if err != nil || len(lib.Tracks) != 1 || lib.Tracks[0].ID != track.ID {
		t.Fatalf("library=%+v err=%v", lib, err)
	}
	// An upload is idempotent and does not clutter the persistent library.
	duplicate := uploadTestMusic(t, server, "licensed.mp3", true, data)
	if duplicate.Code != http.StatusOK {
		t.Fatalf("duplicate HTTP %d: %s", duplicate.Code, duplicate.Body.String())
	}
	lib, err = server.loadMusicLibrary(context.Background())
	if err != nil || len(lib.Tracks) != 1 {
		t.Fatalf("duplicate catalog=%+v err=%v", lib, err)
	}

	staged, cleanup, err := server.stageBroadcastMusic(context.Background(), track.ID)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(staged)
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatalf("staged data=%q err=%v", actual, err)
	}
	cleanup()
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("staged music not removed: %v", err)
	}
	if _, _, err := server.stageBroadcastMusic(context.Background(), strings.Repeat("0", 64)); err == nil {
		t.Fatal("unknown track was accepted")
	}
	// Tampering/partial S3 reads must fail rather than reach FFmpeg.
	key, _ := broadcastMusicKey(track)
	fake.mu.Lock()
	fake.objects[key] = []byte("tampered")
	fake.mu.Unlock()
	if _, _, err := server.stageBroadcastMusic(context.Background(), track.ID); err == nil {
		t.Fatal("corrupt track was accepted")
	}
}

func TestBroadcastMusicRejectsUnauthorizedOrMalformedUploads(t *testing.T) {
	store, _ := testBroadcastMusicStore(t)
	server := &replayServer{store: store}
	valid := []byte("OggS\x00\x00licensed")
	cases := []struct {
		name     string
		filename string
		licensed bool
		data     []byte
		want     int
	}{
		{"unlicensed", "music.ogg", false, valid, http.StatusBadRequest},
		{"wrong extension", "music.exe", true, valid, http.StatusBadRequest},
		{"fake file", "music.ogg", true, []byte("not ogg"), http.StatusBadRequest},
		{"path traversal", "..%2Fmusic.ogg", true, valid, http.StatusBadRequest},
		{"empty", "music.ogg", true, nil, http.StatusRequestEntityTooLarge},
		{"too large", "music.ogg", true, bytes.Repeat([]byte{'a'}, broadcastMusicMaxBytes+1), http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := uploadTestMusic(t, server, tc.filename, tc.licensed, tc.data)
			if rec.Code != tc.want {
				t.Fatalf("HTTP %d wanted %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
	noS3 := &replayServer{}
	if rec := uploadTestMusic(t, noS3, "music.ogg", true, valid); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing S3 HTTP %d", rec.Code)
	}
}

func TestBroadcastMusicListHidesS3Metadata(t *testing.T) {
	store, _ := testBroadcastMusicStore(t)
	server := &replayServer{store: store}
	rec := uploadTestMusic(t, server, "allowed.flac", true, []byte("fLaC\x00\x00hello"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload HTTP %d: %s", rec.Code, rec.Body.String())
	}
	response := httptest.NewRecorder()
	server.handleBroadcastMusicList(response, httptest.NewRequest(http.MethodGet, "/v1/live/broadcast/music", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list HTTP %d", response.Code)
	}
	for _, private := range []string{"test-secret", "/music-test/", "media/broadcast/music/tracks/"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatalf("library leaked internal storage detail %q", private)
		}
	}
}
