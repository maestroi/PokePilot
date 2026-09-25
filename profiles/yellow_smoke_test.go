package profiles_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/profiles"
	"github.com/maestroi/pokepilot/red/state"
	redsym "github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowsym "github.com/maestroi/pokepilot/yellow/sym"
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

// TestYellowCanonicalViewDecodesWithSharedGen1State boots the real Yellow
// cartridge, binds Yellow's generated canonical Gen-I view, and decodes it
// with the unmodified red/state decoders. Each canonical fact must equal the
// same fact read from Yellow's own native addresses.
func TestYellowCanonicalViewDecodesWithSharedGen1State(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed Yellow canonical view")
	}
	path := os.Getenv("POKEMON_YELLOW_ROM")
	if path == "" {
		path = "../roms/pokemon_yellow.gbc"
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("POKEMON_YELLOW_ROM: %v", err)
	}
	m, err := emu.OpenCGB(path)
	if err != nil {
		t.Fatalf("OpenCGB Yellow: %v", err)
	}
	defer m.Close()
	if _, err := skill.BootToOverworld(m); err != nil {
		t.Fatalf("BootToOverworld Yellow: %v", err)
	}

	native := func(addr uint16) byte { return m.Peek8Native(addr) }
	m.BindMemoryView(&yellowsym.Canonical)

	var mem state.Mem
	state.Snapshot(m, &mem)
	gs := state.Decode(&mem)
	if gs.Player.MapID != native(yellowsym.CurMap) || gs.Player.MapID != 0x26 {
		t.Fatalf("canonical map %#02x, Yellow native %#02x, want 0x26", gs.Player.MapID, native(yellowsym.CurMap))
	}
	if gs.Player.X != native(yellowsym.XCoord) || gs.Player.Y != native(yellowsym.YCoord) {
		t.Fatalf("canonical position (%d,%d), Yellow native (%d,%d)", gs.Player.X, gs.Player.Y, native(yellowsym.XCoord), native(yellowsym.YCoord))
	}
	if gs.World.Width != native(yellowsym.CurMapWidth) || gs.World.Height != native(yellowsym.CurMapHeight) || gs.World.Width == 0 {
		t.Fatalf("canonical map size %dx%d, Yellow native %dx%d", gs.World.Width, gs.World.Height, native(yellowsym.CurMapWidth), native(yellowsym.CurMapHeight))
	}
	if !state.Controllable(&mem) {
		t.Fatal("shared Gen-I Controllable rejects Yellow's controllable bedroom through the canonical view")
	}
	if len(gs.Party.Mons) != int(native(yellowsym.PartyCount)) {
		t.Fatalf("canonical party %d mons, Yellow native count %d", len(gs.Party.Mons), native(yellowsym.PartyCount))
	}
	if got := m.Peek8(redsym.CurMap); got != gs.Player.MapID {
		t.Fatalf("single-byte canonical read %#02x disagrees with snapshot %#02x", got, gs.Player.MapID)
	}
	var tiles [yellowsym.TileMapLen]byte
	m.PeekIntoNative(yellowsym.TileMap, tiles[:])
	if !bytes.Equal(tiles[:], mem.Slice(redsym.TileMap, yellowsym.TileMapLen)) {
		t.Fatal("canonical tile map differs from Yellow's native tile map")
	}

	// Unbound, the same Red address reads Yellow's byte after wCurMap: the
	// view, not luck, is what made the shared decoder right.
	m.BindMemoryView(nil)
	if got := m.Peek8(redsym.CurMap); got == gs.Player.MapID {
		t.Fatalf("unbound Red wCurMap read %#02x also matches; this test no longer proves the view", got)
	}
	if got := m.Peek8(yellowsym.CurMap); got != 0x26 {
		t.Fatalf("after unbinding, native wCurMap reads %#02x", got)
	}
}
