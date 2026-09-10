package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeTrainerFlagRef(t *testing.T) {
	romData := make([]byte, 0x8000)
	const (
		bank = uint8(1)
		ptr  = uint16(0x4567)
	)
	off := 0x4567
	romData[off] = 10 // CURRENT_TRAINER_BIT: byte carry 1, bit 2
	base := sym.EventFlags + 20
	romData[off+2] = byte(base)
	romData[off+3] = byte(base >> 8)

	got, err := decodeTrainerFlagRef(romData, bank, ptr)
	if err != nil {
		t.Fatalf("decodeTrainerFlagRef: %v", err)
	}
	wantAddr := base + 1 // FlagAction advances HL by CURRENT_TRAINER_BIT/8.
	if got.addr != wantAddr {
		t.Fatalf("addr = %#04x, want %#04x", got.addr, wantAddr)
	}
	if got.mask != 1<<2 {
		t.Fatalf("mask = %#02x, want %#02x", got.mask, uint8(1<<2))
	}
}

func TestDecodeTrainerFlagRefRejectsNonEventPointer(t *testing.T) {
	romData := make([]byte, 0x8000)
	const ptr = uint16(0x4100)
	romData[0x4100] = 3
	romData[0x4102] = 0x00
	romData[0x4103] = 0xC0

	if _, err := decodeTrainerFlagRef(romData, 1, ptr); err == nil {
		t.Fatal("decodeTrainerFlagRef accepted a pointer outside wEventFlags")
	}
}

func TestDecodeTrainerFlagRefRejectsBadBankedPointer(t *testing.T) {
	romData := make([]byte, 0x8000)
	if _, err := decodeTrainerFlagRef(romData, 1, 0x3000); err == nil {
		t.Fatal("decodeTrainerFlagRef accepted a fixed-bank pointer with nonzero bank")
	}
}

func TestOrdinaryTrainerClass(t *testing.T) {
	const opponentOffset = 200
	cases := []struct {
		name string
		id   uint8
		want bool
	}{
		{"youngster", opponentOffset + 0x01, true},
		{"rocket", opponentOffset + 0x1e, true},
		{"scientist", opponentOffset + 0x1c, true},
		{"channeler", opponentOffset + 0x2d, true},
		{"rival", opponentOffset + 0x19, false},
		{"giovanni", opponentOffset + 0x1d, false},
		{"brock", opponentOffset + 0x22, false},
		{"lorelei", opponentOffset + 0x2c, false},
		{"lance", opponentOffset + 0x2f, false},
		{"special pokemon", 0x83, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ordinaryTrainerClass(tc.id); got != tc.want {
				t.Fatalf("ordinaryTrainerClass(%#02x) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}
