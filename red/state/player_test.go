package state

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

// newMem builds a zeroed Mem by hand — no emulator, no ROM.
func newMem() *Mem {
	var m Mem
	return &m
}

func TestDecodePlayer(t *testing.T) {
	m := newMem()
	m[sym.CurMap] = 0x26
	m[sym.XCoord] = 3
	m[sym.YCoord] = 6
	m[sym.PlayerDirection] = 4
	m[sym.WalkCounter] = 0

	got := DecodePlayer(m)
	want := PlayerState{MapID: 0x26, X: 3, Y: 6, Facing: FacingUp, Walking: false}
	if got != want {
		t.Fatalf("DecodePlayer() = %+v, want %+v", got, want)
	}
}

func TestDecodePlayerWalking(t *testing.T) {
	m := newMem()
	m[sym.WalkCounter] = 3

	if got := DecodePlayer(m); !got.Walking {
		t.Fatalf("Walking = false, want true (WalkCounter=%d)", m[sym.WalkCounter])
	}
}

func TestDecodeWorld(t *testing.T) {
	m := newMem()
	m[sym.CurMap] = 0x26
	m[sym.CurMapWidth] = 10
	m[sym.CurMapHeight] = 12
	m[sym.CurMapTileset] = 1

	got := DecodeWorld(m)
	want := WorldState{MapID: 0x26, Width: 10, Height: 12, Tileset: 1}
	if got != want {
		t.Fatalf("DecodeWorld() = %+v, want %+v", got, want)
	}
}

func TestU16Endianness(t *testing.T) {
	m := newMem()
	const addr uint16 = 0x1234
	m[addr] = 0x01
	m[addr+1] = 0x02

	if got := m.U16LE(addr); got != 0x0201 {
		t.Errorf("U16LE = %#04x, want 0x0201", got)
	}
	if got := m.U16BE(addr); got != 0x0102 {
		t.Errorf("U16BE = %#04x, want 0x0102", got)
	}
}

func TestFacingString(t *testing.T) {
	known := []struct {
		f    Facing
		want string
	}{
		{FacingDown, "down"},
		{FacingUp, "up"},
		{FacingLeft, "left"},
		{FacingRight, "right"},
	}
	for _, tc := range known {
		if got := tc.f.String(); got != tc.want {
			t.Errorf("Facing(%d).String() = %q, want %q", uint8(tc.f), got, tc.want)
		}
	}

	unknown := Facing(7)
	if got := unknown.String(); !strings.HasPrefix(got, "unknown(") {
		t.Errorf("Facing(7).String() = %q, want prefix %q", got, "unknown(")
	}
}
