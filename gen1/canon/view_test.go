package canon

import (
	"math/rand"
	"testing"
)

func testView() View {
	return View{
		Spans: []Span{
			{Canon: 0xC000, Native: 0xC000, Len: 0x10},
			{Canon: 0xC010, Native: 0xC00F, Len: 0x40}, // shifted one byte lower
			{Canon: 0xC060, Native: 0xC070, Len: 0x08},
			{Canon: 0xFF80, Native: 0xFF81, Len: 0x10},
		},
		Flags: []FlagArray{{Canon: 0xC020, Native: 0xC01F, CanonToNative: []int16{3, -1, 0, 1, 2, 9, -1, 4, 8}}},
		Lists: []IndexList{{Canon: 0xC030, Native: 0xC02F, Len: 9, Stride: 2, ValueOffset: 1, Terminator: 0xff, NativeToCanon: []int16{2, 3, 4, 0, 7, -1, -1, -1, 8, 5}}},
	}
}

func nativeReader(mem *[0x10000]byte) func(uint16, []byte) {
	return func(addr uint16, dst []byte) { copy(dst, mem[addr:]) }
}

func TestNativeTranslatesSpansAndDropsUnmapped(t *testing.T) {
	v := testView()
	cases := []struct {
		canon, native uint16
		ok            bool
	}{
		{0x8000, 0x8000, true}, // VRAM passes through
		{0xC005, 0xC005, true},
		{0xC010, 0xC00F, true},
		{0xC04F, 0xC04E, true},
		{0xC050, 0, false}, // canonical-only gap
		{0xC063, 0xC073, true},
		{0xE011, 0xC010, true}, // echo RAM mirrors work RAM
		{0xFF00, 0xFF00, true}, // I/O register
		{0xFF85, 0xFF86, true},
		{0xFFFF, 0xFFFF, true}, // IE register
	}
	for _, c := range cases {
		got, ok := v.Native(c.canon)
		if ok != c.ok || (ok && got != c.native) {
			t.Errorf("Native(%#04x) = %#04x,%v want %#04x,%v", c.canon, got, ok, c.native, c.ok)
		}
	}
}

func TestFlagAndListRenumbering(t *testing.T) {
	v := testView()
	var mem [0x10000]byte
	mem[0xC01F] = 1<<3 | 1<<4 // native bits 3 and 4
	mem[0xC020] = 1 << 1      // native bit 9
	// list: (obj 1, native flag 3) (obj 2, native flag 9) (obj 3, native 5) terminator
	copy(mem[0xC02F:], []byte{1, 3, 2, 9, 3, 5, 0xff, 7, 7})
	var got [9]byte
	v.ReadInto(nativeReader(&mem), 0xC020, got[:2])
	// canonical 0 <- native 3, canonical 5 <- native 9, canonical 7 <- native 4
	if got[0] != 1<<0|1<<5|1<<7 || got[1] != 0 {
		t.Fatalf("flags = %08b %08b", got[0], got[1])
	}
	v.ReadInto(nativeReader(&mem), 0xC030, got[:])
	want := [9]byte{1, 0, 2, 5, 3, 0xff, 0xff, 7, 7}
	if got != want {
		t.Fatalf("list = %v want %v", got, want)
	}
}

// The byte-by-byte path and the snapshot path must agree everywhere.
func TestSmallAndLargeReadsAgree(t *testing.T) {
	v := testView()
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 4; round++ {
		var mem [0x10000]byte
		rng.Read(mem[:])
		if round%2 == 0 {
			mem[0xC02F+4] = 0xff // terminate the list early
		}
		var full [0x10000]byte
		v.ReadInto(nativeReader(&mem), 0, full[:])
		for a := 0xBFF0; a < 0x10000; a++ {
			var one [1]byte
			v.ReadInto(nativeReader(&mem), uint16(a), one[:])
			if one[0] != full[a] {
				t.Fatalf("round %d addr %#04x: small %#02x large %#02x", round, a, one[0], full[a])
			}
		}
		var mid [48]byte
		v.ReadInto(nativeReader(&mem), 0xC01C, mid[:])
		for i, b := range mid {
			if b != full[0xC01C+i] {
				t.Fatalf("round %d range read %#04x differs", round, 0xC01C+i)
			}
		}
	}
}

func TestReadsOutsideRAMPassThrough(t *testing.T) {
	v := testView()
	var mem [0x10000]byte
	mem[0x4000], mem[0x9800] = 0x12, 0x34
	var b [1]byte
	calls := 0
	native := func(addr uint16, dst []byte) { calls++; copy(dst, mem[addr:]) }
	v.ReadInto(native, 0x9800, b[:])
	if b[0] != 0x34 || calls != 1 {
		t.Fatalf("VRAM read = %#02x after %d native calls", b[0], calls)
	}
}
