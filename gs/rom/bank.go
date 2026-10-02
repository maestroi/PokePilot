package rom

import "fmt"

const (
	romBankSize   = 0x4000
	romWindowAddr = 0x4000
	romWindowEnd  = 0x8000
)

func bankedOffset(bank uint8, addr uint16) (int, error) {
	if addr < romWindowAddr {
		if bank != 0 {
			return 0, fmt.Errorf("gs/rom: address %#04x is in bank 0, not bank %#02x", addr, bank)
		}
		return int(addr), nil
	}
	if addr >= romWindowEnd {
		return 0, fmt.Errorf("gs/rom: address %#04x is outside the ROM window", addr)
	}
	if bank == 0 {
		return 0, fmt.Errorf("gs/rom: address %#04x needs a nonzero ROMX bank", addr)
	}
	return int(bank)*romBankSize + int(addr-romWindowAddr), nil
}

func offsetBankAddr(off int) (uint8, uint16, error) {
	if off < 0 {
		return 0, 0, fmt.Errorf("gs/rom: negative ROM offset %d", off)
	}
	if off < romBankSize {
		return 0, uint16(off), nil
	}
	return uint8(off / romBankSize), uint16(romWindowAddr + off%romBankSize), nil
}

func readU16(rom []byte, off int) (uint16, error) {
	if off < 0 || off+2 > len(rom) {
		return 0, fmt.Errorf("gs/rom: word at %#x exceeds ROM of %d bytes", off, len(rom))
	}
	return uint16(rom[off]) | uint16(rom[off+1])<<8, nil
}

func mustInROM(rom []byte, off, n int, what string) error {
	if off < 0 || off+n > len(rom) {
		return fmt.Errorf("gs/rom: %s at %#x+%d exceeds ROM of %d bytes", what, off, n, len(rom))
	}
	return nil
}
