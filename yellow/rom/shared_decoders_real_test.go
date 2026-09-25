package rom

import (
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/gen1rom"
	redrom "github.com/maestroi/pokepilot/red/rom"
)

// TestRealYellowSharedGen1DecodersReadYellowTables proves the table binding:
// the shared Gen-I decoders, handed the Yellow cartridge, read Yellow's own
// tables and agree with the Yellow-owned parsers.
func TestRealYellowSharedGen1DecodersReadYellowTables(t *testing.T) {
	data := loadRealYellowROM(t)
	if _, ok := gen1rom.TableLayoutFor(data); !ok {
		t.Fatal("Yellow cartridge has no bound table layout")
	}
	for _, id := range MapIDs() {
		want, err := ParseMap(data, id)
		if err != nil {
			t.Fatalf("yellow ParseMap(%02x): %v", id, err)
		}
		got, err := redrom.ParseMap(data, id)
		if err != nil {
			t.Fatalf("shared ParseMap(%02x %s): %v", id, MapName(id), err)
		}
		if !reflect.DeepEqual(gen1rom.MapHeader(got), gen1rom.MapHeader(want)) {
			t.Fatalf("shared ParseMap(%02x %s) diverges from Yellow's", id, MapName(id))
		}
	}

	wild, err := WildEncounters(data)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := redrom.WildEncounters(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(shared) != len(wild) {
		t.Fatalf("shared wild slots = %d, Yellow's = %d", len(shared), len(wild))
	}
	for i := range wild {
		if shared[i].MapID != wild[i].MapID || shared[i].Species != wild[i].Species || shared[i].Level != wild[i].Level {
			t.Fatalf("wild slot %d: shared %+v, Yellow %+v", i, shared[i], wild[i])
		}
	}

	for id := uint8(1); id <= 0xA5; id++ {
		want, err := LookupMove(data, id)
		if err != nil {
			t.Fatal(err)
		}
		got, err := redrom.LookupMove(data, id)
		if err != nil || got.Power != want.Power || got.Type != want.Type || got.PP != want.PP {
			t.Fatalf("move %d: shared %+v,%v Yellow %+v", id, got, err, want)
		}
	}
	for species := uint8(1); species < 0xBE; species++ {
		want, wantErr := InternalSpeciesDexNumber(data, species)
		got, gotErr := redrom.InternalSpeciesDexNumber(data, species)
		if (wantErr == nil) != (gotErr == nil) || got != want {
			t.Fatalf("species %#02x dex: shared %d,%v Yellow %d,%v", species, got, gotErr, want, wantErr)
		}
	}
	for _, item := range []uint8{redrom.HM01Item, redrom.HM05Item, redrom.TM01Item, redrom.TM50Item} {
		if _, err := redrom.LookupTMHM(data, item); err != nil {
			t.Fatalf("LookupTMHM(%#02x): %v", item, err)
		}
	}
	if _, err := redrom.Evolutions(data); err != nil {
		t.Fatalf("Evolutions: %v", err)
	}
	if _, err := redrom.NPCTrades(data); err != nil {
		t.Fatalf("NPCTrades: %v", err)
	}
	if got, err := redrom.TypeEffectiveness(data, 0x15, 0x14, 0x14); err != nil || got != 20 {
		t.Fatalf("WATER vs FIRE effectiveness = %d,%v, want 20", got, err)
	}
	fishing, err := redrom.FishingEncounters(data)
	if err != nil {
		t.Fatalf("FishingEncounters: %v", err)
	}
	super := 0
	for _, f := range fishing {
		if f.Rod == redrom.RodSuper {
			super++
		}
	}
	if super == 0 {
		t.Fatal("Yellow SuperRodFishingSlots decoded no encounters")
	}
}
