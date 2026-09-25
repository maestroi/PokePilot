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

func TestPhase4AdvertisesOnlyImplementedCapabilities(t *testing.T) {
	p := New()
	for _, feature := range []game.ProfileFeature{
		game.FeatureMapParsing,
		game.FeatureInventory,
		game.FeatureStoryProgress,
		game.FeatureSemanticSpecies,
	} {
		if !p.Features().Has(feature) {
			t.Errorf("Yellow profile missing implemented capability %q", feature)
		}
	}
	for _, feature := range []game.ProfileFeature{
		game.FeatureBattles,
		game.FeatureFieldMoves,
		game.FeatureTrainerFlags,
	} {
		if p.Features().Has(feature) {
			t.Errorf("Yellow profile unexpectedly advertises unimplemented %q", feature)
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

func TestDecodeObservationProjectsYellowPartyMoneyAndBadges(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurMap] = 0x26
	mem[sym.CurMapHeight] = 4
	mem[sym.CurMapWidth] = 4
	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+0x00] = 0x54 // Pikachu
	mem[base+0x01], mem[base+0x02] = 0x00, 0x23
	mem[base+0x04] = 1 << 6 // paralyzed
	mem[base+0x0e], mem[base+0x0f], mem[base+0x10] = 0x01, 0x02, 0x03
	mem[base+0x21] = 12
	mem[base+0x22], mem[base+0x23] = 0x00, 0x30
	mem[sym.PlayerMoney], mem[sym.PlayerMoney+1], mem[sym.PlayerMoney+2] = 0x12, 0x34, 0x56
	mem[sym.ObtainedBadges] = 0b00000101
	mem[sym.NumBagItems] = 2
	mem[sym.BagItems], mem[sym.BagItems+1] = 0x04, 7
	mem[sym.BagItems+2], mem[sym.BagItems+3] = 0x14, 3

	obs, err := New().DecodeObservation(&mem, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !obs.Controllable {
		t.Fatal("Yellow bedroom observation should be controllable")
	}
	if len(obs.Party) != 1 || obs.Party[0].Species != "pikachu" || obs.Party[0].Level != 12 {
		t.Fatalf("party = %+v", obs.Party)
	}
	if obs.Party[0].Experience != 0x010203 || obs.Party[0].HP != 0x23 || obs.Party[0].MaxHP != 0x30 || obs.Party[0].Status != "paralyzed" {
		t.Fatalf("Pikachu = %+v", obs.Party[0])
	}
	if obs.Money != 123456 {
		t.Fatalf("money = %d, want 123456", obs.Money)
	}
	if len(obs.Badges) != 2 || obs.Badges[0] != "Boulder" || obs.Badges[1] != "Thunder" {
		t.Fatalf("badges = %v", obs.Badges)
	}
	if obs.BagCapacity != 20 || len(obs.Bag) != 2 || obs.Bag[0].Quantity != 7 || obs.Bag[1].Quantity != 3 {
		t.Fatalf("bag = %+v capacity=%d", obs.Bag, obs.BagCapacity)
	}
	if obs.PokedexTotal != 151 {
		t.Fatalf("Pokédex total = %d, want 151", obs.PokedexTotal)
	}
}

func TestYellowROMParserCoversFullMapNamespaceAndSpecies(t *testing.T) {
	p := New().ROMParser()
	if got, ok := p.MapName(0xF8); !ok || got != "SUMMER_BEACH_HOUSE" {
		t.Fatalf("MapName(F8) = %q,%v", got, ok)
	}
	if got, ok := p.Species(0x54); !ok || got != "pikachu" {
		t.Fatalf("Species(54) = %q,%v", got, ok)
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

func TestYellowVictoryRoadClearedUsesRoute23ExitPocket(t *testing.T) {
	for _, tc := range []struct {
		x, y uint8
		want bool
	}{{18, 30, true}, {14, 32, true}, {4, 31, false}, {4, 32, false}, {14, 40, false}} {
		var mem fakeMemory
		mem[sym.XCoord], mem[sym.YCoord] = tc.x, tc.y
		if got := yellowVictoryRoadCleared(&mem, route23Map, true, false, false, false); got != tc.want {
			t.Errorf("Route 23 (%d,%d) cleared=%v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}
