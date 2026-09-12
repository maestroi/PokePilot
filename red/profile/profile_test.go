package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/sym"
)

type fakeMemory [0x10000]byte

func (m *fakeMemory) Peek8(addr uint16) byte { return m[addr] }
func (m *fakeMemory) PeekInto(addr uint16, dst []byte) {
	copy(dst, m[int(addr):int(addr)+len(dst)])
}

func TestProfileContract(t *testing.T) {
	if err := game.ValidateProfileContract(New()); err != nil {
		t.Fatal(err)
	}
}

func TestDetectUsesExactSupportedRevision(t *testing.T) {
	p := New()
	if !p.Detect(game.ROMInfo{Title: sym.ROMTitle, SHA1: sym.ROMSHA1}) {
		t.Fatal("known Pokémon Red revision was not detected")
	}
	if p.Detect(game.ROMInfo{Title: sym.ROMTitle, SHA1: "different"}) {
		t.Fatal("title-only match must not accept an unknown revision")
	}
}

func TestDecodeObservation(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurMap] = 0x00
	mem[sym.XCoord] = 7
	mem[sym.YCoord] = 9
	mem[sym.SpritePlayerFacing] = 12
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonSpecies] = 0xB0 // Charmander
	mem[sym.PartyMon1+sym.MonLevel] = 8
	mem[sym.PartyMon1+sym.MonHP] = 0
	mem[sym.PartyMon1+sym.MonHP+1] = 21
	mem[sym.PartyMon1+sym.MonMaxHP] = 0
	mem[sym.PartyMon1+sym.MonMaxHP+1] = 24
	mem[sym.ObtainedBadges] = 1
	mem[sym.LastBlackoutMap] = 0x01

	obs, err := New().DecodeObservation(&mem, nil)
	if err != nil {
		t.Fatal(err)
	}
	if obs.NativeMapID != 0 || obs.X != 7 || obs.Y != 9 || obs.Facing != "right" {
		t.Fatalf("position = %#v", obs)
	}
	if obs.Location != "pallet town" || obs.RespawnPlace != "viridian city" {
		t.Fatalf("locations = current %q respawn %q", obs.Location, obs.RespawnPlace)
	}
	if len(obs.Party) != 1 || obs.Party[0].Species != "charmander" || obs.Party[0].Level != 8 || obs.Party[0].HP != 21 || obs.Party[0].MaxHP != 24 {
		t.Fatalf("party = %#v", obs.Party)
	}
	if len(obs.Badges) != 1 || obs.Badges[0] != "Boulder" {
		t.Fatalf("badges = %#v", obs.Badges)
	}
}

func TestSemanticSymbolsMatchRedLayout(t *testing.T) {
	symbols := New().Symbols()
	for name, want := range map[string]uint16{
		"player.map":  sym.CurMap,
		"player.x":    sym.XCoord,
		"player.y":    sym.YCoord,
		"party.count": sym.PartyCount,
		"battle.mode": sym.IsInBattle,
		"badges":      sym.ObtainedBadges,
		"money":       sym.PlayerMoney,
	} {
		got, ok := symbols.Lookup(name)
		if !ok || got.Address != want {
			t.Fatalf("symbol %q = %#v, want address %#04x", name, got, want)
		}
	}
}
