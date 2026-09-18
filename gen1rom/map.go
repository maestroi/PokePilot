// Package gen1rom contains ROM byte-format decoders shared by compatible
// Generation I games. Concrete game packages own addresses, valid map ids and
// version-specific tables; this package only decodes byte formats proven equal.
package gen1rom

import "fmt"

// HeaderRef identifies one map header in a banked Gen-I ROM.
type HeaderRef struct {
	Bank uint8
	Addr uint16
}

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

// Object is a sprite/NPC/item/trainer entry in Gen-I map object data.
type Object struct {
	X        uint8
	Y        uint8
	SpriteID uint8
	Movement uint8
	Range    uint8
	TextID   uint8

	ItemID       uint8
	TrainerClass uint8
	TrainerSet   uint8
}

const (
	MovementWalk uint8 = 0xFE
	MovementStay uint8 = 0xFF
)

// Connection links a map to an adjacent map. Dir: 0=N, 1=S, 2=W, 3=E.
type Connection struct {
	Dir    uint8
	MapID  uint8
	Offset int8
}

// These accessors satisfy world's portable connection view without making the
// shared Gen-I decoder depend on the world package.
func (c Connection) WorldDirection() uint8 { return c.Dir }
func (c Connection) WorldMapID() uint8     { return c.MapID }
func (c Connection) WorldOffset() int8     { return c.Offset }

// MapHeader is the common Gen-I map-header/object-data byte format.
type MapHeader struct {
	ID           uint8
	Tileset      uint8
	WidthBlocks  uint8
	HeightBlocks uint8
	BlocksAddr   uint16
	TextsAddr    uint16
	ScriptAddr   uint16
	Bank         uint8
	BorderBlock  uint8
	Connections  []Connection
	Warps        []Warp
	Signs        []Sign
	Objects      []Object
}

// BankedOffset converts an RGBDS bank:address pair into a ROM file offset.
func BankedOffset(bank uint8, addr uint16) (int, error) {
	if addr >= 0x4000 {
		return int(bank)*0x4000 + int(addr-0x4000), nil
	}
	if bank != 0 {
		return 0, fmt.Errorf("address %04X in bank %d is below 0x4000", addr, bank)
	}
	return int(addr), nil
}

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

// ParseMapAt decodes the shared Red/Blue/Yellow map header and object layout at
// a game-owned header reference.
func ParseMapAt(rom []byte, mapID uint8, ref HeaderRef) (MapHeader, error) {
	h := MapHeader{ID: mapID, Bank: ref.Bank}
	headerOff, err := BankedOffset(ref.Bank, ref.Addr)
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
	if h.TextsAddr, err = r.u16(); err != nil {
		return h, mapErr(mapID, err)
	}
	if h.ScriptAddr, err = r.u16(); err != nil {
		return h, mapErr(mapID, err)
	}
	connFlags, err := r.byte()
	if err != nil {
		return h, mapErr(mapID, err)
	}

	for dir, bit := range [4]uint8{0x08, 0x04, 0x02, 0x01} {
		if connFlags&bit == 0 {
			continue
		}
		dest, err := r.byte()
		if err != nil {
			return h, mapErr(mapID, err)
		}
		if err := r.skip(6); err != nil {
			return h, mapErr(mapID, err)
		}
		y, err := r.byte()
		if err != nil {
			return h, mapErr(mapID, err)
		}
		x, err := r.byte()
		if err != nil {
			return h, mapErr(mapID, err)
		}
		offset := int8(y)
		if dir < 2 {
			offset = int8(x)
		}
		h.Connections = append(h.Connections, Connection{Dir: uint8(dir), MapID: dest, Offset: offset})
		if err := r.skip(2); err != nil {
			return h, mapErr(mapID, err)
		}
	}

	objPtr, err := r.u16()
	if err != nil {
		return h, mapErr(mapID, err)
	}
	objOff, err := BankedOffset(h.Bank, objPtr)
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
		obj.Y -= 4
		obj.X -= 4
		switch {
		case obj.TextID&0x40 != 0:
			if obj.TrainerClass, err = o.byte(); err != nil {
				return h, mapErr(mapID, err)
			}
			if obj.TrainerSet, err = o.byte(); err != nil {
				return h, mapErr(mapID, err)
			}
		case obj.TextID&0x80 != 0:
			if obj.ItemID, err = o.byte(); err != nil {
				return h, mapErr(mapID, err)
			}
		}
		h.Objects = append(h.Objects, obj)
	}

	return h, nil
}

// Blocks returns WidthBlocks*HeightBlocks raw map block ids.
func Blocks(rom []byte, h MapHeader) ([]byte, error) {
	off, err := BankedOffset(h.Bank, h.BlocksAddr)
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
