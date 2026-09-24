package profile

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/game"
	redsym "github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/yellow/sym"
)

type fakeMemory [0x10000]byte

func (m *fakeMemory) Peek8(addr uint16) byte { return m[int(addr)] }

func (m *fakeMemory) PeekInto(addr uint16, dst []byte) {
	copy(dst, m[int(addr):])
}

func TestProfileContract(t *testing.T) {
	if err := game.ValidateProfileContract(New()); err != nil {
		t.Fatal(err)
	}
}

func TestDetectUsesExactSupportedRevision(t *testing.T) {
	p := New()
	if !p.Detect(game.ROMInfo{Title: sym.ROMTitle, SHA1: sym.ROMSHA1}) {
		t.Fatal("known Pokémon Yellow revision was not detected")
	}
	if p.Detect(game.ROMInfo{Title: sym.ROMTitle, SHA1: "different"}) {
		t.Fatal("title-only match must not accept an unknown Yellow revision")
	}
	if p.Detect(game.ROMInfo{Title: sym.ROMTitle, SHA1: redsym.ROMSHA1}) {
		t.Fatal("Yellow profile must not accept the Red image")
	}
}

func TestIdentityIsYellow(t *testing.T) {
	p := New()
	if p.ID() != GameID || p.Revision() != Revision {
		t.Fatalf("identity = %s@%s, want %s@%s", p.ID(), p.Revision(), GameID, Revision)
	}
}

func TestPhase0DoesNotAdvertiseUnimplementedCapabilities(t *testing.T) {
	p := New()
	for _, feature := range []game.ProfileFeature{
		game.FeatureMapParsing,
		game.FeatureInventory,
		game.FeatureStoryProgress,
		game.FeatureBattles,
		game.FeatureFieldMoves,
		game.FeatureTrainerFlags,
		game.FeatureSemanticSpecies,
	} {
		if p.Features().Has(feature) {
			t.Errorf("phase-0 Yellow profile unexpectedly advertises %q", feature)
		}
	}
}

func TestDecodeObservationReadsYellowPlayerBaseline(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurMap] = 0
	mem[sym.XCoord] = 7
	mem[sym.YCoord] = 11
	mem[sym.SpritePlayerFacing] = 0x08
	mem[sym.IsInBattle] = 1

	obs, err := New().DecodeObservation(&mem, nil)
	if err != nil {
		t.Fatal(err)
	}
	if obs.NativeMapID != 0 || obs.MapName != "PALLET_TOWN" || obs.Location != "pallet town" {
		t.Fatalf("location = map:%d name:%q place:%q", obs.NativeMapID, obs.MapName, obs.Location)
	}
	if obs.X != 7 || obs.Y != 11 || obs.Facing != "left" {
		t.Fatalf("player = (%d,%d) facing %q", obs.X, obs.Y, obs.Facing)
	}
	if !obs.InBattle {
		t.Fatal("wIsInBattle was not decoded from the Yellow address")
	}
	if obs.Controllable {
		t.Fatal("phase-0 profile must not claim controllability before the control-state decoder exists")
	}
}

func TestRealYellowImageIsDetected(t *testing.T) {
	path := os.Getenv("POKEMON_YELLOW_ROM")
	if path == "" {
		path = "roms/pokemon_yellow.gb"
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("POKEMON_YELLOW_ROM: %v", err)
	}
	info := game.InspectROM(rom)
	if !New().Detect(info) {
		t.Fatalf("supported Yellow image not detected: sha1=%s title=%q", info.SHA1, info.Title)
	}
	if info.SHA1 != sym.ROMSHA1 {
		t.Fatalf("Yellow image sha1=%s, want %s", info.SHA1, sym.ROMSHA1)
	}
}
