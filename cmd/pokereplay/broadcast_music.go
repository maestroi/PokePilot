package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
)

const (
	broadcastMusicMaxBytes = 20 << 20
	broadcastMusicMaxTracks = 32
	broadcastMusicMaxCatalogBytes = 48 << 10
	broadcastMusicCatalogKey = "media/broadcast/music/library-v1.json"
)

type broadcastMusicTrack struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Format string `json:"format"`
	Size int64 `json:"size"`
	UploadedAtMS int64 `json:"uploaded_at_unix_ms"`
}

type broadcastMusicLibrary struct {
	Tracks []broadcastMusicTrack `json:"tracks"`
}

func broadcastMusicKey(track broadcastMusicTrack) (string, error) {
	if len(track.ID) != 64 || strings.Trim(track.ID, "0123456789abcdef") != "" {
		return "", errors.New("invalid music track identity")
	}
	switch track.Format {
	case "mp3", "ogg", "wav", "flac", "m4a":
		return "media/broadcast/music/tracks/" + track.ID + "." + track.Format, nil
	default:
		return "", errors.New("invalid music track format")
	}
}

func musicFormat(filename string, data []byte) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	header := data
	if len(header) > 16 {
		header = header[:16]
	}
	switch {
	case ext == ".mp3" && (bytes.HasPrefix(header, []byte("ID3")) ||
		(len(header) >= 2 && header[0] == 0xff && header[1]&0xe0 == 0xe0)):
		return "mp3", "audio/mpeg", nil
	case ext == ".ogg" && bytes.HasPrefix(header, []byte("OggS")):
		return "ogg", "audio/ogg", nil
	case ext == ".wav" && len(header) >= 12 && bytes.Equal(header[:4], []byte("RIFF")) && bytes.Equal(header[8:12], []byte("WAVE")):
		return "wav", "audio/wav", nil
	case ext == ".flac" && bytes.HasPrefix(header, []byte("fLaC")):
		return "flac", "audio/flac", nil
	case ext == ".m4a" && len(header) >= 12 && bytes.Equal(header[4:8], []byte("ftyp")):
		return "m4a", "audio/mp4", nil
	default:
		return "", "", errors.New("unsupported music format (MP3, OGG, WAV, FLAC or M4A)")
	}
}

func safeMusicName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 120 || strings.ContainsAny(name, "/\\\x00\r\n") || name == "." || name == ".." {
		return "", errors.New("invalid music filename")
	}
	return name, nil
}

func (s *replayServer) loadMusicLibrary(ctx context.Context) (broadcastMusicLibrary, error) {
	lib := broadcastMusicLibrary{Tracks: []broadcastMusicTrack{}}
	if s.store == nil {
		return lib, errors.New("S3 artifact storage is not configured")
	}
	obj, err := s.store.GetObject(ctx, broadcastMusicCatalogKey, "")
	if artifactstore.IsNotFound(err) {
		return lib, nil
	}
	if err != nil {
		return lib, err
	}
	defer obj.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(obj.Body, broadcastMusicMaxCatalogBytes+1))
	if err != nil || len(raw) > broadcastMusicMaxCatalogBytes {
		return lib, errors.New("broadcast music catalog cannot be read")
	}
	if err := json.Unmarshal(raw, &lib); err != nil {
		return lib, errors.New("broadcast music catalog is invalid")
	}
	if len(lib.Tracks) > broadcastMusicMaxTracks {
		return lib, errors.New("broadcast music catalog exceeds track limit")
	}
	for _, track := range lib.Tracks {
		if _, err := broadcastMusicKey(track); err != nil || track.Size <= 0 || track.Size > broadcastMusicMaxBytes {
			return lib, errors.New("broadcast music catalog has an invalid track")
		}
	}
	if lib.Tracks == nil {
		lib.Tracks = []broadcastMusicTrack{}
	}
	return lib, nil
}

