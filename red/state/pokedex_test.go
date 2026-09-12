package state

import (
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodePokedexDistinguishesSeenFromOwned(t *testing.T) {
	var mem Mem

	// #1 Bulbasaur is seen but not owned.
	mem[sym.PokedexSeen] |= 1 << 0
	// #4 Charmander is owned but not seen in this synthetic snapshot. Keeping
	// the arrays independent proves the decoder does not infer one from the other.
	mem[sym.PokedexOwned] |= 1 << 3
	// #25 Pikachu is both.
	mem[sym.PokedexSeen+3] |= 1 << 0
	mem[sym.PokedexOwned+3] |= 1 << 0

	got := DecodePokedex(&mem)
	if want := []uint8{1, 25}; !reflect.DeepEqual(got.Seen, want) {
		t.Fatalf("seen = %v, want %v", got.Seen, want)
	}
	if want := []uint8{4, 25}; !reflect.DeepEqual(got.Owned, want) {
		t.Fatalf("owned = %v, want %v", got.Owned, want)
	}
}

func TestDecodePokedexIncludes151AndIgnoresPadding(t *testing.T) {
	var mem Mem
	last := sym.PokedexBytes - 1

	// #151 Mew is bit 6 of the 19th byte. Bit 7 is unused padding and must not
	// produce a phantom #152 entry.
	mem[sym.PokedexOwned+uint16(last)] = 1<<6 | 1<<7

	got := DecodePokedex(&mem)
	if want := []uint8{151}; !reflect.DeepEqual(got.Owned, want) {
		t.Fatalf("owned = %v, want %v", got.Owned, want)
	}
}

func TestDecodePokedexOrderingIsDeterministic(t *testing.T) {
	var mem Mem
	for _, dex := range []int{150, 2, 87, 1} {
		bit := dex - 1
		mem[sym.PokedexOwned+uint16(bit/8)] |= 1 << uint(bit%8)
	}

	got := DecodePokedex(&mem)
	if want := []uint8{1, 2, 87, 150}; !reflect.DeepEqual(got.Owned, want) {
		t.Fatalf("owned = %v, want deterministic dex order %v", got.Owned, want)
	}
}
