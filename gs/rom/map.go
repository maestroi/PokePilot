package rom

import (
	"fmt"

	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/worldmodel"
)

const (
	connEast  uint8 = 1 << 0
	connWest  uint8 = 1 << 1
	connSouth uint8 = 1 << 2
	connNorth uint8 = 1 << 3
)

// MapHeader is the Gold/Silver first and second map headers.
type MapHeader struct {
	ID           uint16
	Tileset      uint8
	Environment  uint8
	BorderBlock  uint8
	WidthBlocks  uint8
	HeightBlocks uint8
	Warps        []Warp
	Connections  []Connection
	Objects      []Object
	Signs        []Sign
}

// Warp is one def_warp_events entry. DestWarpID is 0-based.
type Warp struct {
	X, Y       uint8
	DestWarpID uint8
	DestMap    uint16
}

// Connection is one attributes.asm connection, with the decomp's signed
// block offset recovered from the encoded seam bytes.
type Connection struct {
	Dir    uint8
	MapID  uint16
	Offset int8
}

// Sign is one def_bg_events entry.
type Sign struct {
	X, Y     uint8
	Function uint8
	Script   uint16
}

func (h MapHeader) NativeHeader() worldmodel.NativeMapHeader {
	out := worldmodel.NativeMapHeader{
		ID:           h.ID,
		WidthBlocks:  h.WidthBlocks,
		HeightBlocks: h.HeightBlocks,
		Warps:        make([]worldmodel.NativeWarp, 0, len(h.Warps)),
		Connections:  make([]worldmodel.NativeConnection, 0, len(h.Connections)),
	}
	for _, w := range h.Warps {
		out.Warps = append(out.Warps, worldmodel.NativeWarp{
			X: w.X, Y: w.Y, DestWarpID: w.DestWarpID, DestMap: w.DestMap,
		})
	}
	for _, c := range h.Connections {
		out.Connections = append(out.Connections, worldmodel.NativeConnection{
			Dir: c.Dir, MapID: c.MapID, Offset: c.Offset,
		})
	}
	return out
}

// ParseMap reads one map from romData using located MapGroupPointers.
func ParseMap(rom []byte, mapID uint16) (MapHeader, error) {
	off, err := locateMapGroupPointers(rom)
	if err != nil {
		return MapHeader{}, err
	}
	return parseMapAt(rom, Tables{MapGroupPointers: off}, mapID)
}

func parseMapAt(rom []byte, tables Tables, mapID uint16) (MapHeader, error) {
	info, ok := gsdata.Map(mapID)
	if !ok {
		return MapHeader{ID: mapID}, fmt.Errorf("gs/rom: map %#04x is not in the generated catalog", mapID)
	}
	bank, _, err := offsetBankAddr(tables.MapGroupPointers)
	if err != nil {
		return MapHeader{ID: mapID}, err
	}
	if info.Group == 0 || info.Group > numMapGroups {
		return MapHeader{ID: mapID}, fmt.Errorf("gs/rom: map %#04x has invalid group %d", mapID, info.Group)
	}
	groupPtr, err := readU16(rom, tables.MapGroupPointers+int(info.Group-1)*2)
	if err != nil {
		return MapHeader{ID: mapID}, err
	}
	groupOff, err := bankedOffset(bank, groupPtr)
	if err != nil {
		return MapHeader{ID: mapID}, err
	}
	if info.Number == 0 {
		return MapHeader{ID: mapID}, fmt.Errorf("gs/rom: map %#04x has zero map number", mapID)
	}
	headerOff := groupOff + int(info.Number-1)*mapHeaderLen
	if err := mustInROM(rom, headerOff, mapHeaderLen, "map header"); err != nil {
		return MapHeader{ID: mapID}, err
	}

	h := MapHeader{
		ID:          mapID,
		Tileset:     rom[headerOff+1],
		Environment: rom[headerOff+2],
	}
	attrBank := rom[headerOff]
	attrAddr := uint16(rom[headerOff+3]) | uint16(rom[headerOff+4])<<8
	attrOff, err := bankedOffset(attrBank, attrAddr)
	if err != nil {
		return h, err
	}
	if err := mustInROM(rom, attrOff, mapAttrLen, "map attributes"); err != nil {
		return h, err
	}
	h.BorderBlock = rom[attrOff]
	h.HeightBlocks = rom[attrOff+1]
	h.WidthBlocks = rom[attrOff+2]
	if h.WidthBlocks != info.WidthBlocks || h.HeightBlocks != info.HeightBlocks {
		return h, fmt.Errorf("gs/rom: map %#04x attributes %dx%d, catalog %dx%d",
			mapID, h.WidthBlocks, h.HeightBlocks, info.WidthBlocks, info.HeightBlocks)
	}

	scriptBank := rom[attrOff+6]
	eventAddr := uint16(rom[attrOff+9]) | uint16(rom[attrOff+10])<<8
	eventOff, err := bankedOffset(scriptBank, eventAddr)
	if err != nil {
		return h, err
	}
	if err := readMapEvents(rom, eventOff, scriptBank, &h); err != nil {
		return h, err
	}

	flags := rom[attrOff+11]
	connOff := attrOff + mapAttrLen
	var err2 error
	h.Connections, err2 = readConnections(rom, connOff, flags)
	if err2 != nil {
		return h, err2
	}
	return h, nil
}

