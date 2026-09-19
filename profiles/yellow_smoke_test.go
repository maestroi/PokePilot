package profiles_test

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/profiles"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

// TestYellowROMBootAndProfileSelection is the opt-in cartridge compatibility
// smoke for Phase 0. It proves GomeBoy can execute the supported Yellow image
// and that the exact bytes still resolve through the normal profile registry.
//
// It intentionally does not use skill.BootToOverworld: that helper is still a
// Red-owned intro driver and is one of the boundaries Phase 1 must generalize.
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

	// Reach well past reset/initial cartridge startup. Phase 0 only asserts
	// emulator compatibility and identity; intro-driving semantics come later.
	m.StepFrames(300)

	profile, info, err := profiles.Detect(m.ROM())
	if err != nil {
		t.Fatalf("Detect Yellow after boot: %v", err)
	}
	if profile.ID() != yellowprofile.GameID || profile.Revision() != yellowprofile.Revision {
		t.Fatalf("detected %s@%s, want %s@%s",
			profile.ID(), profile.Revision(), yellowprofile.GameID, yellowprofile.Revision)
	}
	if info.SHA1 == "" {
		t.Fatal("Yellow ROM identity did not include a SHA-1 fingerprint")
	}
}
