package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeBoxReadsSpeciesMovesAndMasksBoxFlag(t *testing.T) {
	var mem Mem
	mem[sym.CurrentBoxNum] = 0x80 | 3
	mem[sym.BoxCount] = 2
	mem[sym.BoxMon1+sym.BoxMonSpecies] = 0x54
	copy(mem[sym.BoxMon1+sym.BoxMonMoves:], []byte{15, 33, 0, 0})
	second := sym.BoxMon1 + sym.BoxMonSize
	mem[second+sym.BoxMonSpecies] = 0xA5
	copy(mem[second+sym.BoxMonMoves:], []byte{57, 70, 0, 0})

	box := DecodeBox(&mem)
	if box.Number != 3 || box.Count != 2 || len(box.Mons) != 2 {
		t.Fatalf("DecodeBox header = number %d count %d len %d, want 3/2/2", box.Number, box.Count, len(box.Mons))
	}
	if box.Mons[0].Species != 0x54 || box.Mons[0].Moves != [4]uint8{15, 33, 0, 0} {
		t.Fatalf("first box mon = %+v", box.Mons[0])
	}
	if box.Mons[1].Species != 0xA5 || box.Mons[1].Moves != [4]uint8{57, 70, 0, 0} {
		t.Fatalf("second box mon = %+v", box.Mons[1])
	}
}

func TestDecodeBoxClampsCorruptCount(t *testing.T) {
	var mem Mem
	mem[sym.BoxCount] = 0xff
	if got := DecodeBox(&mem); got.Count != activeBoxCapacity || len(got.Mons) != activeBoxCapacity {
		t.Fatalf("clamped box count = %d len %d, want %d", got.Count, len(got.Mons), activeBoxCapacity)
	}
}
