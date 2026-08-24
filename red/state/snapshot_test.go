package state

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
)

func TestSnapshotFromEmulator(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	e, err := emu.Open(path)
	if err != nil {
		t.Fatalf("emu.Open: %v", err)
	}
	defer e.Close()
	e.StepFrames(300)

	var m1, m2 Mem
	Snapshot(e, &m1)

	nz := 0
	for a := uint16(0xC000); a < 0xE000; a++ {
		if m1[a] != 0 {
			nz++
		}
	}
	if nz == 0 {
		t.Fatal("snapshot WRAM range 0xC000..0xDFFF is all zeroes")
	}

	Snapshot(e, &m2)
	if m1 != m2 {
		t.Fatal("two snapshots without stepping differ; observation is not side-effect free")
	}
}
