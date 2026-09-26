package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

type fakeMemory [0x10000]byte

func (m *fakeMemory) Peek8(addr uint16) byte { return m[addr] }
func (m *fakeMemory) PeekInto(addr uint16, dst []byte) {
	copy(dst, m[int(addr):int(addr)+len(dst)])
}

func TestGoldSilverProfilesSatisfyContract(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		if err := game.ValidateProfileContract(p); err != nil {
			t.Fatalf("%s: %v", p.ID(), err)
		}
		if !p.Features().Has(game.FeatureBankedMemory) {
			t.Fatalf("%s: banked-memory feature not advertised", p.ID())
		}
		for _, name := range []string{
			"player.map", "player.x", "player.y", "player.direction",
			"party.count", "party.members", "battle.mode", "badges", "bag", "money",
		} {
			s, ok := p.Symbols().Lookup(name)
			if !ok {
				t.Fatalf("%s: missing %s", p.ID(), name)
			}
			if s.Bank != sym.WRAMBank {
				t.Fatalf("%s: %s bank=%d, want %d", p.ID(), name, s.Bank, sym.WRAMBank)
			}
		}
	}
}

func TestGoldSilverExactFingerprintsOnly(t *testing.T) {
	tests := []struct {
		name string
		p    *Profile
		sha1 string
	}{
		{"gold", NewGold(), sym.GoldSHA1},
		{"silver", NewSilver(), sym.SilverSHA1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := game.ROMInfo{Title: "same title", SHA1: tc.sha1, Size: sym.ROMSize}
			if !tc.p.Detect(info) {
				t.Fatal("known retail fingerprint rejected")
			}
			info.SHA1 = "0000000000000000000000000000000000000000"
			if tc.p.Detect(info) {
				t.Fatal("same-title different revision accepted")
			}
			info.SHA1 = tc.sha1
			info.Size--
			if tc.p.Detect(info) {
				t.Fatal("wrong-sized image accepted")
			}
		})
	}
}

func TestDecodeObservationSemanticFixture(t *testing.T) {
	var mem fakeMemory
	mem[sym.MapGroup] = 24
	mem[sym.MapNumber] = 4 // NEW_BARK_TOWN
	mem[sym.XCoord] = 7
	mem[sym.YCoord] = 8
	mem[sym.PlayerDirection] = 0x04 // up
	mem[sym.BattleMode] = 1
	mem[sym.PartyCount] = 2
	mem[sym.Money+0] = 0x01
	mem[sym.Money+1] = 0x23
	mem[sym.Money+2] = 0x45
	mem[sym.JohtoBadges] = 0x83 // Zephyr, Hive, Rising
	mem[sym.KantoBadges] = 0x01 // Boulder

	first := sym.PartyMon1
	mem[first+0x00] = 0x98 // Chikorita
	mem[first+0x01] = 0xad // Berry
	mem[first+0x08] = 0x00
	mem[first+0x09] = 0x00
	mem[first+0x0a] = 0x7d
	mem[first+0x1f] = 5
	mem[first+0x22] = 0
	mem[first+0x23] = 20
	mem[first+0x24] = 0
	mem[first+0x25] = 20

	second := first + sym.PartyMonSize
	mem[second+0x00] = 0xfd // Egg
	mem[second+0x1f] = 5

	mem[sym.NumItems] = 2
	mem[sym.Items+0] = 0x12 // Potion
	mem[sym.Items+1] = 3
	mem[sym.Items+2] = 0xad // Berry
	mem[sym.Items+3] = 2
	mem[sym.NumKeyItems] = 1
	mem[sym.KeyItems] = 0x07 // Bicycle
	mem[sym.NumBalls] = 1
	mem[sym.Balls+0] = 0x05 // Poke Ball
	mem[sym.Balls+1] = 5
	mem[sym.TMsHMs] = 1 // TM01

	obs, err := NewGold().DecodeObservation(&mem, nil)
	if err != nil {
		t.Fatal(err)
	}
	if obs.NativeMapID != 0x1804 || obs.Location != "new-bark-town" || obs.MapName != "NEW_BARK_TOWN" {
		t.Fatalf("map = %#x %q %q", obs.NativeMapID, obs.Location, obs.MapName)
	}
	if obs.X != 7 || obs.Y != 8 || obs.Facing != "up" || !obs.InBattle {
		t.Fatalf("player = (%d,%d) facing=%q battle=%v", obs.X, obs.Y, obs.Facing, obs.InBattle)
	}
	if obs.Money != 12345 {
		t.Fatalf("money=%d, want 12345", obs.Money)
	}
	if len(obs.Party) != 2 {
		t.Fatalf("party=%+v", obs.Party)
	}
	if got := obs.Party[0]; got.Species != "chikorita" || got.HeldItem != "berry" || got.IsEgg || got.Level != 5 || got.Experience != 125 || got.HP != 20 || got.MaxHP != 20 {
		t.Fatalf("lead=%+v", got)
	}
	if got := obs.Party[1]; got.Species != "egg" || !got.IsEgg {
		t.Fatalf("egg=%+v", got)
	}
	if len(obs.Badges) != 4 || obs.Badges[0] != "zephyr" || obs.Badges[1] != "hive" || obs.Badges[2] != "rising" || obs.Badges[3] != "boulder" {
		t.Fatalf("badges=%v", obs.Badges)
	}
	if fact, ok := obs.Story.Lookup(ProgressJohtoBadges); !ok || fact.Value != 3 || fact.Complete {
		t.Fatalf("Johto progress=%+v ok=%v", fact, ok)
	}
	if fact, ok := obs.Story.Lookup(ProgressKantoBadges); !ok || fact.Value != 1 || fact.Complete {
		t.Fatalf("Kanto progress=%+v ok=%v", fact, ok)
	}

	wantBag := map[game.ItemID]int{
		"potion": 3, "berry": 2, "bicycle": 1, "poke-ball": 5, "tm-dynamicpunch": 1,
	}
	for _, item := range obs.Bag {
		if want, ok := wantBag[item.ID]; ok {
			if item.Quantity != want {
				t.Fatalf("%s quantity=%d, want %d", item.ID, item.Quantity, want)
			}
			delete(wantBag, item.ID)
		}
	}
	if len(wantBag) != 0 {
		t.Fatalf("missing bag items: %v; got %+v", wantBag, obs.Bag)
	}
}

func TestDecodeObservationRejectsMalformedMoney(t *testing.T) {
	var mem fakeMemory
	mem[sym.Money] = 0xfa
	if _, err := NewSilver().DecodeObservation(&mem, nil); err == nil {
		t.Fatal("invalid packed BCD money accepted")
	}
}
