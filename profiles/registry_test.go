package profiles

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
)

func TestBuiltinProfilesSatisfyContract(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	profiles := registry.Profiles()
	if len(profiles) != 1 {
		t.Fatalf("built-in profile count = %d, want 1", len(profiles))
	}
	if profiles[0].ID() != redprofile.GameID || profiles[0].Revision() != redprofile.Revision {
		t.Fatalf("built-in profile = %s@%s", profiles[0].ID(), profiles[0].Revision())
	}
	if err := game.ValidateProfileContract(profiles[0]); err != nil {
		t.Fatal(err)
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
