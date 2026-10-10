package rom

import (
	"fmt"
	"sync"
	"unsafe"
)

// TypeMatchups (data/types/type_matchups.asm) is a flat list of three-byte
// entries — attacking type, defending type, multiplier in tenths — with a
// 0xFE marker before the two Ghost immunities that Foresight skips and 0xFF
// at the end. The battle engine reads it from ROM, so the adapter does too.
const (
	typeMatchupEntry   = 3
	typeMatchupsSplit  = 0xFE
	typeMatchupsEnd    = 0xFF
	typeMatchupsMax    = 256
	TypeNeutralEffect  = 10
	typeMatchupsAnchor = 6 // bytes of the leading entries used to locate the table
)

// The table's first two rows: NORMAL vs ROCK and NORMAL vs STEEL, both
// NOT_VERY_EFFECTIVE (type_constants.asm: NORMAL=0, ROCK=5, STEEL=9).
var typeMatchupsPrefix = []byte{0x00, 0x05, 0x05, 0x00, 0x09, 0x05}

var typeMatchupCache sync.Map // romKey -> map[[2]uint8]int

// typeROMKey identifies an image cheaply: backing array, length and
// cartridge header, so a reused allocation holding another ROM never
// inherits a stale chart.
type typeROMKey struct {
	data   uintptr
	n      int
	header [0x1c]byte
}

// TypeEffectiveness returns the multiplier in tenths for a move of moveType
// against a defender of def1/def2, applying both types (an identical pair
// once), exactly as the engine's TypeMatchups walk does without Foresight.
func TypeEffectiveness(rom []byte, moveType, def1, def2 uint8) (int, error) {
	chart, err := typeMatchups(rom)
	if err != nil {
		return 0, err
	}
	e := pairEffect(chart, moveType, def1)
	if def2 != def1 {
		e = e * pairEffect(chart, moveType, def2) / TypeNeutralEffect
	}
	return e, nil
}

func pairEffect(chart map[[2]uint8]int, move, def uint8) int {
	if v, ok := chart[[2]uint8{move, def}]; ok {
		return v
	}
	return TypeNeutralEffect
}

func typeMatchups(rom []byte) (map[[2]uint8]int, error) {
	if len(rom) == 0 {
		return nil, fmt.Errorf("gs/rom: empty ROM")
	}
	key := typeROMKey{data: uintptr(unsafe.Pointer(unsafe.SliceData(rom))), n: len(rom)}
	if len(rom) >= 0x150 {
		copy(key.header[:], rom[0x134:0x150])
	}
	if v, ok := typeMatchupCache.Load(key); ok {
		return v.(map[[2]uint8]int), nil
	}
	at, err := locateTypeMatchups(rom)
	if err != nil {
		return nil, err
	}
	chart := map[[2]uint8]int{}
	for off := at; off+typeMatchupEntry <= len(rom) && len(chart) < typeMatchupsMax; off += typeMatchupEntry {
		if rom[off] == typeMatchupsSplit {
			// Foresight rows follow; ordinary battles apply them too.
			off -= typeMatchupEntry - 1
			continue
		}
		if rom[off] == typeMatchupsEnd {
			typeMatchupCache.Store(key, chart)
			return chart, nil
		}
		chart[[2]uint8{rom[off], rom[off+1]}] = int(rom[off+2])
	}
	return nil, fmt.Errorf("gs/rom: TypeMatchups at %#x has no terminator", at)
}

// locateTypeMatchups finds the unique table that starts with the decomp's
// leading rows and whose entries are well-formed up to its terminator.
func locateTypeMatchups(rom []byte) (int, error) {
	found := -1
	for i := 0; i+typeMatchupsAnchor <= len(rom); i++ {
		if !bytesEqual(rom[i:i+typeMatchupsAnchor], typeMatchupsPrefix) || !wellFormedTypeMatchups(rom, i) {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("gs/rom: ambiguous TypeMatchups at %#x and %#x", found, i)
		}
		found = i
	}
	if found < 0 {
		return 0, fmt.Errorf("gs/rom: TypeMatchups table not found")
	}
	return found, nil
}

func wellFormedTypeMatchups(rom []byte, at int) bool {
	rows := 0
	for off := at; off < len(rom) && rows < typeMatchupsMax; {
		switch rom[off] {
		case typeMatchupsSplit:
			off++
			continue
		case typeMatchupsEnd:
			return rows > 64
		}
		if off+typeMatchupEntry > len(rom) {
			return false
		}
		switch rom[off+2] {
		case 0, 5, 20:
		default:
			return false
		}
		rows++
		off += typeMatchupEntry
	}
	return false
}
