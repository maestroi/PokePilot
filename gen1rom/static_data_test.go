package gen1rom

import "testing"

func TestWildEncountersUsesLayout(t *testing.T) {
	rom := make([]byte, 0x9000)
	// bank 2 pointer table at 0x4000 -> offset 0x8000.
	rom[0x8000], rom[0x8001] = 0x20, 0x40
	rec := 0x8020
	rom[rec] = 10
	for i := 0; i < wildSlots; i++ {
		rom[rec+1+i*2] = uint8(5 + i)
		rom[rec+2+i*2] = 0x24 // Pidgey
	}
	water := rec + 1 + 2*wildSlots
	rom[water] = 0

	got, err := WildEncounters(rom, WildLayout{PointerBank: 2, PointerAddr: 0x4000, MapCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 || got[0].MapID != 0 || got[0].Habitat != HabitatGrass || got[0].Species != 0x24 || got[9].Level != 14 {
		t.Fatalf("encounters = %+v", got)
	}
}

func TestMoveAndStringListDecoders(t *testing.T) {
	rom := make([]byte, 256)
	copy(rom[20:], []byte{1, 2, 40, 3, 255, 35, 2, 4, 0, 5, 200, 10})
	move, err := LookupMove(rom, 2, MoveLayout{Offset: 20, EntryLen: 6})
	if err != nil {
		t.Fatal(err)
	}
	if move.ID != 2 || move.Effect != 4 || move.PP != 10 {
		t.Fatalf("move = %+v", move)
	}
	copy(rom[100:], []byte{0x80, 0x50, 0x81, 0x82, 0x50})
	name, err := ListName(rom, 2, StringListLayout{Offset: 100, Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	if name != "BC" {
		t.Fatalf("list name = %q, want BC", name)
	}
}

func TestSpeciesAndTrainerDecoders(t *testing.T) {
	rom := make([]byte, 0x9000)
	layout := SpeciesLayout{
		NamesOffset: 100, NameLength: 10, InternalCount: 2,
		PokedexOrderOffset: 200, PokedexOrderLen: 2,
		BaseStatsOffset: 300, BaseStatsEntryLen: 28,
	}
	copy(rom[100:110], []byte{0x8f, 0x88, 0x8a, 0x80, 0x82, 0x87, 0x94, 0x50}) // PIKACHU@
	rom[200] = 25
	stats := 300 + 24*28
	rom[stats] = 25
	copy(rom[stats+1:], []byte{35, 55, 30, 90, 50, 23, 23, 190})
	copy(rom[stats+15:], []byte{84, 45, 0, 0})
	rom[stats+19] = 0

	name, err := SpeciesName(rom, 1, layout)
	if err != nil || name != "PIKACHU" {
		t.Fatalf("SpeciesName = %q, %v", name, err)
	}
	base, err := LookupSpeciesBaseStats(rom, 1, layout)
	if err != nil {
		t.Fatal(err)
	}
	if base.Dex != 25 || base.HP != 35 || base.Moves[0] != 84 {
		t.Fatalf("base stats = %+v", base)
	}

	// Trainer pointer table in bank 2, class 1 -> 0x4040.
	rom[0x8000], rom[0x8001] = 0x40, 0x40
	copy(rom[0x8040:], []byte{
		5, 0x54, 0,
		0xff, 8, 0x24, 9, 0xa5, 0,
	})
	party, err := TrainerParty(rom, 1, 2, TrainerLayout{PointerBank: 2, PointerAddr: 0x4000, TrainerCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(party) != 2 || party[0].Level != 8 || party[1].Species != 0xa5 {
		t.Fatalf("trainer party = %+v", party)
	}
}
