package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

// replayROMLibrary is the replay sidecar's mounted cartridge set. Paths are
// keyed by the semantic game detected from their bytes, never by filename.
// Recording ROM SHA-256 remains the final identity check in prepareStreamROM.
type replayROMLibrary struct {
	primary  string
	paths    map[game.GameID]string
	cacheDir string
	mu       sync.Mutex
}

const (
	// replayROMObjectPrefix matches the farm worker store layout: one object
	// per semantic game id, not per filename.
	replayROMObjectPrefix = "roms/"
	// maxReplayROMBytes bounds a store download. The largest Game Boy
	// cartridge is 8 MiB; this leaves headroom for a mis-published object.
	maxReplayROMBytes = 16 << 20
)

func buildReplayROMLibrary(primaryPath, romDir string) *replayROMLibrary {
	lib := &replayROMLibrary{
		primary:  strings.TrimSpace(primaryPath),
		paths:    map[game.GameID]string{},
		cacheDir: filepath.Join(os.TempDir(), "pokereplay-roms"),
	}
	seen := map[string]bool{}
	for _, candidate := range []string{primaryPath, romDir} {
		for _, path := range replayROMFilesIn(candidate) {
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			lib.add(path)
		}
	}
	return lib
}

func (l *replayROMLibrary) add(path string) {
	rom, err := os.ReadFile(path)
	if err != nil {
		log.Printf("pokereplay: ignoring unreadable ROM %s: %v", path, err)
		return
	}
	profile, _, err := profiles.DetectCartridge(rom)
	if err != nil {
		log.Printf("pokereplay: ignoring unrecognised ROM %s: %v", path, err)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.remember(profile.ID(), path)
}

func (l *replayROMLibrary) remember(id game.GameID, path string) {
	if l.paths == nil {
		l.paths = map[game.GameID]string{}
	}
	if _, exists := l.paths[id]; !exists {
		l.paths[id] = path
	}
}

// candidates returns the only cartridges that may back a recording. New
// recordings carry a semantic game id and therefore fail closed if that game
// is not mounted. Legacy recordings predate that metadata, so they try every
// detected cartridge and let the recording ROM hash select the exact image.
func (l *replayROMLibrary) candidates(metadata map[string]string) []string {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if id := game.GameID(strings.TrimSpace(metadata["game"])); id != "" {
		if path := l.paths[id]; path != "" {
			return []string{path}
		}
		return nil
	}

	paths := make([]string, 0, len(l.paths)+1)
	seen := map[string]bool{}
	if l.primary != "" {
		paths = append(paths, l.primary)
		seen[l.primary] = true
	}
	ids := make([]string, 0, len(l.paths))
	for id := range l.paths {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, raw := range ids {
		path := l.paths[game.GameID(raw)]
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

func replayROMFilesIn(candidate string) []string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return nil
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		return []string{candidate}
	}
	entries, err := os.ReadDir(candidate)
	if err != nil {
		log.Printf("pokereplay: cannot read ROM directory %s: %v", candidate, err)
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		out = append(out, filepath.Join(candidate, entry.Name()))
	}
	return out
}

// fetchStored downloads roms/<game-id> from the same object store farm workers
// use when the render host did not mount that cartridge. The bytes are checked
// with profiles.DetectCartridge before they are cached, so a misnamed object
// cannot be remembered as the requested game. A nil store keeps the historical
// fail-closed error: the game is not mounted and there is nowhere to fetch it.
func (l *replayROMLibrary) fetchStored(ctx context.Context, store *artifactstore.S3, metadata map[string]string) (string, error) {
	id := game.GameID(strings.TrimSpace(metadata["game"]))
	if id == "" {
		return "", nil
	}
	if l == nil || store == nil {
		return "", fmt.Errorf("no mounted replay ROM for game %q", id)
	}
	l.mu.Lock()
	if path := l.paths[id]; path != "" {
		l.mu.Unlock()
		return path, nil
	}
	cacheDir := l.cacheDir
	l.mu.Unlock()
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "pokereplay-roms")
	}
	path, err := fetchReplayROMObject(ctx, store, cacheDir, id)
	if err != nil {
		return "", fmt.Errorf("no mounted replay ROM for game %q: %w", id, err)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if existing := l.paths[id]; existing != "" {
		return existing, nil
	}
	l.remember(id, path)
	return path, nil
}

func fetchReplayROMObject(ctx context.Context, store *artifactstore.S3, cacheDir string, id game.GameID) (string, error) {
	path := filepath.Join(cacheDir, string(id))
	if rom, err := os.ReadFile(path); err == nil {
		if profile, _, detectErr := profiles.DetectCartridge(rom); detectErr == nil && profile.ID() == id {
			return path, nil
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return "", fmt.Errorf("discard invalid cached ROM %s: %w", path, removeErr)
		}
	}
	key := replayROMObjectPrefix + string(id)
	obj, err := store.GetObject(ctx, key, "")
	if err != nil {
		return "", fmt.Errorf("fetch %s from ROM store: %w", key, err)
	}
	defer obj.Body.Close()
	rom, err := io.ReadAll(io.LimitReader(obj.Body, maxReplayROMBytes+1))
	if err != nil {
		return "", fmt.Errorf("fetch %s from ROM store: %w", key, err)
	}
	if len(rom) > maxReplayROMBytes {
		return "", fmt.Errorf("ROM store object %s exceeds %d bytes", key, maxReplayROMBytes)
	}
	profile, _, err := profiles.DetectCartridge(rom)
	if err != nil || profile.ID() != id {
		return "", fmt.Errorf("ROM store object %s is not game %q", key, id)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(cacheDir, string(id)+".*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(rom); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	log.Printf("pokereplay: fetched %s from ROM store (%d bytes)", id, len(rom))
	return path, nil
}
