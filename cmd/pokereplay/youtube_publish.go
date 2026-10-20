package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
)

// YouTube VOD publishing uploads the already-cached replay MP4; it never
// re-renders or replays gameplay. Publication state lives in its own S3
// record (publications/youtube/<run>.json), separate from run status, so an
// upload failure can never change a run's result or its replay artifact.

const (
	youtubeUploadTimeout   = 6 * time.Hour
	youtubeUploadAttempts  = 5
	youtubeDescMaxBytes    = 4500
	youtubeDefaultTokenURL = "https://oauth2.googleapis.com/token"
	// part=snippet,status is the minimum needed to set title + privacy.
	youtubeDefaultUploadURL = "https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status"

	youtubeStatePublished   = "published"
	youtubeStateUploading   = "uploading"
	youtubeStateFailed      = "failed"
	youtubeStateInterrupted = "interrupted"
	youtubeStateNone        = "none"
)

// youtubeRetryDelay is the backoff unit between upload attempts (tests shrink it).
var youtubeRetryDelay = 2 * time.Second

// youtubeConfig is server-side only. Credentials come from the replay host's
// environment and never appear in API responses, records, or logs.
type youtubeConfig struct {
	ClientID       string
	ClientSecret   string
	RefreshToken   string
	RunURLTemplate string // optional backlink; "{run_id}" is replaced
	TokenURL       string
	UploadURL      string
}

func youtubeConfigFromEnv(getenv func(string) string) youtubeConfig {
	return youtubeConfig{
		ClientID:       strings.TrimSpace(getenv("POKEPILOT_YOUTUBE_CLIENT_ID")),
		ClientSecret:   strings.TrimSpace(getenv("POKEPILOT_YOUTUBE_CLIENT_SECRET")),
		RefreshToken:   strings.TrimSpace(getenv("POKEPILOT_YOUTUBE_REFRESH_TOKEN")),
		RunURLTemplate: youtubeRunURLTemplate(getenv("POKEPILOT_YOUTUBE_RUN_URL_TEMPLATE")),
		TokenURL:       youtubeDefaultTokenURL,
		UploadURL:      youtubeDefaultUploadURL,
	}
}

// youtubeRunURLTemplate defaults the description backlink to the public run
// page so every video can be traced to its run; "-" disables it.
func youtubeRunURLTemplate(value string) string {
	switch value = strings.TrimSpace(value); value {
	case "":
		return "https://rompilot.app/runs/{run_id}"
	case "-":
		return ""
	}
	return value
}

func (c youtubeConfig) runURL(runID string) string {
	if c.RunURLTemplate == "" {
		return ""
	}
	return strings.ReplaceAll(c.RunURLTemplate, "{run_id}", url.PathEscape(runID))
}

func (c youtubeConfig) configured() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.RefreshToken != ""
}

// youtubePublication is the durable publication record. SessionURI is the
// resumable-upload session, which authorizes writes to that upload, so it is
// persisted for resume but stripped from every API response.
type youtubePublication struct {
	RunID       string `json:"run_id"`
	State       string `json:"state"`
	Privacy     string `json:"privacy,omitempty"`
	Title       string `json:"title,omitempty"`
	SourceKey   string `json:"source_key,omitempty"`
	SourceSize  int64  `json:"source_size,omitempty"`
	VideoID     string `json:"video_id,omitempty"`
	VideoURL    string `json:"video_url,omitempty"`
	Error       string `json:"error,omitempty"`
	UpdatedAt   int64  `json:"updated_at,omitempty"`
	PublishedAt int64  `json:"published_at,omitempty"`
	SessionURI  string `json:"session_uri,omitempty"`
	// UploadedBytes is live progress, filled only in status responses.
	UploadedBytes int64 `json:"uploaded_bytes,omitempty"`
}

func (p youtubePublication) view() youtubePublication {
	p.SessionURI = ""
	return p
}

type youtubeVideoMeta struct {
	Title       string
	Description string
}

func youtubePublicationKey(runID string) string {
	return "publications/youtube/" + sanitizeKeySegment(runID) + ".json"
}

func (s *replayServer) loadYouTubePublication(ctx context.Context, runID string) (youtubePublication, bool, error) {
	obj, err := s.store.GetObject(ctx, youtubePublicationKey(runID), "")
	if artifactstore.IsNotFound(err) {
		return youtubePublication{RunID: runID, State: youtubeStateNone}, false, nil
	}
	if err != nil {
		return youtubePublication{}, false, err
	}
	defer obj.Body.Close()
	var rec youtubePublication
	if err := json.NewDecoder(io.LimitReader(obj.Body, 1<<20)).Decode(&rec); err != nil {
		return youtubePublication{}, false, fmt.Errorf("decode youtube publication record: %w", err)
	}
	return rec, true, nil
}

