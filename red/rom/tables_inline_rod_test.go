package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/gen1rom"
)

// inlineRodImageSize is unique to this test so its binding matches nothing else.
const inlineRodImageSize = 0x100000 + 0x1234

func TestFishingEncountersReadInlineSuperRodLayout(t *testing.T) {
	layout := redTables
	layout.SuperRod = gen1rom.Symbol{Bank: 0x3D, Addr: 0x5EDA}
	layout.SuperRodFormat = gen1rom.SuperRodInline
	gen1rom.RegisterTableLayout(func(rom []byte) bool { return len(rom) == inlineRodImageSize }, layout)

	romData := make([]byte, inlineRodImageSize)
	oldOff, _ := layout.ItemUseOldRod.Offset()
	copy(romData[oldOff+oldRodOpcodeOffset:], []byte{0x01, 0x85, 5})
	off, _ := layout.SuperRod.Offset()
	// PALLET_TOWN: STARYU 10, TENTACOOL 10, STARYU 5, empty; then $ff.
	copy(romData[off:], []byte{0x00, 0x1B, 10, 0x18, 10, 0x1B, 5, 0x00, 0, 0xFF})

	got, err := FishingEncounters(romData)
	if err != nil {
		t.Fatal(err)
	}
	var super []FishingEncounter
	for _, f := range got {
		if f.Rod == RodSuper {
			super = append(super, f)
		}
	}
	want := []FishingEncounter{
		{MapID: 0, Rod: RodSuper, Species: 0x1B, Level: 10},
		{MapID: 0, Rod: RodSuper, Species: 0x18, Level: 10},
		{MapID: 0, Rod: RodSuper, Species: 0x1B, Level: 5},
	}
	if len(super) != len(want) {
		t.Fatalf("super rod = %+v, want %+v", super, want)
	}
	for i := range want {
		if super[i] != want[i] {
			t.Fatalf("super rod[%d] = %+v, want %+v", i, super[i], want[i])
		}
	}
}
