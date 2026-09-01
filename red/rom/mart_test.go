package rom

import (
	"os"
	"reflect"
	"testing"
)

// Item ids as the ROM numbers them (pokered/constants/item_constants.asm),
// spelled out here because this package predates the agent's name tables.
const (
	itemPokeBall   = 0x04
	itemPotion     = 0x14
	itemAntidote   = 0x0B
	itemBurnHeal   = 0x0C
	itemParlyzHeal = 0x0F
)

// MEASURED 2026-08-31: the Viridian Mart's shelf is POKe BALL, ANTIDOTE,
// PARLYZ HEAL, BURN HEAL — no POTION. The decoder must read that from the
// ROM, not reproduce it from memory: if this list ever disagrees with the
// bytes, the decoder is wrong.
func TestMartItemsViridian(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}

	got, err := MartItems(rom, 0x2A) // VIRIDIAN_MART
	if err != nil {
		t.Fatalf("MartItems(viridian mart) = %v, want nil", err)
	}
	want := []uint8{itemPokeBall, itemAntidote, itemParlyzHeal, itemBurnHeal}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Viridian Mart shelf = %v, want %v", got, want)
	}
}

// The Pewter Mart (0x38) DOES stock a POTION (pokered/data/items/marts.asm):
// the fix must keep offering it there. The shelf is read, not assumed.
func TestMartItemsPewterStocksPotion(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}

	got, err := MartItems(rom, 0x38) // PEWTER_MART
	if err != nil {
		t.Fatalf("MartItems(pewter mart) = %v, want nil", err)
	}
	for _, id := range got {
		if id == itemPotion {
			return
		}
	}
	t.Errorf("Pewter Mart shelf = %v, want it to include POTION (%#02x)", got, itemPotion)
}

// A map with no mart clerk has no shelf: the decoder says so, and callers
// must read that as "offer nothing".
func TestMartItemsNonMartMap(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}

	if _, err := MartItems(rom, 0x00); err == nil { // PALLET_TOWN
		t.Error("MartItems(pallet town) = nil error, want an error: no mart script on this map")
	}
}