func (s *replayServer) handleBroadcastMusicList(w http.ResponseWriter, r *http.Request) {
	lib, err := s.loadMusicLibrary(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "broadcast music library is unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, lib)
}

// Uploading is operator-only (the private pokeui proxy exposes this route).
// The browser can supply bytes and a display name, but never an S3 key or path.
func (s *replayServer) handleBroadcastMusicUpload(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "S3 artifact storage is not configured"})
		return
	}
	if r.URL.Query().Get("licensed") != "true" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "confirm you have permission to broadcast the uploaded audio"})
		return
	}
	name, err := safeMusicName(r.URL.Query().Get("name"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if r.ContentLength > broadcastMusicMaxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "music track exceeds 20 MiB"})
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, broadcastMusicMaxBytes+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "music upload could not be read"})
		return
	}
	if len(data) == 0 || len(data) > broadcastMusicMaxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "music track must be between 1 byte and 20 MiB"})
		return
	}
	format, mediaType, err := musicFormat(name, data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	digest := sha256.Sum256(data)
	track := broadcastMusicTrack{
		ID: hex.EncodeToString(digest[:]), Name: name, Format: format,
		Size: int64(len(data)), UploadedAtMS: time.Now().UnixMilli(),
	}
	key, _ := broadcastMusicKey(track)

	s.musicMu.Lock()
	defer s.musicMu.Unlock()
	lib, err := s.loadMusicLibrary(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "broadcast music library is unavailable"})
		return
	}
	for _, existing := range lib.Tracks {
		if existing.ID == track.ID && existing.Format == track.Format {
			writeJSON(w, http.StatusOK, existing)
			return
		}
	}
	if len(lib.Tracks) >= broadcastMusicMaxTracks {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "music library is full (32 tracks)"})
		return
	}
	if _, err := s.store.PutObject(r.Context(), key, mediaType, data); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "cannot store music track in S3"})
		return
	}
	lib.Tracks = append(lib.Tracks, track)
	catalog, err := json.Marshal(lib)
	if err != nil || len(catalog) > broadcastMusicMaxCatalogBytes {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "music library metadata is too large"})
		return
	}
	if _, err := s.store.PutObject(r.Context(), broadcastMusicCatalogKey, "application/json", catalog); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "cannot update music library in S3"})
		return
	}
	writeJSON(w, http.StatusCreated, track)
}

// stageBroadcastMusic fetches only a known catalog track and returns a private,
// bounded local file. It is never a presigned URL passed to FFmpeg.
func (s *replayServer) stageBroadcastMusic(ctx context.Context, trackID string) (string, func(), error) {
	lib, err := s.loadMusicLibrary(ctx)
	if err != nil {
		return "", nil, errors.New("music library is unavailable")
	}
	var selected *broadcastMusicTrack
	for i := range lib.Tracks {
		if lib.Tracks[i].ID == trackID {
			selected = &lib.Tracks[i]
			break
		}
	}
	if selected == nil {
		return "", nil, errors.New("selected music track is not in the library")
	}
	key, err := broadcastMusicKey(*selected)
	if err != nil {
		return "", nil, err
	}
	obj, err := s.store.GetObject(ctx, key, "")
	if err != nil {
		return "", nil, errors.New("selected music track is unavailable in S3")
	}
	defer obj.Body.Close()
	file, err := os.CreateTemp("", "pokepilot-broadcast-music-*."+selected.Format)
	if err != nil {
		return "", nil, fmt.Errorf("cannot create private music file: %w", err)
	}
	cleanup := func() { _ = os.Remove(file.Name()) }
	_ = file.Chmod(0o600)
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(file, h), io.LimitReader(obj.Body, broadcastMusicMaxBytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || n != selected.Size || hex.EncodeToString(h.Sum(nil)) != selected.ID {
		cleanup()
		return "", nil, errors.New("music track download failed integrity validation")
	}
	return file.Name(), cleanup, nil
}
