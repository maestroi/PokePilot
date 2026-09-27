package profiles

import (
	"os"
	"testing"

	blueprofile "github.com/maestroi/pokepilot/blue/profile"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	redprofile "github.com/maestroi/pokepilot/red/profile"
	tetrisprofile "github.com/maestroi/pokepilot/tetris/profile"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestBuiltinProfilesSatisfyContract(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	profiles := registry.Profiles()
	if len(profiles) != 5 {
		t.Fatalf("built-in profile count = %d, want 5", len(profiles))
	}
	seen := map[game.GameID]bool{}
	for _, p := range profiles {
		if err := game.ValidateProfileContract(p); err != nil {
			t.Fatal(err)
		}
		seen[p.ID()] = true
	}
	for _, want := range []game.GameID{
		redprofile.GameID,
		blueprofile.GameID,
		yellowprofile.GameID,
		gsprofile.GoldGameID,
		gsprofile.SilverGameID,
	} {
		if !seen[want] {
			t.Fatalf("built-in profile ids = %v, missing %s", seen, want)
		}
	}
}


func TestBuiltinCartridgesIncludeTetrisWithoutPokemonContract(t *testing.T) {
	registry, err := Cartridges()
	if err != nil {
		t.Fatal(err)
	}
	profiles := registry.Profiles()
	if len(profiles) != 6 {
		t.Fatalf("built-in cartridge profile count = %d, want 6", len(profiles))
	}
	seen := map[game.GameID]bool{}
	for _, p := range profiles {
		if err := game.ValidateCartridgeProfileContract(p); err != nil {
			t.Fatal(err)
		}
		seen[p.ID()] = true
	}
	for _, want := range []game.GameID{
		redprofile.GameID,
		blueprofile.GameID,
		yellowprofile.GameID,
		gsprofile.GoldGameID,
		gsprofile.SilverGameID,
		tetrisprofile.GameID,
	} {
		if !seen[want] {
			t.Fatalf("built-in cartridge profile ids = %v, missing %s", seen, want)
		}
	}
	if _, ok := any(tetrisprofile.New()).(game.GameProfile); ok {
		t.Fatal("Tetris must not implement the Pokémon GameProfile contract")
	}
}

func TestTetrisCartridgeResolvesWhenROMAvailable(t *testing.T) {
	path := os.Getenv("TETRIS_ROM")
	if path == "" {
		path = "roms/tetris.gb"
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("TETRIS_ROM: %v", err)
	}
	profile, info, err := DetectCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID() != tetrisprofile.GameID {
		t.Fatalf("Tetris resolved to %s, want %s", profile.ID(), tetrisprofile.GameID)
	}
	if info.SHA1 != tetrisprofile.ROMSHA1 || info.SHA256 != tetrisprofile.ROMSHA256 {
		t.Fatalf("unexpected Tetris fingerprint: sha1=%s sha256=%s", info.SHA1, info.SHA256)
	}
}

// TestEachRegisteredImageResolvesToItsOwnProfile is the guard the registry
// exists for: all registered images must still each resolve to exactly one
// profile because every other layer dispatches on that id.
func TestEachRegisteredImageResolvesToItsOwnProfile(t *testing.T) {
	for _, tc := range []struct {
		env, fallback string
		want          game.GameID
	}{
		{"POKEMON_RED_ROM", "roms/pokemon_red.gb", redprofile.GameID},
		{"POKEMON_BLUE_ROM", "roms/pokemon_blue.gb", blueprofile.GameID},
		{"POKEMON_YELLOW_ROM", "roms/pokemon_yellow.gb", yellowprofile.GameID},
		{"POKEMON_GOLD_ROM", "roms/pokemon_gold.gbc", gsprofile.GoldGameID},
		{"POKEMON_SILVER_ROM", "roms/pokemon_silver.gbc", gsprofile.SilverGameID},
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

func TestGen1ProfilesShareSemanticBootBoundary(t *testing.T) {
	for _, p := range []game.GameProfile{
		redprofile.New(),
		blueprofile.New(),
		yellowprofile.New(),
	} {
		if _, ok := p.(game.BootProfile); !ok {
			t.Errorf("%s@%s does not implement game.BootProfile", p.ID(), p.Revision())
		}
	}
}
