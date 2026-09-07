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
	romData[off] = 10 // CURRENT_TRAINER_BIT; fought bit is 10 % 8 = 2
	addr := sym.EventFlags + 20
	romData[off+2] = byte(addr)
	romData[off+3] = byte(addr >> 8)

	got, err := decodeTrainerFlagRef(romData, bank, ptr)
	if err != nil {
		t.Fatalf("decodeTrainerFlagRef: %v", err)
	}
	if got.addr != addr {
		t.Fatalf("addr = %#04x, want %#04x", got.addr, addr)
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
