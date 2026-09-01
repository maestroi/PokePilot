package rom

import "fmt"

// Banked symbol addresses from pokered.sym, written bank:addr.
const (
	mapHeaderPointersBank uint8  = 0x00
	mapHeaderPointersAddr uint16 = 0x01AE
	mapHeaderBanksBank    uint8  = 0x03
	mapHeaderBanksAddr    uint16 = 0x423D
	tilesetsBank          uint8  = 0x03
	tilesetsAddr          uint16 = 0x47BE
)

// Warp is a floor tile that teleports the player to another map.
type Warp struct {
	X          uint8
	Y          uint8
	DestWarpID uint8
	DestMap    uint8
}

// Sign is a floor tile with a signpost text.
type Sign struct {
	X      uint8
	Y      uint8
	TextID uint8
}

// Object is a sprite (NPC, item, or trainer) on the map.
type Object struct {
	X        uint8
	Y        uint8
	SpriteID uint8
	Movement uint8
	Range    uint8
	TextID   uint8

	// ItemID is set when TextID has the 0x80 (item) bit; zero otherwise.
	ItemID uint8
	// TrainerClass and TrainerSet are set when TextID has the 0x40 (trainer)
	// bit; zero otherwise.
	TrainerClass uint8
	TrainerSet   uint8
}

// Connection links this map to an adjacent map. Dir: 0=north 1=south 2=west 3=east.
type Connection struct {
	Dir   uint8
	MapID uint8
}

// MapHeader is one map's static header plus its object data.
type MapHeader struct {
	ID           uint8
	Tileset      uint8
	WidthBlocks  uint8
	HeightBlocks uint8
	BlocksAddr   uint16 // banked address of the block list
	TextsAddr    uint16 // address (map bank) of the text pointer table
	ScriptAddr   uint16 // address (map bank) of the default map script
	Bank         uint8
	BorderBlock  uint8
	Connections  []Connection
	Warps        []Warp
	Signs        []Sign
	Objects      []Object
}

// bankedOffset converts a banked address (bank:addr) to a ROM file offset.
func bankedOffset(bank uint8, addr uint16) (int, error) {
	if addr >= 0x4000 {
		return int(bank)*0x4000 + int(addr-0x4000), nil
	}
	if bank != 0 {
		return 0, fmt.Errorf("address %04X in bank %d is below 0x4000", addr, bank)
	}
	return int(addr), nil
}

// reader is a bounds-checked cursor over a ROM image.
type reader struct {
	rom []byte
	off int
}

func (r *reader) byte() (byte, error) {
	if r.off >= len(r.rom) {
		return 0, fmt.Errorf("read at offset %d exceeds ROM of %d bytes", r.off, len(r.rom))
	}
	b := r.rom[r.off]
	r.off++
	return b, nil
}

func (r *reader) u16() (uint16, error) {
	if r.off+2 > len(r.rom) {
		return 0, fmt.Errorf("read at offset %d exceeds ROM of %d bytes", r.off, len(r.rom))
	}
	v := uint16(r.rom[r.off]) | uint16(r.rom[r.off+1])<<8
	r.off += 2
	return v, nil
}

func (r *reader) skip(n int) error {
	if r.off+n > len(r.rom) {
		return fmt.Errorf("skip %d bytes at offset %d exceeds ROM of %d bytes", n, r.off, len(r.rom))
	}
	r.off += n
	return nil
}

func mapErr(mapID uint8, err error) error {
	return fmt.Errorf("map %d: %v", mapID, err)
}