func (s *replayServer) saveYouTubePublication(ctx context.Context, rec youtubePublication) error {
	rec.UpdatedAt = time.Now().Unix()
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = s.store.PutObject(ctx, youtubePublicationKey(rec.RunID), "application/json", data)
	return err
}

// claimYouTubeUpload is the in-process guard against two concurrent uploads
// of one run. ponytail: single pokereplay replica; a multi-replica deploy
// would need a durable lease (e.g. the wall media-job authority).
func (s *replayServer) claimYouTubeUpload(runID string) bool {
	s.youtubeMu.Lock()
	defer s.youtubeMu.Unlock()
	if s.youtubeActive[runID] != nil {
		return false
	}
	s.youtubeActive[runID] = new(atomic.Int64)
	return true
}

// youtubeUploadProgress is the in-flight upload's byte counter, nil when idle.
func (s *replayServer) youtubeUploadProgress(runID string) *atomic.Int64 {
	s.youtubeMu.Lock()
	defer s.youtubeMu.Unlock()
	return s.youtubeActive[runID]
}

// countingReader publishes how far the upload body has been read.
type countingReader struct {
	io.Reader
	n *atomic.Int64
}

func (r countingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.n.Add(int64(n))
	return n, err
}

func (s *replayServer) releaseYouTubeUpload(runID string) {
	s.youtubeMu.Lock()
	delete(s.youtubeActive, runID)
	s.youtubeMu.Unlock()
}

func (s *replayServer) youtubeUploadActive(runID string) bool {
	s.youtubeMu.Lock()
	defer s.youtubeMu.Unlock()
	return s.youtubeActive[runID] != nil
}

func (s *replayServer) handleYouTubeStatus(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "S3 artifact storage is not configured for the replay service"})
		return
	}
	rec, _, err := s.loadYouTubePublication(r.Context(), runID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": clipError(err)})
		return
	}
	if rec.State == youtubeStateUploading {
		if progress := s.youtubeUploadProgress(runID); progress != nil {
			rec.UploadedBytes = progress.Load()
		} else {
			rec.State = youtubeStateInterrupted
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"configured": s.youtube.configured(), "publication": rec.view()})
}

// publishYouTube runs one upload to completion and records the outcome. A
// recorded session is resumed first, so an interrupted upload continues and an
// upload that finished before its result was recorded is not repeated.
func (s *replayServer) publishYouTube(ctx context.Context, rec youtubePublication, meta youtubeVideoMeta) youtubePublication {
	videoID, err := s.uploadYouTube(ctx, &rec, meta)
	if err != nil {
		rec.State = youtubeStateFailed
		rec.Error = clipError(err)
		log.Printf("pokereplay: youtube publish %s failed: %s", rec.RunID, rec.Error)
	} else {
		rec.State = youtubeStatePublished
		rec.VideoID = videoID
		rec.VideoURL = "https://www.youtube.com/watch?v=" + url.QueryEscape(videoID)
		rec.PublishedAt = time.Now().Unix()
		rec.SessionURI = ""
		rec.Error = ""
		log.Printf("pokereplay: youtube publish %s complete: video %s", rec.RunID, videoID)
	}
	// Record the outcome even if the upload context expired.
	saveCtx, cancel := context.WithTimeout(context.Background(), wallTimeout)
	defer cancel()
	if err := s.saveYouTubePublication(saveCtx, rec); err != nil {
		log.Printf("pokereplay: record youtube publication %s: %v", rec.RunID, err)
	}
	return rec
}

func (s *replayServer) uploadYouTube(ctx context.Context, rec *youtubePublication, meta youtubeVideoMeta) (string, error) {
	var lastErr error
	for attempt := 0; attempt < youtubeUploadAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * youtubeRetryDelay):
			}
		}
		// Access tokens last an hour; fetch one per attempt rather than tracking expiry.
		token, err := s.youtubeAccessToken(ctx)
		if err != nil {
			return "", err
		}
		offset := int64(0)
		if rec.SessionURI != "" {
			var videoID string
			var gone bool
			offset, videoID, gone, err = s.youtubeQueryUpload(ctx, token, rec.SessionURI, rec.SourceSize)
			if err != nil {
				lastErr = err
				continue
			}
			if videoID != "" {
				return videoID, nil
			}
			if gone {
				rec.SessionURI = ""
				offset = 0
			}
		}
		if rec.SessionURI == "" {
			session, err := s.youtubeCreateSession(ctx, token, rec, meta)
			if err != nil {
				return "", err
			}
			rec.SessionURI = session
			if err := s.saveYouTubePublication(ctx, *rec); err != nil {
				return "", fmt.Errorf("record youtube upload session: %w", err)
			}
		}
		videoID, err := s.youtubeUploadFrom(ctx, token, rec, offset)
		if err == nil {
			return videoID, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("youtube upload did not complete after %d attempts: %w", youtubeUploadAttempts, lastErr)
}

