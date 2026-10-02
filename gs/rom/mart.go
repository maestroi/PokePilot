package rom

import "fmt"

// MartItems returns the item ids one MART_* shelf stocks, in order.
func MartItems(rom []byte, martID int) ([]uint8, error) {
	if martID < 0 || martID >= numMarts {
		return nil, fmt.Errorf("gs/rom: mart %d is outside 0..%d", martID, numMarts-1)
	}
	base, err := locateMarts(rom)
	if err != nil {
		return nil, err
	}
	bank, _, err := offsetBankAddr(base)
	if err != nil {
		return nil, err
	}
	addr, err := readU16(rom, base+martID*2)
	if err != nil {
		return nil, err
	}
	rec, err := bankedOffset(bank, addr)
	if err != nil {
		return nil, err
	}
	if err := mustInROM(rom, rec, 1, "mart count"); err != nil {
		return nil, err
	}
	n := int(rom[rec])
	if err := mustInROM(rom, rec+1, n+1, "mart items"); err != nil {
		return nil, err
	}
	if rom[rec+1+n] != 0xff {
		return nil, fmt.Errorf("gs/rom: mart %d is not -1 terminated", martID)
	}
	out := make([]uint8, n)
	copy(out, rom[rec+1:rec+1+n])
	return out, nil
}
