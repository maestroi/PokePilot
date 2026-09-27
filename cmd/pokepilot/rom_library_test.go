package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

// TestROMLibraryFetchesMissingGameFromStore proves a worker with no local
// cartridge for a game pulls it from the ROM store, and refuses to cache an
// object whose bytes are a different game than its key claims.
func TestROMLibraryFetchesMissingGameFromStore(t *testing.T) {
	romPath := os.Getenv("POKEMON_BLUE_ROM")
	if romPath == "" {
		romPath = "../../roms/pokemon_blue.gb"
	}
	blue, err := os.ReadFile(romPath)
	if err != nil {
		t.Skipf("Blue ROM unavailable: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pokepilot/roms/pokemon-blue", "/pokepilot/roms/pokemon-red": // red key serves blue bytes
			_, _ = w.Write(blue)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	store, err := artifactstore.NewS3(artifactstore.S3Config{
		Endpoint: srv.URL, Bucket: "pokepilot", AccessKey: "test", SecretKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	lib := &romLibrary{paths: map[game.GameID]string{}, remote: store, cacheDir: t.TempDir()}

	path, err := lib.fetch("pokemon-blue")
	if err != nil {
		t.Fatalf("fetch blue: %v", err)
	}
	if got, _ := os.ReadFile(path); len(got) != len(blue) {
		t.Fatalf("cached %d bytes, want %d", len(got), len(blue))
	}

	if _, err := lib.fetch("pokemon-red"); err == nil {
		t.Fatal("fetch red accepted Blue bytes")
	}
	if _, err := os.Stat(filepath.Join(lib.cacheDir, "pokemon-red")); !os.IsNotExist(err) {
		t.Fatalf("mismatched object was cached: %v", err)
	}
	if _, err := lib.fetch("pokemon-yellow"); err == nil {
		t.Fatal("fetch of absent object succeeded")
	}
}


// TestROMLibraryCachedBootStateSwitchesCartridge reproduces #2000: a worker
// can cache Blue's neutral boot state, run Red, then lease Blue again. The
// cached state is only RAM/CPU state; the cartridge itself still has to be
// switched back to Blue before that state (or a Blue checkpoint) is restored.
func TestROMLibraryCachedBootStateSwitchesCartridge(t *testing.T) {
	redPath := os.Getenv("POKEMON_RED_ROM")
	if redPath == "" {
		redPath = "../../roms/pokemon_red.gb"
	}
	bluePath := os.Getenv("POKEMON_BLUE_ROM")
	if bluePath == "" {
		bluePath = "../../roms/pokemon_blue.gb"
	}
	if _, err := os.Stat(redPath); err != nil {
		t.Skipf("Red ROM unavailable: %v", err)
	}
	if _, err := os.Stat(bluePath); err != nil {
		t.Skipf("Blue ROM unavailable: %v", err)
	}

	m, err := emu.Open(redPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })

	cached := []byte("cached-blue-boot-state")
	lib := &romLibrary{
		paths: map[game.GameID]string{
			"pokemon-red":  redPath,
			"pokemon-blue": bluePath,
		},
		primary: "pokemon-red",
		bootStates: map[game.GameID][]byte{
			"pokemon-blue": cached,
		},
	}

	got, err := lib.bootStateFor(m, "pokemon-blue")
	if err != nil {
		t.Fatalf("bootStateFor blue: %v", err)
	}
	if !bytes.Equal(got, cached) {
		t.Fatalf("bootStateFor returned %q, want cached state %q", got, cached)
	}
	active, _, err := profiles.DetectCartridge(m.ROM())
	if err != nil {
		t.Fatalf("detect active cartridge: %v", err)
	}
	if active.ID() != "pokemon-blue" {
		t.Fatalf("active cartridge = %q, want pokemon-blue", active.ID())
	}
}
