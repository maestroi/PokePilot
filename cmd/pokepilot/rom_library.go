package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
	"github.com/maestroi/pokepilot/skill"
)

// romLibrary is the set of cartridges this worker can run, keyed by the game
// each file actually is. Identity comes from profiles.Detect on the bytes,
// never from a filename: an operator's roms/ directory holds pokemon_red.gb
// and pokemon_blue.gb, and a third game needs no code or deploy change here.
type romLibrary struct {
	paths      map[game.GameID]string
	primary    game.GameID
	bootStates map[game.GameID][]byte
}

// buildROMLibrary enumerates the mounted ROM directory (POKEPILOT_ROM_DIR),
// plus the one cartridge this process already opened when it is set. bootState
// is the post-boot state already captured for primaryPath, so the game the
// worker started on is never booted twice.
func buildROMLibrary(primaryPath string, bootState []byte) *romLibrary {
	lib := &romLibrary{
		paths:      map[game.GameID]string{},
		bootStates: map[game.GameID][]byte{},
	}
	seen := map[string]bool{}
	candidates := []string{primaryPath, primaryGameDir(primaryPath)}
	if dir := os.Getenv("POKEPILOT_ROM_DIR"); dir != "" {
		candidates = append(candidates, dir)
	}
	for _, candidate := range candidates {
		for _, path := range romFilesIn(candidate) {
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			lib.add(path, bootState, primaryPath)
		}
	}
	return lib
}

// add registers path's detected game, seeding the boot state when path is the
// cartridge this process already booted. Unrecognised files are skipped with a
// warning rather than fataling the worker: an operator's ROM directory may
// hold an image this build has no adapter for yet.
func (l *romLibrary) add(path string, bootState []byte, primaryPath string) {
	rom, err := os.ReadFile(path)
	if err != nil {
		log.Printf("farm: ignoring unreadable ROM %s: %v", path, err)
		return
	}
	profile, _, err := profiles.Detect(rom)
	if err != nil {
		log.Printf("farm: ignoring unrecognised ROM %s: %v", path, err)
		return
	}
	id := profile.ID()
	if _, exists := l.paths[id]; !exists {
		l.paths[id] = path
	}
	if path == primaryPath {
		l.primary = id
		if bootState != nil {
			l.bootStates[id] = bootState
		}
	}
}

// romFilesIn expands one candidate, which is either a directory or a single
// file. Subdirectories are not descended: /rom is flat by contract.
func romFilesIn(candidate string) []string {
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
		log.Printf("farm: cannot read ROM directory %s: %v", candidate, err)
		return nil
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		out = append(out, filepath.Join(candidate, entry.Name()))
	}
	return out
}

// primaryGameDir is the directory the primary ROM lives in, so a deployment
// that mounts a directory but only names one file still finds its siblings.
func primaryGameDir(primaryPath string) string {
	if primaryPath == "" {
		return ""
	}
	return filepath.Dir(primaryPath)
}

// bootStateFor returns the post-boot state for the requested game, loading and
// booting the cartridge first when this worker is not already on it. An empty
// game means no preference and resolves to the cartridge the worker started
// on, which keeps older specs and one-ROM deployments working unchanged.
func (l *romLibrary) bootStateFor(m *emu.Emu, id game.GameID) ([]byte, error) {
	if id == "" {
		id = l.primary
		if id == "" {
			return nil, fmt.Errorf("no supported ROM mounted for this worker; checked %s", l.checked())
		}
	}
	if state := l.bootStates[id]; state != nil {
		return state, nil
	}
	path, ok := l.paths[id]
	if !ok {
		return nil, fmt.Errorf("this worker has no ROM for game %q; mounted: %s", id, l.games())
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ROM for %s: %w", id, err)
	}
	// Re-detect the bytes being loaded: the library was built once at startup,
	// and this is the trust boundary that proves the file still is the game
	// the lease asked for.
	if profile, _, err := profiles.Detect(rom); err != nil || profile.ID() != id {
		return nil, fmt.Errorf("ROM %s is not game %q", path, id)
	}
	if err := m.LoadROMBytes(rom, string(id)); err != nil {
		return nil, fmt.Errorf("load %s: %w", id, err)
	}
	m.Pace(0) // boot unthrottled; runOne sets the run's pace afterwards
	if _, err := skill.BootToOverworld(m); err != nil {
		return nil, fmt.Errorf("boot %s: %w", id, err)
	}
	state, err := m.SaveState()
	if err != nil {
		return nil, fmt.Errorf("save %s boot state: %w", id, err)
	}
	l.bootStates[id] = state
	log.Printf("farm: booted %s from %s", id, path)
	return state, nil
}

func (l *romLibrary) games() string {
	out := make([]string, 0, len(l.paths))
	for id := range l.paths {
		out = append(out, string(id))
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// checked names every candidate the library looked at, for error messages when
// nothing usable turned up.
func (l *romLibrary) checked() string {
	paths := make([]string, 0, len(l.paths))
	for _, path := range l.paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return strings.Join(paths, ", ")
}
