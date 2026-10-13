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
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
)

// YouTube VOD publishing uploads the already-cached replay MP4; it never
// re-renders or replays gameplay. Publication state lives in its own S3
// record (publications/youtube/<run>.json), separate from run status, so an
// upload failure can never change a run's result or its replay artifact.

const (
	youtubeUploadTimeout   = 6 * time.Hour
	youtubeUploadAttempts  = 5
	youtubeTitleMaxRunes   = 100
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
		RunURLTemplate: strings.TrimSpace(getenv("POKEPILOT_YOUTUBE_RUN_URL_TEMPLATE")),
		TokenURL:       youtubeDefaultTokenURL,
		UploadURL:      youtubeDefaultUploadURL,
	}
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
	if s.youtubeActive[runID] {
		return false
	}
	s.youtubeActive[runID] = true
	return true
}

func (s *replayServer) releaseYouTubeUpload(runID string) {
	s.youtubeMu.Lock()
	delete(s.youtubeActive, runID)
	s.youtubeMu.Unlock()
}

func (s *replayServer) youtubeUploadActive(runID string) bool {
	s.youtubeMu.Lock()
	defer s.youtubeMu.Unlock()
	return s.youtubeActive[runID]
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
	if rec.State == youtubeStateUploading && !s.youtubeUploadActive(runID) {
		rec.State = youtubeStateInterrupted
	}
	writeJSON(w, http.StatusOK, map[string]any{"configured": s.youtube.configured(), "publication": rec.view()})
}

type youtubePublishRequest struct {
	Privacy string `json:"privacy,omitempty"`
}

// handleYouTubePublish starts (or resumes) the upload of a run's ready replay
// MP4. A run that is already published returns its existing video instead of
// uploading again.
func (s *replayServer) handleYouTubePublish(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	mode, err := parseReplayMode(r.URL.Query().Get("mode"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var req youtubePublishRequest
	if r.ContentLength != 0 {
		dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid publish request: " + err.Error()})
			return
		}
	}
	privacy := strings.ToLower(strings.TrimSpace(req.Privacy))
	switch privacy {
	case "":
		privacy = "unlisted"
	case "private", "unlisted", "public":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "privacy must be private, unlisted, or public"})
		return
	}
	if !s.youtube.configured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "YouTube publishing is not configured on the replay host"})
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "S3 artifact storage is not configured for the replay service"})
		return
	}
	if !s.claimYouTubeUpload(runID) {
		rec, _, _ := s.loadYouTubePublication(r.Context(), runID)
		writeJSON(w, http.StatusAccepted, rec.view())
		return
	}
	started := false
	defer func() {
		if !started {
			s.releaseYouTubeUpload(runID)
		}
	}()

	rec, _, err := s.loadYouTubePublication(r.Context(), runID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": clipError(err)})
		return
	}
	if rec.State == youtubeStatePublished && rec.VideoID != "" {
		writeJSON(w, http.StatusOK, rec.view())
		return
	}
	recordings, err := s.recordings(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	replay := s.replayStatus(r.Context(), runID, recordings, mode)
	if replay.State != "ready" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "replay MP4 is not ready; render the replay first", "replay": replay})
		return
	}
	meta, err := s.youtubeMetaForRun(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	if rec.SourceKey != replay.ObjectKey || rec.SourceSize != replay.Size {
		rec.SessionURI = "" // a different MP4 is a different upload
	}
	rec.RunID = runID
	rec.State = youtubeStateUploading
	rec.Privacy = privacy
	rec.Title = meta.Title
	rec.SourceKey = replay.ObjectKey
	rec.SourceSize = replay.Size
	rec.Error = ""
	if err := s.saveYouTubePublication(r.Context(), rec); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": clipError(err)})
		return
	}
	started = true
	go func() {
		defer s.releaseYouTubeUpload(runID)
		ctx, cancel := context.WithTimeout(context.Background(), youtubeUploadTimeout)
		defer cancel()
		s.publishYouTube(ctx, rec, meta)
	}()
	writeJSON(w, http.StatusAccepted, rec.view())
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, rec.SessionURI, obj.Body)
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

// wallRunForPublish is the subset of pokewall's GET /v1/runs/{id} the
// metadata needs. Traces, decisions, and raw model output are deliberately
// not decoded, so they cannot leak into a public description.
type wallRunForPublish struct {
	Run struct {
		RunID      string       `json:"run_id"`
		Status     string       `json:"status"`
		Game       string       `json:"game"`
		Starter    string       `json:"starter"`
		Goal       string       `json:"goal"`
		Planner    string       `json:"planner"`
		LLMProfile string       `json:"llm_profile"`
		QueuedAt   int64        `json:"queued_at"`
		EndedAt    int64        `json:"ended_at"`
		Attempts   int          `json:"attempts"`
		Reason     string       `json:"reason"`
		Player     *farm.Player `json:"player"`
	} `json:"run"`
	Finish *struct {
		Reason        string         `json:"reason"`
		RunnerVersion string         `json:"runner_version"`
		ProgressFinal *farm.Progress `json:"progress_final"`
	} `json:"finish"`
}

