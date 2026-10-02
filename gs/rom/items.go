package rom

import "fmt"

// ItemAttribute is one ItemAttributes record.
type ItemAttribute struct {
	ID          uint8
	Price       uint16
	HeldEffect  uint8
	Param       uint8
	Permissions uint8
	Pocket      uint8
	Help        uint8
}

// LookupItem reads item id from ItemAttributes. Id 0 is NO_ITEM.
func LookupItem(rom []byte, id uint8) (ItemAttribute, error) {
	if id == 0 {
		return ItemAttribute{}, fmt.Errorf("gs/rom: item id 0 is NO_ITEM")
	}
	base, err := locateItemAttributes(rom)
	if err != nil {
		return ItemAttribute{}, err
	}
	off := base + int(id-1)*itemAttrLen
	if err := mustInROM(rom, off, itemAttrLen, "item attribute"); err != nil {
		return ItemAttribute{}, err
	}
	return ItemAttribute{
		ID:          id,
		Price:       uint16(rom[off]) | uint16(rom[off+1])<<8,
		HeldEffect:  rom[off+2],
		Param:       rom[off+3],
		Permissions: rom[off+4],
		Pocket:      rom[off+5],
		Help:        rom[off+6],
	}, nil
}
