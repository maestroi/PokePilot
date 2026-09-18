package gen1rom

import "testing"

func TestParseMapAtSharedGen1Format(t *testing.T) {
	rom := make([]byte, 0x9000)
	// Header at bank 2:0x4000 -> file offset 0x8000.
	at := 0x8000
	rom[at+0] = 3    // tileset
	rom[at+1] = 2    // height
	rom[at+2] = 4    // width
	rom[at+3] = 0x20 // blocks ptr 0x4120
	rom[at+4] = 0x41
	rom[at+5] = 0x40 // texts ptr
	rom[at+6] = 0x41
	rom[at+7] = 0x60 // script ptr
	rom[at+8] = 0x41
	rom[at+9] = 0x08  // north connection only
	rom[at+10] = 0x0c // Route 1
	// six connection bytes skipped.
	rom[at+17] = 0xfe // y alignment = -2
	rom[at+18] = 0x02 // x alignment, used by N/S offset
	// two bytes skipped.
	rom[at+21] = 0x80 // object ptr 0x4180
	rom[at+22] = 0x41

	obj := 0x8180
	rom[obj] = 0x2a // border block
	rom[obj+1] = 1
	rom[obj+2] = 7 // warp y
	rom[obj+3] = 8 // warp x
	rom[obj+4] = 2 // destination warp
	rom[obj+5] = 9 // destination map
	rom[obj+6] = 1
	rom[obj+7] = 4 // sign y
	rom[obj+8] = 5 // sign x
	rom[obj+9] = 6 // text id
	rom[obj+10] = 2
	// trainer object
	copy(rom[obj+11:], []byte{1, 8, 9, MovementStay, 0, 0x41, 12, 3})
	// item object
	copy(rom[obj+19:], []byte{2, 10, 11, MovementWalk, 1, 0x82, 0x14})

	h, err := ParseMapAt(rom, 5, HeaderRef{Bank: 2, Addr: 0x4000})
	if err != nil {
		t.Fatal(err)
	}
	if h.ID != 5 || h.Tileset != 3 || h.WidthBlocks != 4 || h.HeightBlocks != 2 {
		t.Fatalf("header = %+v", h)
	}
	if len(h.Connections) != 1 || h.Connections[0].MapID != 0x0c || h.Connections[0].Offset != 2 {
		t.Fatalf("connections = %+v", h.Connections)
	}
	if len(h.Warps) != 1 || h.Warps[0].DestMap != 9 || h.Warps[0].X != 8 || h.Warps[0].Y != 7 {
		t.Fatalf("warps = %+v", h.Warps)
	}
	if len(h.Signs) != 1 || h.Signs[0].TextID != 6 {
		t.Fatalf("signs = %+v", h.Signs)
	}
	if len(h.Objects) != 2 {
		t.Fatalf("objects = %+v", h.Objects)
	}
	if got := h.Objects[0]; got.X != 5 || got.Y != 4 || got.TrainerClass != 12 || got.TrainerSet != 3 {
		t.Fatalf("trainer = %+v", got)
	}
	if got := h.Objects[1]; got.X != 7 || got.Y != 6 || got.ItemID != 0x14 {
		t.Fatalf("item = %+v", got)
	}
}