func (s *replayServer) youtubeMetaForRun(ctx context.Context, runID string) (youtubeVideoMeta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.wallBase+"/v1/runs/"+url.PathEscape(runID), nil)
	if err != nil {
		return youtubeVideoMeta{}, err
	}
	res, err := s.wallHTTP.Do(req)
	if err != nil {
		return youtubeVideoMeta{}, fmt.Errorf("pokewall unavailable: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return youtubeVideoMeta{}, errRunNotFound
	}
	if res.StatusCode != http.StatusOK {
		return youtubeVideoMeta{}, fmt.Errorf("pokewall returned %s", res.Status)
	}
	var run wallRunForPublish
	if err := json.NewDecoder(io.LimitReader(res.Body, maxWallResponseBytes)).Decode(&run); err != nil {
		return youtubeVideoMeta{}, fmt.Errorf("decode pokewall run: %w", err)
	}
	if run.Run.RunID == "" {
		run.Run.RunID = runID
	}
	return buildYouTubeMeta(run, s.youtube.RunURLTemplate), nil
}

// buildYouTubeMeta is deterministic: the same run record always produces the
// same title and description. It reads only game-neutral run fields.
func buildYouTubeMeta(run wallRunForPublish, runURLTemplate string) youtubeVideoMeta {
	r := run.Run
	game := r.Game
	if game == "" {
		game = "Game Boy"
	}
	badges := -1
	if run.Finish != nil && run.Finish.ProgressFinal != nil {
		badges = run.Finish.ProgressFinal.Badges
	} else if r.Player != nil && r.Player.Badges != nil {
		badges = len(r.Player.Badges)
	}
	result := r.Reason
	if run.Finish != nil && run.Finish.Reason != "" {
		result = run.Finish.Reason
	}
	if result == "" {
		result = r.Status
	}

	titleParts := []string{"PokePilot AI plays " + game}
	if r.Starter != "" {
		titleParts = append(titleParts, r.Starter)
	}
	if badges >= 0 {
		titleParts = append(titleParts, plural(badges, "badge"))
	}
	if result != "" {
		titleParts = append(titleParts, result)
	}
	title := youtubeSafeText(strings.Join(titleParts, " · "))
	if runes := []rune(title); len(runes) > youtubeTitleMaxRunes {
		title = string(runes[:youtubeTitleMaxRunes-1]) + "…"
	}

	var d strings.Builder
	fmt.Fprintf(&d, "An autonomous PokePilot run of %s, replayed from its deterministic recording.\n\n", game)
	line := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(&d, "%s: %s\n", label, value)
		}
	}
	line("Run", r.RunID)
	line("Game", r.Game)
	line("Starter", r.Starter)
	if r.Player != nil && len(r.Player.Party) > 0 {
		party := make([]string, 0, len(r.Player.Party))
		for _, mon := range r.Player.Party {
			party = append(party, fmt.Sprintf("%s Lv%d", mon.Name, mon.Level))
		}
		line("Final party", strings.Join(party, ", "))
	}
	line("Goal", r.Goal)
	line("Result", result)
	if badges >= 0 {
		line("Badges", strconv.Itoa(badges))
	}
	if run.Finish != nil && run.Finish.ProgressFinal != nil && run.Finish.ProgressFinal.Maps > 0 {
		line("Maps visited", strconv.Itoa(run.Finish.ProgressFinal.Maps))
	}
	if r.QueuedAt > 0 && r.EndedAt > r.QueuedAt {
		line("Duration", (time.Duration(r.EndedAt-r.QueuedAt) * time.Second).String())
	}
	if r.Attempts > 1 {
		line("Attempts", strconv.Itoa(r.Attempts))
	}
	line("Planner", r.Planner)
	line("Model profile", r.LLMProfile)
	if run.Finish != nil {
		line("PokePilot build", run.Finish.RunnerVersion)
	}
	if runURLTemplate != "" {
		line("Run page", strings.ReplaceAll(runURLTemplate, "{run_id}", url.PathEscape(r.RunID)))
	}
	desc := youtubeSafeText(d.String())
	if len(desc) > youtubeDescMaxBytes {
		desc = strings.ToValidUTF8(desc[:youtubeDescMaxBytes], "")
	}
	return youtubeVideoMeta{Title: title, Description: desc}
}

// youtubeSafeText removes angle brackets, which the YouTube API rejects in
// titles and descriptions.
func youtubeSafeText(s string) string {
	return strings.NewReplacer("<", "", ">", "").Replace(s)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
