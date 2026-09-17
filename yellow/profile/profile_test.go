package profile

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestProfileContract(t *testing.T) {
	if err := game.ValidateProfileContract(New()); err != nil {
		t.Fatal(err)
	}
}

// TestIdentity pins the exact supported image. A different Yellow revision
// with the same title must stay unsupported until it has its own tested
// profile; detection by title alone would silently bind the wrong layout.
func TestIdentity(t *testing.T) {
	p := New()
	if p.ID() != GameID || p.Revision() != Revision {
		t.Fatalf("identity = %q@%q, want %q@%q", p.ID(), p.Revision(), GameID, Revision)
	}
	info := game.ROMInfo{Title: "POKEMON YELLOW", SHA1: "deadbeef"}
	if p.Detect(info) {
		t.Fatal("detected an unknown Yellow revision by title")
	}
	info.SHA1 = yellowSHA1
	if !p.Detect(info) {
		t.Fatal("did not detect the supported Yellow image")
	}
}

// yellowSHA1 identifies the supported English Yellow image: it matches
// pokeyellow/roms.sha1 and roms/pokemon_yellow.gb. A test-time ROM is not
// required to exercise detection.
const yellowSHA1 = "cc7d03262ebfaf2f06772c1a480c7d9d5f4a38e1"

// TestDetectAgainstRealROM runs when the actual image is present and proves
// the profile owns the bytes the emulator would run.
func TestDetectAgainstRealROM(t *testing.T) {
	rom, err := os.ReadFile("../../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	info := game.InspectROM(rom)
	if info.SHA1 != yellowSHA1 {
		t.Fatalf("rom sha1 = %s, want %s (detection would bind the wrong layout)", info.SHA1, yellowSHA1)
	}
	if !New().Detect(info) {
		t.Fatal("profile did not detect the real Yellow ROM")
	}
}

func TestSymbolsCoverBaseline(t *testing.T) {
	s := New().Symbols()
	for _, name := range []string{"player.map", "player.x", "player.y", "player.direction",
		"party.count", "party.members", "battle.mode", "badges", "bag", "money"} {
		if _, ok := s[name]; !ok {
			t.Errorf("missing symbol %q", name)
		}
	}
}

func TestMapName(t *testing.T) {
	p := parser{}
	for _, tc := range []struct {
		raw  uint16
		want string
	}{
		{0x00, "PALLET_TOWN"},
		{0x3F, "CERULEAN_MELANIES_HOUSE"},
		{0xF8, "SUMMER_BEACH_HOUSE"},
	} {
		if got, ok := p.MapName(tc.raw); !ok || got != tc.want {
			t.Errorf("MapName(0x%02x) = %q, %v; want %q, true", tc.raw, got, ok, tc.want)
		}
	}
	if _, ok := p.MapName(0xF9); ok {
		t.Error("MapName(0xF9) should be an unused slot")
	}
}

func TestSpecies(t *testing.T) {
	p := parser{}
	for _, tc := range []struct {
		raw  uint16
		want game.SpeciesID
	}{
		{0x15, "mew"},
		{0x54, "pikachu"},
		{0x99, "bulbasaur"},
	} {
		if got, ok := p.Species(tc.raw); !ok || got != tc.want {
			t.Errorf("Species(0x%02x) = %q, %v; want %q, true", tc.raw, got, ok, tc.want)
		}
	}
	if _, ok := p.Species(0x100); ok {
		t.Error("Species(0x100) should be out of range")
	}
}
