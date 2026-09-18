package gen1rom

import "fmt"

type TrainerLayout struct {
	PointerBank  uint8
	PointerAddr  uint16
	TrainerCount int
}

type TrainerMon struct {
	Level   uint8
	Species uint8
}

func TrainerParty(rom []byte, class, set uint8, layout TrainerLayout) ([]TrainerMon, error) {
	if class == 0 || int(class) > layout.TrainerCount || set == 0 {
		return nil, fmt.Errorf("trainer class/set %d/%d outside table", class, set)
	}
	base, err := BankedOffset(layout.PointerBank, layout.PointerAddr)
	if err != nil {
		return nil, err
	}
	p := base + (int(class)-1)*2
	if p+2 > len(rom) {
		return nil, fmt.Errorf("trainer class %d pointer at %#x exceeds ROM", class, p)
	}
	addr := uint16(rom[p]) | uint16(rom[p+1])<<8
	at, err := BankedOffset(layout.PointerBank, addr)
	if err != nil {
		return nil, err
	}
	for n := uint8(1); n <= set; n++ {
		party, next, err := readTrainerParty(rom, at)
		if err != nil {
			return nil, fmt.Errorf("trainer %d/%d: %w", class, n, err)
		}
		if n == set {
			return party, nil
		}
		at = next
	}
	return nil, fmt.Errorf("trainer class/set %d/%d not found", class, set)
}

func readTrainerParty(rom []byte, at int) ([]TrainerMon, int, error) {
	if at >= len(rom) {
		return nil, 0, fmt.Errorf("party at %#x exceeds ROM", at)
	}
	first := rom[at]
	at++
	var out []TrainerMon
	if first == 0xff {
		for {
			if at >= len(rom) {
				return nil, 0, fmt.Errorf("variable-level party exceeds ROM")
			}
			if rom[at] == 0 {
				return out, at + 1, nil
			}
			if at+2 > len(rom) {
				return nil, 0, fmt.Errorf("variable-level party pair exceeds ROM")
			}
			out = append(out, TrainerMon{Level: rom[at], Species: rom[at+1]})
			at += 2
		}
	}
	level := first
	for {
		if at >= len(rom) {
			return nil, 0, fmt.Errorf("uniform-level party exceeds ROM")
		}
		species := rom[at]
		at++
		if species == 0 {
			return out, at, nil
		}
		out = append(out, TrainerMon{Level: level, Species: species})
	}
}

const martScriptID = 0xfe

func MartItems(rom []byte, h MapHeader) ([]uint8, error) {
	_, items, ok := martClerk(rom, h)
	if !ok {
		return nil, fmt.Errorf("map %02x: no object carries a mart script", h.ID)
	}
	return items, nil
}

func MartClerkPosition(rom []byte, h MapHeader) (uint8, uint8, error) {
	obj, _, ok := martClerk(rom, h)
	if !ok {
		return 0, 0, fmt.Errorf("map %02x: no object carries a mart script", h.ID)
	}
	return obj.X, obj.Y, nil
}

func martClerk(rom []byte, h MapHeader) (Object, []uint8, bool) {
	for _, table := range textPointerTables(rom, h) {
		for _, obj := range h.Objects {
			if obj.TextID == 0 || obj.TextID&0xc0 != 0 {
				continue
			}
			if items, ok := martScriptAt(rom, table, int(obj.TextID)-1); ok {
				return obj, items, true
			}
		}
	}
	return Object{}, nil, false
}

func textPointerTables(rom []byte, h MapHeader) []int {
	var tables []int
	seen := map[int]bool{}
	add := func(off int) {
		if off >= 0 && off < len(rom) && !seen[off] {
			seen[off] = true
			tables = append(tables, off)
		}
	}
	if off, err := BankedOffset(h.Bank, h.TextsAddr); err == nil {
		add(off)
	}
	if h.ScriptAddr != 0 {
		if off, err := BankedOffset(h.Bank, h.ScriptAddr); err == nil {
			for _, table := range ldhlTables(rom, off, h.Bank) {
				add(table)
			}
		}
	}
	return tables
}

func ldhlTables(rom []byte, start int, bank uint8) []int {
	var tables []int
	seen := map[int]bool{}
	queue := []int{start}
	for len(queue) > 0 && len(seen) < 16 {
		at := queue[0]
		queue = queue[1:]
		if seen[at] || at < 0 || at >= len(rom) {
			continue
		}
		seen[at] = true
		limit := at + 128
		if limit > len(rom) {
			limit = len(rom)
		}
		for i := at; i+2 < limit; i++ {
			imm := uint16(rom[i+1]) | uint16(rom[i+2])<<8
			switch rom[i] {
			case 0x21:
				if off, err := BankedOffset(bank, imm); err == nil {
					tables = append(tables, off)
				}
				i += 2
			case 0xcd:
				if off, err := BankedOffset(bank, imm); err == nil {
					queue = append(queue, off)
				}
				i += 2
			}
		}
	}
	return tables
}

func martScriptAt(rom []byte, table, index int) ([]uint8, bool) {
	at := table + index*2
	if at+2 > len(rom) {
		return nil, false
	}
	ptr := uint16(rom[at]) | uint16(rom[at+1])<<8
	off, err := BankedOffset(0, ptr)
	if err != nil || off+2 >= len(rom) || rom[off] != martScriptID {
		return nil, false
	}
	n := int(rom[off+1])
	if n == 0 || off+2+n+1 > len(rom) || rom[off+2+n] != 0xff {
		return nil, false
	}
	items := append([]uint8(nil), rom[off+2:off+2+n]...)
	return items, true
}
