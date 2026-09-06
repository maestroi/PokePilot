package skill

import (
	"bytes"
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestReadLiveMapBlocksSkipsConnectionBorder(t *testing.T) {
	const widthBlocks, heightBlocks = 2, 2
	stride := widthBlocks + 2*liveMapBorderBlocks
	first := liveMapBorderBlocks*stride + liveMapBorderBlocks

	mem := make(map[uint16]uint8)
	for i := 0; i < sym.OverworldMapLen; i++ {
		mem[sym.OverworldMap+uint16(i)] = 0xee
	}
	want := []byte{0x11, 0x12, 0x21, 0x22}
	for y := 0; y < heightBlocks; y++ {
		for x := 0; x < widthBlocks; x++ {
			off := first + y*stride + x
			mem[sym.OverworldMap+uint16(off)] = want[y*widthBlocks+x]
		}
	}

	got, err := readLiveMapBlocks(func(addr uint16) uint8 { return mem[addr] }, widthBlocks, heightBlocks)
	if err != nil {
		t.Fatalf("readLiveMapBlocks: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("blocks = % x, want % x", got, want)
	}
}

func TestReadLiveMapBlocksRejectsBufferOverflow(t *testing.T) {
	_, err := readLiveMapBlocks(func(uint16) uint8 { return 0 }, 255, 255)
	if err == nil {
		t.Fatal("readLiveMapBlocks accepted dimensions larger than wOverworldMap")
	}
}
