package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
)

// memS3 is a minimal in-memory S3: GET (with "bytes=N-" ranges), HEAD, PUT.
type memS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newMemS3(t *testing.T) (*memS3, *artifactstore.S3) {
	t.Helper()
	m := &memS3{objects: map[string][]byte{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/pokepilot/")
		m.mu.Lock()
		defer m.mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			m.objects[key] = body
		case http.MethodHead, http.MethodGet:
			body, ok := m.objects[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			if start, ok := strings.CutPrefix(r.Header.Get("Range"), "bytes="); ok {
				from, _ := strconv.Atoi(strings.TrimSuffix(start, "-"))
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(body)-1, len(body)))
				w.Header().Set("Content-Length", strconv.Itoa(len(body)-from))
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(body[from:])
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			if r.Method == http.MethodGet {
				_, _ = w.Write(body)
			}
		}
	}))
	t.Cleanup(srv.Close)
	store, err := artifactstore.NewS3(artifactstore.S3Config{
		Endpoint: srv.URL, Bucket: "pokepilot", Region: "us-east-1",
		AccessKey: "test", SecretKey: "secret", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m, store
}

func (m *memS3) put(key string, data []byte) {
	m.mu.Lock()
	m.objects[key] = data
	m.mu.Unlock()
}

// fakeYouTube records sessions/uploads. failFirstUpload accepts 4 bytes of
// the first upload and then fails, so the publisher must resume at offset 4.
type fakeYouTube struct {
	mu              sync.Mutex
	sessions        int
	uploads         int
	received        []byte
	contentRanges   []string
	failFirstUpload bool
	completed       bool
	sawSecret       bool
}

func (f *fakeYouTube) server(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/token":
			_ = r.ParseForm()
			if r.Form.Get("refresh_token") != "refresh-secret" || r.Form.Get("client_secret") != "client-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"access-secret"}`))
		case r.URL.Path == "/upload" && r.Method == http.MethodPost:
			f.sessions++
			w.Header().Set("Location", srv.URL+"/session/"+strconv.Itoa(f.sessions)+"?upload_id=session-secret")
		case strings.HasPrefix(r.URL.Path, "/session/") && r.Method == http.MethodPut:
			if r.Header.Get("Authorization") != "Bearer access-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			cr := r.Header.Get("Content-Range")
			if strings.HasPrefix(cr, "bytes */") { // status query
				if f.completed {
					_, _ = w.Write([]byte(`{"id":"vid-123"}`))
					return
				}
				if len(f.received) > 0 {
					w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", len(f.received)-1))
				}
				w.WriteHeader(308)
				return
			}
			f.uploads++
			f.contentRanges = append(f.contentRanges, cr)
			body, _ := io.ReadAll(r.Body)
			if f.failFirstUpload && f.uploads == 1 {
				f.received = append(f.received, body[:4]...)
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			f.received = append(f.received, body...)
			f.completed = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"vid-123"}`))
		default:
			t.Errorf("unexpected youtube request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func youtubeTestServer(t *testing.T, wallURL string, store *artifactstore.S3, yt *httptest.Server) *replayServer {
	t.Helper()
	s := newReplayServer(wallURL, "", "", store)
	s.youtube = youtubeConfig{
		ClientID: "client", ClientSecret: "client-secret", RefreshToken: "refresh-secret",
		RunURLTemplate: "https://rompilot.app/runs/{run_id}",
		TokenURL:       yt.URL + "/token", UploadURL: yt.URL + "/upload",
	}
	return s
}

func TestYouTubePublishUploadsReadyReplayOnce(t *testing.T) {
	mem, store := newMemS3(t)
	fake := &fakeYouTube{}
	yt := fake.server(t)
	recording := artifactRef{
		Name: "run.gbrun", SHA256: strings.Repeat("a", 64), Store: "s3",
		ObjectKey: "runs/run-1/attempt-1/run.gbrun", Replayable: true,
	}
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/runs/run-1/artifacts":
			_ = json.NewEncoder(w).Encode(artifactList{RunID: "run-1", Attempt: 1, Artifacts: []artifactRef{recording}})
		case "/v1/runs/run-1":
			_, _ = w.Write([]byte(`{"run":{"run_id":"run-1","status":"done","game":"red","starter":"Squirtle","trace":"SECRET-TRACE"},"finish":{"reason":"done","progress_final":{"badges":2,"maps":30}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer wall.Close()

	s := youtubeTestServer(t, wall.URL, store, yt)
	mp4 := []byte("fake-mp4-bytes")
	mem.put(s.replayCacheKeyForMode("run-1", []replayRecording{{Attempt: 1, Artifact: recording}}, replayModeBroadcast), mp4)
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	post := func() (int, string) {
		res, err := http.Post(srv.URL+"/v1/runs/run-1/publish/youtube", "application/json", strings.NewReader(`{"privacy":"private"}`))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(body)
	}
	if code, body := post(); code != http.StatusAccepted {
		t.Fatalf("first publish = %d %s", code, body)
	}
	deadline := time.Now().Add(5 * time.Second)
	var status string
	for time.Now().Before(deadline) {
		res, err := http.Get(srv.URL + "/v1/runs/run-1/publish/youtube")
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		status = string(data)
		if strings.Contains(status, `"state":"published"`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(status, `"video_id":"vid-123"`) {
		t.Fatalf("publication never completed: %s", status)
	}
	code, body := post()
	if code != http.StatusOK || !strings.Contains(body, "vid-123") {
		t.Fatalf("retry after publish = %d %s, want 200 with existing video", code, body)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.sessions != 1 || fake.uploads != 1 || !bytes.Equal(fake.received, mp4) {
		t.Fatalf("sessions=%d uploads=%d received=%q, want one upload of the cached MP4", fake.sessions, fake.uploads, fake.received)
	}
	for _, out := range []string{status, body, string(mem.objects[youtubePublicationKey("run-1")])} {
		for _, secret := range []string{"refresh-secret", "client-secret", "access-secret"} {
			if strings.Contains(out, secret) {
				t.Fatalf("credential %q leaked: %s", secret, out)
			}
		}
	}
	for _, out := range []string{status, body} {
		if strings.Contains(out, "session-secret") {
			t.Fatalf("upload session URI leaked to API: %s", out)
		}
	}
}

func TestYouTubePublishResumesInterruptedUpload(t *testing.T) {
	mem, store := newMemS3(t)
	youtubeRetryDelay = time.Millisecond
	t.Cleanup(func() { youtubeRetryDelay = 2 * time.Second })
	fake := &fakeYouTube{failFirstUpload: true}
	yt := fake.server(t)
	s := youtubeTestServer(t, "http://wall.invalid", store, yt)
	mp4 := []byte("0123456789")
	mem.put("replays/run-2.mp4", mp4)

	rec := s.publishYouTube(context.Background(), youtubePublication{
		RunID: "run-2", State: youtubeStateUploading, Privacy: "unlisted",
		SourceKey: "replays/run-2.mp4", SourceSize: int64(len(mp4)),
	}, youtubeVideoMeta{Title: "t"})

	if rec.State != youtubeStatePublished || rec.VideoID != "vid-123" {
		t.Fatalf("publication = %+v, want published", rec)
	}
	if fake.sessions != 1 || !bytes.Equal(fake.received, mp4) {
		t.Fatalf("sessions=%d received=%q, want one session holding the whole MP4", fake.sessions, fake.received)
	}
	if got := fake.contentRanges; len(got) != 2 || got[1] != "bytes 4-9/10" {
		t.Fatalf("content ranges = %q, want resume from byte 4", got)
	}
}

func TestYouTubePublishFinishesUnrecordedUploadWithoutReuploading(t *testing.T) {
	mem, store := newMemS3(t)
	fake := &fakeYouTube{completed: true}
	yt := fake.server(t)
	s := youtubeTestServer(t, "http://wall.invalid", store, yt)
	mem.put("replays/run-3.mp4", []byte("mp4"))

	// The process died after YouTube accepted the bytes but before the
	// result was recorded: the stored session must resolve to that video.
	rec := s.publishYouTube(context.Background(), youtubePublication{
		RunID: "run-3", State: youtubeStateUploading, SourceKey: "replays/run-3.mp4", SourceSize: 3,
		SessionURI: yt.URL + "/session/1?upload_id=session-secret",
	}, youtubeVideoMeta{Title: "t"})

	if rec.State != youtubeStatePublished || rec.VideoID != "vid-123" {
		t.Fatalf("publication = %+v, want published", rec)
	}
	if fake.sessions != 0 || fake.uploads != 0 {
		t.Fatalf("sessions=%d uploads=%d, want neither", fake.sessions, fake.uploads)
	}
}

func TestYouTubePublishFailureRecordsRedactedError(t *testing.T) {
	mem, store := newMemS3(t)
	fake := &fakeYouTube{}
	yt := fake.server(t)
	s := youtubeTestServer(t, "http://wall.invalid", store, yt)
	s.youtube.RefreshToken = "wrong-refresh-secret"
	mem.put("replays/run-4.mp4", []byte("mp4"))

	rec := s.publishYouTube(context.Background(), youtubePublication{
		RunID: "run-4", State: youtubeStateUploading, SourceKey: "replays/run-4.mp4", SourceSize: 3,
	}, youtubeVideoMeta{Title: "t"})

	if rec.State != youtubeStateFailed || rec.Error == "" {
		t.Fatalf("publication = %+v, want failed with error", rec)
	}
	stored := string(mem.objects[youtubePublicationKey("run-4")])
	if !strings.Contains(stored, `"state":"failed"`) || strings.Contains(stored, "secret") {
		t.Fatalf("stored record = %s, want failed without credentials", stored)
	}
}

func TestBuildYouTubeMetaIsDeterministicAndOmitsTraces(t *testing.T) {
	var run wallRunForPublish
	if err := json.Unmarshal([]byte(`{"run":{"run_id":"r 1","status":"done","game":"red","starter":"Bulbasaur",
		"planner":"llm","llm_profile":"fast","queued_at":100,"ended_at":3700,"attempts":2,
		"trace":"SECRET-TRACE","raw":"SECRET-RAW","decision":"SECRET-DECISION",
		"player":{"money":1,"party":[{"name":"IVYSAUR","level":18}]}},
		"finish":{"reason":"done","runner_version":"abc123","progress_final":{"badges":1,"maps":12}}}`), &run); err != nil {
		t.Fatal(err)
	}
	a := buildYouTubeMeta(run, "https://rompilot.app/runs/{run_id}")
	b := buildYouTubeMeta(run, "https://rompilot.app/runs/{run_id}")
	if a != b {
		t.Fatalf("metadata is not deterministic:\n%+v\n%+v", a, b)
	}
	if a.Title != "PokePilot AI plays red · Bulbasaur · 1 badge · done" {
		t.Fatalf("title = %q", a.Title)
	}
	for _, want := range []string{"Run: r 1", "Final party: IVYSAUR Lv18", "Badges: 1", "Maps visited: 12",
		"Duration: 1h0m0s", "Attempts: 2", "PokePilot build: abc123", "Run page: https://rompilot.app/runs/r%201"} {
		if !strings.Contains(a.Description, want) {
			t.Fatalf("description missing %q:\n%s", want, a.Description)
		}
	}
	if strings.Contains(a.Description, "SECRET") {
		t.Fatalf("description leaked model trace:\n%s", a.Description)
	}
	if noLink := buildYouTubeMeta(run, ""); strings.Contains(noLink.Description, "Run page") {
		t.Fatalf("backlink present without a configured template")
	}

	long := run
	long.Run.Starter = strings.Repeat("<x>", 80)
	if got := buildYouTubeMeta(long, ""); len([]rune(got.Title)) > youtubeTitleMaxRunes || strings.ContainsAny(got.Title, "<>") {
		t.Fatalf("title not clamped/sanitized: %q", got.Title)
	}
}
