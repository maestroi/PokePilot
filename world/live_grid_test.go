package world

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

func TestBuildFromBlocksRejectsShortBlockMap(t *testing.T) {
	h := rom.MapHeader{ID: 7, WidthBlocks: 2, HeightBlocks: 1}
	_, err := BuildFromBlocks(nil, h, []byte{0x01})
	if err == nil {
		t.Fatal("BuildFromBlocks accepted a short block map")
	}
}