func readConnections(rom []byte, off int, flags uint8) ([]Connection, error) {
	order := []struct {
		bit uint8
		dir uint8
	}{
		{connNorth, dirNorth},
		{connSouth, dirSouth},
		{connWest, dirWest},
		{connEast, dirEast},
	}
	var out []Connection
	for _, step := range order {
		if flags&step.bit == 0 {
			continue
		}
		if err := mustInROM(rom, off, mapConnectionLen, "connection"); err != nil {
			return nil, err
		}
		group, number := rom[off], rom[off+1]
		dest := gsdata.NativeMapID(group, number)
		y, x := int8(rom[off+8]), int8(rom[off+9])
		var offset int8
		switch step.dir {
		case dirNorth, dirSouth:
			offset = -x / 2
		default:
			offset = -y / 2
		}
		out = append(out, Connection{Dir: step.dir, MapID: dest, Offset: offset})
		off += mapConnectionLen
	}
	return out, nil
}

func readMapEvents(rom []byte, off int, scriptBank uint8, h *MapHeader) error {
	// Every *_MapEvents block starts with `db 0, 0 ; filler`.
	if err := mustInROM(rom, off, 2, "event filler"); err != nil {
		return err
	}
	off += 2
	if err := mustInROM(rom, off, 1, "warp count"); err != nil {
		return err
	}
	nWarp := int(rom[off])
	off++
	if err := mustInROM(rom, off, nWarp*warpEventLen, "warps"); err != nil {
		return err
	}
	h.Warps = make([]Warp, 0, nWarp)
	for i := 0; i < nWarp; i++ {
		y := rom[off]
		x := rom[off+1]
		destWarp := rom[off+2]
		dest := gsdata.NativeMapID(rom[off+3], rom[off+4])
		if destWarp == 0 {
			return fmt.Errorf("gs/rom: map %#04x warp %d has zero dest warp", h.ID, i)
		}
		h.Warps = append(h.Warps, Warp{X: x, Y: y, DestWarpID: destWarp - 1, DestMap: dest})
		off += warpEventLen
	}

	if err := mustInROM(rom, off, 1, "coord count"); err != nil {
		return err
	}
	nCoord := int(rom[off])
	off++
	if err := mustInROM(rom, off, nCoord*coordEventLen, "coord events"); err != nil {
		return err
	}
	off += nCoord * coordEventLen

	if err := mustInROM(rom, off, 1, "bg count"); err != nil {
		return err
	}
	nBG := int(rom[off])
	off++
	if err := mustInROM(rom, off, nBG*bgEventLen, "bg events"); err != nil {
		return err
	}
	h.Signs = make([]Sign, 0, nBG)
	for i := 0; i < nBG; i++ {
		h.Signs = append(h.Signs, Sign{
			Y: rom[off], X: rom[off+1], Function: rom[off+2],
			Script: uint16(rom[off+3]) | uint16(rom[off+4])<<8,
		})
		off += bgEventLen
	}

	if err := mustInROM(rom, off, 1, "object count"); err != nil {
		return err
	}
	nObj := int(rom[off])
	off++
	if err := mustInROM(rom, off, nObj*objectEventLen, "object events"); err != nil {
		return err
	}
	h.Objects = make([]Object, 0, nObj)
	for i := 0; i < nObj; i++ {
		obj, err := decodeObject(rom, off, i, scriptBank)
		if err != nil {
			return err
		}
		h.Objects = append(h.Objects, obj)
		off += objectEventLen
	}
	return nil
}