func (s *replayServer) youtubeAccessToken(ctx context.Context) (string, error) {
	form := url.Values{
		"client_id":     {s.youtube.ClientID},
		"client_secret": {s.youtube.ClientSecret},
		"refresh_token": {s.youtube.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.youtube.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := s.youtubeHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("youtube token refresh: %w", stripURLError(err))
	}
	defer res.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&body)
	if res.StatusCode != http.StatusOK || body.AccessToken == "" {
		// Only the OAuth error code is surfaced; never the request form.
		return "", fmt.Errorf("youtube token refresh failed: %s %s", res.Status, body.Error)
	}
	return body.AccessToken, nil
}

func (s *replayServer) youtubeCreateSession(ctx context.Context, token string, rec *youtubePublication, meta youtubeVideoMeta) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"snippet": map[string]any{
			"title":       meta.Title,
			"description": meta.Description,
			"categoryId":  "20", // Gaming
		},
		"status": map[string]any{
			"privacyStatus":           rec.Privacy,
			"selfDeclaredMadeForKids": false,
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.youtube.UploadURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", "video/mp4")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(rec.SourceSize, 10))
	res, err := s.youtubeHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("youtube create upload session: %w", stripURLError(err))
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("youtube create upload session: %s", youtubeResponseError(res))
	}
	location := res.Header.Get("Location")
	if location == "" {
		return "", errors.New("youtube create upload session: response has no Location")
	}
	return location, nil
}

// youtubeQueryUpload asks how much of a resumable upload YouTube holds. It
// returns the next byte offset, the video id if the upload already finished,
// or gone when the session expired and a new one is needed.
func (s *replayServer) youtubeQueryUpload(ctx context.Context, token, session string, size int64) (offset int64, videoID string, gone bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, session, http.NoBody)
	if err != nil {
		return 0, "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Range", fmt.Sprintf("bytes */%d", size))
	res, err := s.youtubeHTTP.Do(req)
	if err != nil {
		return 0, "", false, fmt.Errorf("youtube query upload: %w", stripURLError(err))
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK, http.StatusCreated:
		id, err := youtubeVideoID(res.Body)
		return size, id, false, err
	case 308: // Resume Incomplete
		return youtubeNextOffset(res.Header.Get("Range")), "", false, nil
	case http.StatusNotFound, http.StatusGone:
		return 0, "", true, nil
	default:
		return 0, "", false, fmt.Errorf("youtube query upload: %s", youtubeResponseError(res))
	}
}

func (s *replayServer) youtubeUploadFrom(ctx context.Context, token string, rec *youtubePublication, offset int64) (string, error) {
	obj, err := s.store.GetObject(ctx, rec.SourceKey, fmt.Sprintf("bytes=%d-", offset))
	if err != nil {
		return "", fmt.Errorf("read replay MP4: %w", err)
	}
	defer obj.Body.Close()
	var body io.Reader = obj.Body
	if progress := s.youtubeUploadProgress(rec.RunID); progress != nil {
		progress.Store(offset)
		body = countingReader{obj.Body, progress}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, rec.SessionURI, body)
	if err != nil {
		return "", err
	}
	req.ContentLength = rec.SourceSize - offset
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "video/mp4")
	if offset > 0 {
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, rec.SourceSize-1, rec.SourceSize))
	}
	res, err := s.youtubeHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("youtube upload: %w", stripURLError(err))
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("youtube upload: %s", youtubeResponseError(res))
	}
	return youtubeVideoID(res.Body)
}

func youtubeVideoID(body io.Reader) (string, error) {
	var video struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(&video); err != nil || video.ID == "" {
		return "", errors.New("youtube upload finished without a video id")
	}
	return video.ID, nil
}

// youtubeNextOffset parses a 308 "Range: bytes=0-N" header; no header means
// YouTube holds nothing yet.
func youtubeNextOffset(header string) int64 {
	_, last, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(header), "bytes="), "-")
	if !ok {
		return 0
	}
	n, err := strconv.ParseInt(last, 10, 64)
	if err != nil {
		return 0
	}
	return n + 1
}

func youtubeResponseError(res *http.Response) string {
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
	return strings.TrimSpace(res.Status + " " + string(data))
}

// stripURLError drops the request URL from transport errors: a resumable
// session URI authorizes writes to that upload and must not reach logs or
// status responses.
func stripURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
