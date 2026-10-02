package rom

import (
	"os"
	"testing"
)

func loadROM(t *testing.T, env string) []byte {
	t.Helper()
	path := os.Getenv(env)
	if path == "" {
		t.Skip(env + " not set")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", env, err)
	}
	return rom
}

func loadGold(t *testing.T) []byte   { return loadROM(t, "POKEMON_GOLD_ROM") }
func loadSilver(t *testing.T) []byte { return loadROM(t, "POKEMON_SILVER_ROM") }
