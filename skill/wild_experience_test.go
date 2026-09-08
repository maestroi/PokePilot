package skill

import "testing"

func TestWildGrassSlotsPreservesROMOrderAndLevels(t *testing.T) {
	// WildDataPointers is at 03:4EEB. Point map 0 at 03:5000 and build the
	// normal one-byte rate plus ten (level,species) slots there.
	romData := make([]byte, 0xd000+1+2*wildSlots)
	pointerBase, err := bankedOff(trainWildBank, trainWildAddr)
	if err != nil {
		t.Fatal(err)
	}
	romData[pointerBase], romData[pointerBase+1] = 0x00, 0x50
	record, err := bankedOff(trainWildBank, 0x5000)
	if err != nil {
		t.Fatal(err)
	}
	romData[record] = 10
	for i := 0; i < wildSlots; i++ {
		romData[record+1+2*i] = uint8(i + 2)
		romData[record+2+2*i] = uint8(i + 1)
	}

	got, err := WildGrassSlots(romData, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != wildSlots {
		t.Fatalf("len(WildGrassSlots) = %d, want %d", len(got), wildSlots)
	}
	for i, slot := range got {
		if slot.ID != uint8(i+1) || slot.Level != uint8(i+2) {
			t.Fatalf("slot %d = %+v, want species=%d level=%d", i, slot, i+1, i+2)
		}
	}
}

func TestWildGrassSlotsZeroRateIsEmpty(t *testing.T) {
	romData := make([]byte, 0xd000+1+2*wildSlots)
	pointerBase, _ := bankedOff(trainWildBank, trainWildAddr)
	romData[pointerBase], romData[pointerBase+1] = 0x00, 0x50
	record, _ := bankedOff(trainWildBank, 0x5000)
	romData[record] = 0
	got, err := WildGrassSlots(romData, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("zero-rate WildGrassSlots = %+v, want empty", got)
	}
}
