package rom

import (
	"bytes"
	"fmt"

	gsdata "github.com/maestroi/pokepilot/gs/data"
)

const (
	gen2BaseDataSize  = 32
	gen2TMHMOffset    = 24
	gen2TMHMBytes     = 8
	gen2SpeciesCount  = 251
	gen2MachineCount  = 57
)

// TMHMCompatibility is the ROM-backed Gen-II machine compatibility table.
// BaseData is stored in Pokédex order and the final eight bytes of every
// 32-byte species record are the 57 TM/HM compatibility bits.
type TMHMCompatibility struct {
	rom      []byte
	baseData int
}

// ParseTMHMCompatibility locates the supported retail BaseData table by its
// first three canonical records. The active game profile already pins the ROM
// revision by SHA-1; this signature keeps the parser independent of a magic
// bank/address and fails closed if the table cannot be identified uniquely.
func ParseTMHMCompatibility(romData []byte) (TMHMCompatibility, error) {
	base, err := locateBaseData(romData)
	if err != nil {
		return TMHMCompatibility{}, err
	}
	need := base + gen2SpeciesCount*gen2BaseDataSize
	if need > len(romData) {
		return TMHMCompatibility{}, fmt.Errorf("gs/rom: BaseData truncated: need %#x bytes, ROM has %#x", need, len(romData))
	}
	return TMHMCompatibility{rom: romData, baseData: base}, nil
}

// MachineNumberForItem returns Gen-II's 1-based TM/HM number (TM01..TM50,
// HM01..HM07) for a native item id.
func MachineNumberForItem(item uint8) (int, bool) {
	for i, raw := range gsdata.MachineItems {
		if raw == item {
			return i + 1, true
		}
	}
	return 0, false
}

// CanLearn reports the exact compatibility bit encoded in BaseData.
func (t TMHMCompatibility) CanLearn(species, item uint8) (bool, error) {
	if species == 0 || int(species) > gen2SpeciesCount {
		return false, fmt.Errorf("gs/rom: species %#02x outside 1..%d", species, gen2SpeciesCount)
	}
	machine, ok := MachineNumberForItem(item)
	if !ok || machine < 1 || machine > gen2MachineCount {
		return false, fmt.Errorf("gs/rom: item %#02x is not a Gen-II TM/HM", item)
	}
	if len(t.rom) == 0 {
		return false, fmt.Errorf("gs/rom: empty TM/HM compatibility table")
	}
	off := t.baseData + (int(species)-1)*gen2BaseDataSize + gen2TMHMOffset
	if off < 0 || off+gen2TMHMBytes > len(t.rom) {
		return false, fmt.Errorf("gs/rom: compatibility record for species %#02x is truncated", species)
	}
	bit := machine - 1
	return t.rom[off+bit/8]&(1<<uint(bit%8)) != 0, nil
}

func locateBaseData(romData []byte) (int, error) {
	// dex id followed by HP/Atk/Def/Spd/SpA/SpD.
	signatures := [][]byte{
		{0x01, 45, 49, 49, 45, 65, 65},       // Bulbasaur
		{0x02, 60, 62, 63, 60, 80, 80},       // Ivysaur
		{0x03, 80, 82, 83, 80, 100, 100},     // Venusaur
	}
	limit := len(romData) - (len(signatures)-1)*gen2BaseDataSize
	found := -1
	for off := 0; off+len(signatures[0]) <= limit; off++ {
		match := true
		for i, sig := range signatures {
			start := off + i*gen2BaseDataSize
			if start+len(sig) > len(romData) || !bytes.Equal(romData[start:start+len(sig)], sig) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("gs/rom: BaseData signature is ambiguous at %#x and %#x", found, off)
		}
		found = off
	}
	if found < 0 {
		return 0, fmt.Errorf("gs/rom: BaseData signature not found")
	}
	return found, nil
}
