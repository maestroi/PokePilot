package profiles

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestBuiltinProfilesSatisfyContract(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	profiles := registry.Profiles()
	if len(profiles) != 2 {
		t.Fatalf("built-in profile count = %d, want 2", len(profiles))
	}
	byID := map[game.GameID]game.GameProfile{}
	for _, p := range profiles {
		if err := game.ValidateProfileContract(p); err != nil {
			t.Fatalf("%s@%s: %v", p.ID(), p.Revision(), err)
		}
		byID[p.ID()] = p
	}
	if _, ok := byID[redprofile.GameID]; !ok {
		t.Fatalf("red profile missing from registry: %v", byID)
	}
	if _, ok := byID[yellowprofile.GameID]; !ok {
		t.Fatalf("yellow profile missing from registry: %v", byID)
	}
}

func TestDetectUnsupportedROMIncludesFingerprint(t *testing.T) {
	rom := make([]byte, 0x150)
	copy(rom[0x134:0x144], []byte("POKEMON RED"))

	_, info, err := Detect(rom)
	if err == nil {
		t.Fatal("expected unsupported revision")
	}
	if info.Title != "POKEMON RED" || info.SHA1 == "" || info.SHA256 == "" {
		t.Fatalf("ROM identity = %#v", info)
	}
}
