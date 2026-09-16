package profile

import (
	"fmt"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestProfileContract(t *testing.T) {
	if err := game.ValidateProfileContract(New()); err != nil {
		t.Fatal(err)
	}
}

func TestDetectUsesExactSupportedRevision(t *testing.T) {
	p := New()
	if !p.Detect(game.ROMInfo{Title: "POKEMON BLUE", SHA1: ROMSHA1}) {
		t.Fatal("known Pokémon Blue revision was not detected")
	}
	// Title alone must not accept it, and neither must Red's hash: the registry
	// relies on each Gen I image resolving to exactly one profile.
	if p.Detect(game.ROMInfo{Title: "POKEMON BLUE", SHA1: "different"}) {
		t.Fatal("title-only match must not accept an unknown revision")
	}
	if p.Detect(game.ROMInfo{Title: "POKEMON BLUE", SHA1: sym.ROMSHA1}) {
		t.Fatal("Blue profile must not accept the Red image")
	}
}

func TestIdentityIsBlue(t *testing.T) {
	p := New()
	if p.ID() != GameID || p.Revision() != Revision {
		t.Fatalf("identity = %s@%s, want %s@%s", p.ID(), p.Revision(), GameID, Revision)
	}
	if p.ID() == redprofile.GameID {
		t.Fatal("Blue profile reports the Red game id")
	}
}

// TestBlueSharesTheGen1Engine is the physical-sharing contract: Blue must
// expose the same symbol layout and engine behaviour as Red, because the
// runtime drives both with one adapter. A drift here means the adapter split
// is no longer a thin identity override.
func TestBlueSharesTheGen1Engine(t *testing.T) {
	blue, red := New(), redprofile.New()
	if fmt.Sprint(blue.Symbols()) != fmt.Sprint(red.Symbols()) {
		t.Fatal("Blue symbol table drifted from the shared Gen I layout")
	}
	if fmt.Sprint(blue.Features()) != fmt.Sprint(red.Features()) {
		t.Fatal("Blue feature set drifted from the shared Gen I engine")
	}
	bName, bOK := blue.ROMParser().MapName(0x26)
	rName, rOK := red.ROMParser().MapName(0x26)
	if bName != rName || bOK != rOK {
		t.Fatalf("Blue map-name lookup drifted from the shared Gen I engine: (%q,%v) vs (%q,%v)",
			bName, bOK, rName, rOK)
	}
}

// TestRealBlueImageIsDetected exercises the registry against the actual image,
// so a wrong hash or a duplicate that would make detection ambiguous surfaces
// here rather than during a run.
func TestRealBlueImageIsDetected(t *testing.T) {
	path := os.Getenv("POKEMON_BLUE_ROM")
	if path == "" {
		path = "roms/pokemon_blue.gb"
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("POKEMON_BLUE_ROM: %v", err)
	}
	info := game.InspectROM(rom)
	if !New().Detect(info) {
		t.Fatalf("supported Blue image not detected: sha1=%s title=%q", info.SHA1, info.Title)
	}
	if info.SHA1 != ROMSHA1 {
		t.Fatalf("Blue image sha1=%s, want %s", info.SHA1, ROMSHA1)
	}
}
