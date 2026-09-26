package main

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

// replayROMLibrary is the replay sidecar's mounted cartridge set. Paths are
// keyed by the semantic game detected from their bytes, never by filename.
// Recording ROM SHA-256 remains the final identity check in prepareStreamROM.
type replayROMLibrary struct {
	primary string
	paths   map[game.GameID]string
}

func buildReplayROMLibrary(primaryPath, romDir string) *replayROMLibrary {
	lib := &replayROMLibrary{
		primary: strings.TrimSpace(primaryPath),
		paths:   map[game.GameID]string{},
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
	profile, _, err := profiles.Detect(rom)
	if err != nil {
		log.Printf("pokereplay: ignoring unrecognised ROM %s: %v", path, err)
		return
	}
	if _, exists := l.paths[profile.ID()]; !exists {
		l.paths[profile.ID()] = path
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
