package profiles

import (
	"os"
	"testing"

	blueprofile "github.com/maestroi/pokepilot/blue/profile"
	"github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
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
	seen := map[game.GameID]bool{}
	for _, p := range profiles {
		if err := game.ValidateProfileContract(p); err != nil {
			t.Fatal(err)
		}
		seen[p.ID()] = true
	}
	if !seen[redprofile.GameID] || !seen[blueprofile.GameID] {
		t.Fatalf("built-in profile ids = %v, want both %s and %s", seen, redprofile.GameID, blueprofile.GameID)
	}
}

// TestEachGen1ImageResolvesToItsOwnProfile is the guard the registry exists
// for: two Gen I images sharing one engine must still each resolve to exactly
// one profile, because every other layer dispatches on that id.
func TestEachGen1ImageResolvesToItsOwnProfile(t *testing.T) {
	for _, tc := range []struct {
		env, fallback string
		want          game.GameID
	}{
		{"POKEMON_RED_ROM", "roms/pokemon_red.gb", redprofile.GameID},
		{"POKEMON_BLUE_ROM", "roms/pokemon_blue.gb", blueprofile.GameID},
	} {
		t.Run(string(tc.want), func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				path = tc.fallback
			}
			rom, err := os.ReadFile(path)
			if err != nil {
				t.Skipf("%s: %v", tc.env, err)
			}
			profile, _, err := Detect(rom)
			if err != nil {
				t.Fatalf("%s: %v", tc.env, err)
			}
			if profile.ID() != tc.want {
				t.Fatalf("%s resolved to %s, want %s", tc.env, profile.ID(), tc.want)
			}
		})
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
