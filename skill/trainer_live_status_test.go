package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestGenericTrainerReachableLiveUsesReplacedMapBlocks(t *testing.T) {
	const (
		mapID       = uint8(0xC7)
		tileset     = uint8(1)
		tilesetBase = 0xC7BE // bank 3, address 0x47BE
		blockBase   = 0xC800 // bank 3, address 0x4800
	)

	romData := make([]byte, 0xD000)
	entry := tilesetBase + int(tileset)*12
	romData[entry] = 3      // tileset data bank
	romData[entry+1] = 0x00 // block pointer 0x4800
	romData[entry+2] = 0x48
	romData[entry+5] = 0x00 // collision pointer 0x0200
	romData[entry+6] = 0x02
	romData[0x0200] = 0x01 // only tile 0x01 is walkable
	romData[0x0201] = 0xff
	romData[0x0c7e] = 0xff // no land tile-pair restrictions

	for i := 0; i < 16; i++ {
		romData[blockBase+i] = 0x01    // block 0: fully walkable
		romData[blockBase+16+i] = 0x02 // block 1: fully blocked
	}

	h := rom.MapHeader{
		ID:           mapID,
		Tileset:      tileset,
		WidthBlocks:  2,
		HeightBlocks: 1,
		Bank:         1,
		BlocksAddr:   0x5000,
	}
	// Static ROM geometry says both blocks are open. This is the stale view
	// the observer used before the Rocket Hideout B1F regression.
	romData[0x5000] = 0
	romData[0x5001] = 0

	var mem state.Mem
	mem[sym.CurMap] = mapID
	mem[sym.CurMapWidth] = 2
	mem[sym.CurMapHeight] = 1
	mem[sym.XCoord] = 0
	mem[sym.YCoord] = 0

	// wOverworldMap has a 3-block connection border. For a 2x1 map the first
	// interior block begins at offset 27. The live script-replaced second block
	// is closed even though the immutable ROM block map above still says open.
	mem[sym.OverworldMap+27] = 0
	mem[sym.OverworldMap+28] = 1

	if !genericTrainerReachable(romData, &mem, h, 3, 0) {
		t.Fatal("static fixture should consider the trainer reachable")
	}
	if genericTrainerReachableLive(romData, &mem, h, 3, 0) {
		t.Fatal("live reachability ignored the script-replaced closed block")
	}
}
