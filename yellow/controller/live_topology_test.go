package controller

import (
	"testing"

	"github.com/maestroi/pokepilot/yellow/sym"
)

func TestReadYellowLiveMapBlocks(t *testing.T) {
	const width, height = 2, 2
	stride := width + 2*yellowLiveMapBorderBlocks
	first := yellowLiveMapBorderBlocks*stride + yellowLiveMapBorderBlocks

	mem := map[uint16]uint8{}
	for i := 0; i < sym.OverworldMapLen; i++ {
		mem[sym.OverworldMap+uint16(i)] = 0xee
	}
	want := []byte{0x11, 0x12, 0x21, 0x22}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			off := first + y*stride + x
			mem[sym.OverworldMap+uint16(off)] = want[y*width+x]
		}
	}

	got, err := readYellowLiveMapBlocks(func(addr uint16) uint8 { return mem[addr] }, width, height)
	if err != nil {
		t.Fatalf("readYellowLiveMapBlocks: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d blocks, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("block %d = %#02x, want %#02x", i, got[i], want[i])
		}
	}
}

func TestReadYellowLiveMapBlocksRejectsOversize(t *testing.T) {
	_, err := readYellowLiveMapBlocks(func(uint16) uint8 { return 0 }, 64, 64)
	if err == nil {
		t.Fatal("expected oversized live map buffer error")
	}
}
