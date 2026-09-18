package rom

import "testing"

func TestEvolutionsReadsLevelItemAndTrade(t *testing.T) {
	romData := make([]byte, 0x3C000)
	base, err := bankedOffset(evosMovesBank, evosMovesAddr)
	if err != nil {
		t.Fatal(err)
	}
	// Species 0x24 Pidgey, 0xB9 Oddish, 0x26 Kadabra.
	putEvo := func(species uint8, addr uint16, record []byte) {
		pOff := base + int(species-1)*2
		romData[pOff] = byte(addr)
		romData[pOff+1] = byte(addr >> 8)
		rec, err := bankedOffset(evosMovesBank, addr)
		if err != nil {
			t.Fatal(err)
		}
		copy(romData[rec:], record)
	}
	putEvo(0x24, 0x7220, []byte{EvoLevel, 18, 0x96, 0})     // Pidgeotto
	putEvo(0xB9, 0x7230, []byte{EvoLevel, 21, 0xBA, 0})     // Gloom
	putEvo(0xBA, 0x7240, []byte{EvoItem, 0x2F, 1, 0xBB, 0}) // Leaf Stone -> Vileplume
	putEvo(0x26, 0x7250, []byte{EvoTrade, 1, 0x95, 0})      // Alakazam

	got, err := Evolutions(romData)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("evolutions = %+v, want 4", got)
	}
	if !hasEvo(got, 0x24, 0x96, EvoLevel) {
		t.Fatalf("missing Pidgey level evolution: %+v", got)
	}
	if !hasEvo(got, 0xBA, 0xBB, EvoItem) {
		t.Fatalf("missing Gloom stone evolution: %+v", got)
	}
	if !hasEvo(got, 0x26, 0x95, EvoTrade) {
		t.Fatalf("missing Kadabra trade evolution: %+v", got)
	}
}

func TestEvolutionsFollowsPatchedPointer(t *testing.T) {
	romData := make([]byte, 0x3C000)
	base, err := bankedOffset(evosMovesBank, evosMovesAddr)
	if err != nil {
		t.Fatal(err)
	}
	addr := uint16(0x7200)
	romData[base] = byte(addr)
	romData[base+1] = byte(addr >> 8)
	rec, err := bankedOffset(evosMovesBank, addr)
	if err != nil {
		t.Fatal(err)
	}
	copy(romData[rec:], []byte{EvoLevel, 16, 0xB3, 0}) // species 1 -> Wartortle

	got, err := Evolutions(romData)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].From != 1 || got[0].To != 0xB3 {
		t.Fatalf("patched evolution = %+v, want species 1 -> Wartortle", got)
	}
}

func TestEvolutionsFromROM(t *testing.T) {
	romData := loadROM(t)
	got, err := Evolutions(romData)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEvo(got, 0xB1, 0xB3, EvoLevel) { // Squirtle -> Wartortle
		t.Fatalf("missing Squirtle level evolution: %+v", got)
	}
	if !hasEvo(got, 0xBA, 0xBB, EvoItem) { // Gloom -> Vileplume
		t.Fatalf("missing Gloom stone evolution")
	}
	if !hasEvo(got, 0x26, 0x95, EvoTrade) { // Kadabra -> Alakazam
		t.Fatalf("missing Kadabra trade evolution")
	}
}

func hasEvo(evos []Evolution, from, to, method uint8) bool {
	for _, e := range evos {
		if e.From == from && e.To == to && e.Method == method {
			return true
		}
	}
	return false
}


func TestLevelUpMovesReadsMoveHalfAfterEvolutions(t *testing.T) {
	romData := make([]byte, 0x3C000)
	base, err := bankedOffset(evosMovesBank, evosMovesAddr)
	if err != nil {
		t.Fatal(err)
	}
	const species uint8 = 0x24
	const addr uint16 = 0x7280
	pOff := base + int(species-1)*2
	romData[pOff] = byte(addr)
	romData[pOff+1] = byte(addr >> 8)
	rec, err := bankedOffset(evosMovesBank, addr)
	if err != nil {
		t.Fatal(err)
	}
	// One level evolution, evolution terminator, two exact-level moves, move terminator.
	copy(romData[rec:], []byte{EvoLevel, 18, 0x96, 0, 12, 0x10, 19, 0x11, 0})

	got, err := LevelUpMoves(romData, species)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("level-up moves = %+v, want 2", got)
	}
	if got[0] != (LevelUpMove{Species: species, Level: 12, Move: 0x10}) ||
		got[1] != (LevelUpMove{Species: species, Level: 19, Move: 0x11}) {
		t.Fatalf("level-up moves = %+v", got)
	}
}
