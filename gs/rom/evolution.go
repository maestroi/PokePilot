package rom

import "fmt"

// Evolution methods from pokegold constants/pokemon_data_constants.asm.
const (
	EvoLevel     uint8 = 1
	EvoItem      uint8 = 2
	EvoTrade     uint8 = 3
	EvoHappiness uint8 = 4
	EvoStat      uint8 = 5
)

// Happiness and Tyrogue stat triggers.
const (
	HappinessAnytime uint8 = 1
	HappinessMornDay uint8 = 2
	HappinessNite    uint8 = 3

	StatAtkGTDef uint8 = 1
	StatAtkLTDef uint8 = 2
	StatAtkEQDef uint8 = 3
)

// Evolution is one EvosAttacks evolution record. From and To are National
// Dex indexes. Item is the stone/held item; TimeOrStat holds the happiness
// time or Tyrogue ATK_*_DEF byte.
type Evolution struct {
	From       uint8
	To         uint8
	Method     uint8
	Level      uint8
	Item       uint8
	TimeOrStat uint8
}

// LevelUpMove is one learnset pair.
type LevelUpMove struct {
	Species uint8
	Level   uint8
	Move    uint8
}

// Evolutions walks EvosAttacksPointers in National Dex order.
func Evolutions(rom []byte) ([]Evolution, error) {
	tables, err := LocateTables(rom)
	if err != nil {
		return nil, err
	}
	return evolutionsAt(rom, tables)
}

func evolutionsAt(rom []byte, tables Tables) ([]Evolution, error) {
	bank, _, err := offsetBankAddr(tables.EvosAttacksPtrs)
	if err != nil {
		return nil, err
	}
	out := make([]Evolution, 0, 160)
	for species := 1; species <= gen2PokemonCount; species++ {
		addr, err := readU16(rom, tables.EvosAttacksPtrs+(species-1)*2)
		if err != nil {
			return nil, err
		}
		rec, err := bankedOffset(bank, addr)
		if err != nil {
			return nil, fmt.Errorf("gs/rom: EvosAttacks species %#02x: %w", species, err)
		}
		evos, _, err := readEvolutions(rom, rec, uint8(species))
		if err != nil {
			return nil, err
		}
		out = append(out, evos...)
	}
	return out, nil
}

// LevelUpMoves returns every learnset pair in National Dex order.
func LevelUpMoves(rom []byte) ([]LevelUpMove, error) {
	tables, err := LocateTables(rom)
	if err != nil {
		return nil, err
	}
	bank, _, err := offsetBankAddr(tables.EvosAttacksPtrs)
	if err != nil {
		return nil, err
	}
	out := make([]LevelUpMove, 0, 1024)
	for species := 1; species <= gen2PokemonCount; species++ {
		addr, err := readU16(rom, tables.EvosAttacksPtrs+(species-1)*2)
		if err != nil {
			return nil, err
		}
		rec, err := bankedOffset(bank, addr)
		if err != nil {
			return nil, err
		}
		_, next, err := readEvolutions(rom, rec, uint8(species))
		if err != nil {
			return nil, err
		}
		moves, err := readLearnset(rom, next, uint8(species))
		if err != nil {
			return nil, err
		}
		out = append(out, moves...)
	}
	return out, nil
}

func readEvolutions(rom []byte, off int, from uint8) ([]Evolution, int, error) {
	var out []Evolution
	for i := 0; i < maxEvoRecordBytes; i++ {
		if off >= len(rom) {
			return nil, 0, fmt.Errorf("gs/rom: EvosAttacks for species %#02x ran past ROM", from)
		}
		method := rom[off]
		if method == 0 {
			return out, off + 1, nil
		}
		var evo Evolution
		evo.From = from
		evo.Method = method
		switch method {
		case EvoLevel:
			if off+3 > len(rom) {
				return nil, 0, fmt.Errorf("gs/rom: truncated level evo for %#02x", from)
			}
			evo.Level = rom[off+1]
			evo.To = rom[off+2]
			off += 3
		case EvoItem, EvoTrade:
			if off+3 > len(rom) {
				return nil, 0, fmt.Errorf("gs/rom: truncated item/trade evo for %#02x", from)
			}
			evo.Item = rom[off+1]
			evo.To = rom[off+2]
			off += 3
		case EvoHappiness:
			if off+3 > len(rom) {
				return nil, 0, fmt.Errorf("gs/rom: truncated happiness evo for %#02x", from)
			}
			evo.TimeOrStat = rom[off+1]
			evo.To = rom[off+2]
			off += 3
		case EvoStat:
			if off+4 > len(rom) {
				return nil, 0, fmt.Errorf("gs/rom: truncated stat evo for %#02x", from)
			}
			evo.Level = rom[off+1]
			evo.TimeOrStat = rom[off+2]
			evo.To = rom[off+3]
			off += 4
		default:
			return nil, 0, fmt.Errorf("gs/rom: species %#02x has unknown evo method %#02x", from, method)
		}
		out = append(out, evo)
	}
	return nil, 0, fmt.Errorf("gs/rom: EvosAttacks for species %#02x exceeded bound", from)
}

func readLearnset(rom []byte, off int, species uint8) ([]LevelUpMove, error) {
	var out []LevelUpMove
	for i := 0; i < 64; i++ {
		if off >= len(rom) {
			return nil, fmt.Errorf("gs/rom: learnset for species %#02x ran past ROM", species)
		}
		level := rom[off]
		if level == 0 {
			return out, nil
		}
		if off+2 > len(rom) {
			return nil, fmt.Errorf("gs/rom: truncated learnset for %#02x", species)
		}
		out = append(out, LevelUpMove{Species: species, Level: level, Move: rom[off+1]})
		off += 2
	}
	return nil, fmt.Errorf("gs/rom: learnset for species %#02x exceeded bound", species)
}