// ParseMap reads one map header and its object data from the ROM image.
func ParseMap(rom []byte, mapID uint8) (MapHeader, error) {
	var h MapHeader
	h.ID = mapID

	bankOff, err := bankedOffset(mapHeaderBanksBank, mapHeaderBanksAddr)
	if err != nil {
		return h, mapErr(mapID, err)
	}
	bankAt := bankOff + int(mapID)
	if bankAt >= len(rom) {
		return h, mapErr(mapID, fmt.Errorf("bank table offset %d exceeds ROM of %d bytes", bankAt, len(rom)))
	}
	h.Bank = rom[bankAt]

	ptrOff, err := bankedOffset(mapHeaderPointersBank, mapHeaderPointersAddr)
	if err != nil {
		return h, mapErr(mapID, err)
	}
	ptrAt := ptrOff + int(mapID)*2
	if ptrAt+2 > len(rom) {
		return h, mapErr(mapID, fmt.Errorf("header pointer offset %d exceeds ROM of %d bytes", ptrAt, len(rom)))
	}
	ptr := uint16(rom[ptrAt]) | uint16(rom[ptrAt+1])<<8

	headerOff, err := bankedOffset(h.Bank, ptr)
	if err != nil {
		return h, mapErr(mapID, err)
	}
	if headerOff >= len(rom) {
		return h, mapErr(mapID, fmt.Errorf("header offset %d exceeds ROM of %d bytes", headerOff, len(rom)))
	}

	r := &reader{rom: rom, off: headerOff}
	if h.Tileset, err = r.byte(); err != nil {
		return h, mapErr(mapID, err)
	}
	if h.HeightBlocks, err = r.byte(); err != nil {
		return h, mapErr(mapID, err)
	}
	if h.WidthBlocks, err = r.byte(); err != nil {
		return h, mapErr(mapID, err)
	}
	if h.BlocksAddr, err = r.u16(); err != nil {
		return h, mapErr(mapID, err)
	}
	if h.TextsAddr, err = r.u16(); err != nil { // text pointer table
		return h, mapErr(mapID, err)
	}
	if h.ScriptAddr, err = r.u16(); err != nil { // default map script
		return h, mapErr(mapID, err)
	}
	connFlags, err := r.byte()
	if err != nil {
		return h, mapErr(mapID, err)
	}

	// Connection blocks appear in the order north, south, west, east.
	for dir, bit := range [4]uint8{0x08, 0x04, 0x02, 0x01} {
		if connFlags&bit == 0 {
			continue
		}
		dest, err := r.byte()
		if err != nil {
			return h, mapErr(mapID, err)
		}
		h.Connections = append(h.Connections, Connection{Dir: uint8(dir), MapID: dest})
		if err := r.skip(10); err != nil {
			return h, mapErr(mapID, err)
		}
	}

	objPtr, err := r.u16()
	if err != nil {
		return h, mapErr(mapID, err)
	}

	objOff, err := bankedOffset(h.Bank, objPtr)
	if err != nil {
		return h, mapErr(mapID, err)
	}
	o := &reader{rom: rom, off: objOff}

	if h.BorderBlock, err = o.byte(); err != nil {
		return h, mapErr(mapID, err)
	}

	nWarps, err := o.byte()
	if err != nil {
		return h, mapErr(mapID, err)
	}
	for i := 0; i < int(nWarps); i++ {
		var w Warp
		if w.Y, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		if w.X, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		if w.DestWarpID, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		if w.DestMap, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		h.Warps = append(h.Warps, w)
	}

	nSigns, err := o.byte()
	if err != nil {
		return h, mapErr(mapID, err)
	}
	for i := 0; i < int(nSigns); i++ {
		var s Sign
		if s.Y, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		if s.X, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		if s.TextID, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		h.Signs = append(h.Signs, s)
	}

	nObjects, err := o.byte()
	if err != nil {
		return h, mapErr(mapID, err)
	}
	for i := 0; i < int(nObjects); i++ {
		var obj Object
		if obj.SpriteID, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		if obj.Y, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		if obj.X, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		if obj.Movement, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		if obj.Range, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		if obj.TextID, err = o.byte(); err != nil {
			return h, mapErr(mapID, err)
		}
		obj.Y -= 4 // object coordinates are stored with +4 added
		obj.X -= 4
		switch {
		case obj.TextID&0x40 != 0: // trainer entry: class then roster
			if obj.TrainerClass, err = o.byte(); err != nil {
				return h, mapErr(mapID, err)
			}
			if obj.TrainerSet, err = o.byte(); err != nil {
				return h, mapErr(mapID, err)
			}
		case obj.TextID&0x80 != 0: // item entry: item id
			if obj.ItemID, err = o.byte(); err != nil {
				return h, mapErr(mapID, err)
			}
		}
		h.Objects = append(h.Objects, obj)
	}

	return h, nil
}

// Blocks returns the raw block ids for a map: WidthBlocks*HeightBlocks bytes.
func Blocks(rom []byte, h MapHeader) ([]byte, error) {
	off, err := bankedOffset(h.Bank, h.BlocksAddr)
	if err != nil {
		return nil, mapErr(h.ID, err)
	}
	n := int(h.WidthBlocks) * int(h.HeightBlocks)
	if off+n > len(rom) {
		return nil, mapErr(h.ID, fmt.Errorf("block list at offset %d (%d bytes) exceeds ROM of %d bytes", off, n, len(rom)))
	}
	out := make([]byte, n)
	copy(out, rom[off:off+n])
	return out, nil
}
