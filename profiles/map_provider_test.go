package profiles

import (
	"os"
	"testing"
)

func TestMapProviderRejectsUnknownROM(t *testing.T) {
	if _, err := MapProvider(make([]byte, 0x8000)); err == nil {
		t.Fatal("unknown ROM resolved a map provider")
	}
}

func TestMapProviderResolvesRedThroughProfile(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := MapProvider(rom)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.MapIDs()) == 0 {
		t.Fatal("Red map provider enumerates no maps")
	}
}
