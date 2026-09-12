package rom

import "fmt"

// Evolution methods are the EvosMoves method bytes
// (pokered/constants/pokemon_data_constants.asm).
const (
	EvoLevel = 1
	EvoItem  = 2
	EvoTrade = 3
)

const (
	evosMovesBank     uint8  = 0x0E
	evosMovesAddr     uint16 = 0x705C
	evosMovesCount           = 190 // NUM_POKEMON_INDEXES
	maxEvoRecordBytes        = 64
)

// Evolution is one EvosMoves entry. From and To are Red internal species
// indexes. Item is set only for EvoItem; Level is the method's level byte
// (the stone-evo min level is 1 in stock Red).
type Evolution struct {
	From   uint8
	To     uint8
	Method uint8
	Level  uint8
	Item   uint8
}

// Evolutions walks EvosMovesPointerTable in internal-species order and
// returns every evolution the ROM declares. MissingNo slots with empty
// records contribute nothing. This is the only evolution chart Dex mode
// is allowed to use.
func Evolutions(romData []byte) ([]Evolution, error) {
	base, err := bankedOffset(evosMovesBank, evosMovesAddr)
	if err != nil {
		return nil, fmt.Errorf("rom: EvosMovesPointerTable: %w", err)
	}
	if base+2*evosMovesCount > len(romData) {
		return nil, fmt.Errorf("rom: EvosMovesPointerTable at %#x exceeds ROM of %d bytes", base, len(romData))
	}

	out := make([]Evolution, 0, 128)
	for species := 1; species <= evosMovesCount; species++ {
		pOff := base + (species-1)*2
		addr := uint16(romData[pOff]) | uint16(romData[pOff+1])<<8
		if addr == 0 {
			continue
		}
		rec, err := bankedOffset(evosMovesBank, addr)
		if err != nil {
			return nil, fmt.Errorf("rom: EvosMoves species %#02x: %w", species, err)
		}
		evos, err := readEvolutions(romData, rec, uint8(species))
		if err != nil {
			return nil, err
		}
		out = append(out, evos...)
	}
	return out, nil
}

func readEvolutions(romData []byte, off int, from uint8) ([]Evolution, error) {
	var out []Evolution
	for i := 0; i < maxEvoRecordBytes; i++ {
		if off >= len(romData) {
			return nil, fmt.Errorf("rom: EvosMoves for species %#02x ran past ROM", from)
		}
		method := romData[off]
		if method == 0 {
			return out, nil
		}
		switch method {
		case EvoLevel:
			if off+3 > len(romData) {
				return nil, fmt.Errorf("rom: EVOLVE_LEVEL for species %#02x is truncated", from)
			}
			out = append(out, Evolution{From: from, Method: method, Level: romData[off+1], To: romData[off+2]})
			off += 3
		case EvoItem:
			if off+4 > len(romData) {
				return nil, fmt.Errorf("rom: EVOLVE_ITEM for species %#02x is truncated", from)
			}
			out = append(out, Evolution{From: from, Method: method, Item: romData[off+1], Level: romData[off+2], To: romData[off+3]})
			off += 4
		case EvoTrade:
			if off+3 > len(romData) {
				return nil, fmt.Errorf("rom: EVOLVE_TRADE for species %#02x is truncated", from)
			}
			out = append(out, Evolution{From: from, Method: method, Level: romData[off+1], To: romData[off+2]})
			off += 3
		default:
			return nil, fmt.Errorf("rom: EvosMoves for species %#02x has unknown method %d", from, method)
		}
	}
	return nil, fmt.Errorf("rom: EvosMoves for species %#02x exceeded %d bytes without a terminator", from, maxEvoRecordBytes)
}
