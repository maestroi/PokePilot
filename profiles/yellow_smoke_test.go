package profiles_test

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/profiles"
	"github.com/maestroi/pokepilot/skill"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

// TestYellowROMBootAndProfileSelection is the opt-in cartridge compatibility
// smoke. It proves the shared fresh-game driver can reach Yellow's profile-owned
// controllable starting state without using Red RAM addresses.
func TestYellowROMBootAndProfileSelection(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed Yellow boot smoke")
	}
	path := os.Getenv("POKEMON_YELLOW_ROM")
	if path == "" {
		path = "roms/pokemon_yellow.gb"
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("POKEMON_YELLOW_ROM: %v", err)
	}

	m, err := emu.OpenCGB(path)
	if err != nil {
		t.Fatalf("OpenCGB Yellow: %v", err)
	}
	defer m.Close()

	profile, info, err := profiles.Detect(m.ROM())
	if err != nil {
		t.Fatalf("Detect Yellow: %v", err)
	}
	if profile.ID() != yellowprofile.GameID || profile.Revision() != yellowprofile.Revision {
		t.Fatalf("detected %s@%s, want %s@%s",
			profile.ID(), profile.Revision(), yellowprofile.GameID, yellowprofile.Revision)
	}
	if info.SHA1 == "" {
		t.Fatal("Yellow ROM identity did not include a SHA-1 fingerprint")
	}

	obs, err := skill.BootToOverworld(m)
	if err != nil {
		t.Fatalf("BootToOverworld Yellow: %v", err)
	}
	if obs.NativeMapID != 0x26 || obs.MapName != "REDS_HOUSE_2F" {
		t.Fatalf("Yellow boot location = %#04x %q, want 0x26 REDS_HOUSE_2F", obs.NativeMapID, obs.MapName)
	}
	if !obs.Controllable {
		t.Fatal("Yellow boot observation is not controllable")
	}
}
