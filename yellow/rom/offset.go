package rom

import "fmt"

// bankedOffset converts a Game Boy bank:address pair into a ROM file offset.
// Yellow owns this tiny layout primitive so its decoders do not depend on
// unexported Red/gen1rom implementation details.
func bankedOffset(bank uint8, addr uint16) (int, error) {
	switch {
	case addr < 0x4000:
		if bank != 0 {
			return 0, fmt.Errorf("yellow rom: fixed-bank address %#04x with bank %d", addr, bank)
		}
		return int(addr), nil
	case addr < 0x8000:
		return int(bank)*0x4000 + int(addr-0x4000), nil
	default:
		return 0, fmt.Errorf("yellow rom: address %#04x is outside cartridge ROM", addr)
	}
}
