package rom

import (
	"fmt"

	gsdata "github.com/maestroi/pokepilot/gs/data"
)

const (
	numMapGroups        = 26
	mapHeaderLen        = 9
	mapAttrLen          = 12
	mapConnectionLen    = 12
	pokemonNameLen      = 10
	moveEntryLen        = 7
	itemAttrLen         = 7
	numMarts            = 34
	numMachines         = 57
	grassWildLen        = 2 + 3 + 7*2*3
	waterWildLen        = 2 + 1 + 3*2
	maxWildTables       = 8
	maxEvoRecordBytes   = 96
	objectEventLen      = 13
	warpEventLen        = 5
	coordEventLen       = 8
	bgEventLen          = 5
	objectSpriteYOffset = 4
	objectSpriteXOffset = 4
)

// Tables is the located Gold/Silver static-data layout for one ROM image.
type Tables struct {
	MapGroupPointers int
	PokemonNames     int
	Moves            int
	EvosAttacksPtrs  int
	ItemAttributes   int
	Marts            int
	TMHMMoves        int
	GrassWild        []int
	WaterWild        []int
	BaseData         int
}

// LocateTables finds every Gen 2 table the adapter needs. Locators fail
// closed on an unsupported or ambiguous image.
func LocateTables(rom []byte) (Tables, error) {
	var t Tables
	var err error
	if t.BaseData, err = LocateBaseData(rom); err != nil {
		return Tables{}, err
	}
	if t.PokemonNames, err = locatePokemonNames(rom); err != nil {
		return Tables{}, err
	}
	if t.MapGroupPointers, err = locateMapGroupPointers(rom); err != nil {
		return Tables{}, err
	}
	if t.Moves, err = locateMoves(rom); err != nil {
		return Tables{}, err
	}
	if t.EvosAttacksPtrs, err = locateEvosAttacks(rom); err != nil {
		return Tables{}, err
	}
	if t.ItemAttributes, err = locateItemAttributes(rom); err != nil {
		return Tables{}, err
	}
	if t.Marts, err = locateMarts(rom); err != nil {
		return Tables{}, err
	}
	if t.TMHMMoves, err = locateTMHMMoves(rom); err != nil {
		return Tables{}, err
	}
	if t.GrassWild, err = locateWildTables(rom, grassWildLen, true); err != nil {
		return Tables{}, err
	}
	if t.WaterWild, err = locateWildTables(rom, waterWildLen, false); err != nil {
		return Tables{}, err
	}
	return t, nil
}

func locatePokemonNames(rom []byte) (int, error) {
	// "BULBASAUR@" in the English font, then "IVYSAUR" padded to 10.
	want := append(padName("BULBASAUR"), padName("IVYSAUR")...)
	found := -1
	for i := 0; i+len(want) <= len(rom); i++ {
		if !bytesEqual(rom[i:i+len(want)], want) {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("gs/rom: ambiguous PokemonNames at %#x and %#x", found, i)
		}
		found = i
	}
	if found < 0 {
		return 0, fmt.Errorf("gs/rom: PokemonNames table not found")
	}
	return found, nil
}

func padName(s string) []byte {
	out := make([]byte, pokemonNameLen)
	for i := range out {
		out[i] = Terminator
	}
	copy(out, encodeEnglish(s))
	return out
}

func locateMapGroupPointers(rom []byte) (int, error) {
	groups := mapsByGroup()
	found := -1
	// scenes.asm precedes MapGroupPointers in SECTION "Maps", so the table
	// is not word-aligned in the file. Scan every offset.
	for off := 0; off+2*numMapGroups <= len(rom); off++ {
		if !plausibleGroupTable(rom, off) {
			continue
		}
		if !newBarkHeaderFingerprint(rom, off) {
			continue
		}
		if !mapGroupTableMatchesCatalog(rom, off, groups) {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("gs/rom: ambiguous MapGroupPointers at %#x and %#x", found, off)
		}
		found = off
	}
	if found < 0 {
		return 0, fmt.Errorf("gs/rom: MapGroupPointers not found")
	}
	return found, nil
}

func newBarkHeaderFingerprint(rom []byte, table int) bool {
	bank, _, err := offsetBankAddr(table)
	if err != nil {
		return false
	}
	ptr, err := readU16(rom, table+23*2) // group 24
	if err != nil {
		return false
	}
	base, err := bankedOffset(bank, ptr)
	if err != nil {
		return false
	}
	headerOff := base + 3*mapHeaderLen // NEW_BARK_TOWN is map 4
	if mustInROM(rom, headerOff, mapHeaderLen, "new bark header") != nil {
		return false
	}
	attrOff, err := bankedOffset(rom[headerOff], uint16(rom[headerOff+3])|uint16(rom[headerOff+4])<<8)
	if err != nil || mustInROM(rom, attrOff, mapAttrLen, "new bark attr") != nil {
		return false
	}
	return rom[attrOff+1] == 9 && rom[attrOff+2] == 10 && rom[attrOff+11] == connWest|connEast
}

func plausibleGroupTable(rom []byte, off int) bool {
	bank, _, err := offsetBankAddr(off)
	if err != nil || bank == 0 {
		return false
	}
	for i := 0; i < numMapGroups; i++ {
		addr := uint16(rom[off+2*i]) | uint16(rom[off+2*i+1])<<8
		if addr < romWindowAddr || addr >= romWindowEnd {
			return false
		}
	}
	return true
}

func mapGroupTableMatchesCatalog(rom []byte, table int, groups map[uint8][]gsdata.MapInfo) bool {
	bank, _, err := offsetBankAddr(table)
	if err != nil {
		return false
	}
	for group := uint8(1); group <= numMapGroups; group++ {
		maps := groups[group]
		if len(maps) == 0 {
			return false
		}
		ptr, err := readU16(rom, table+int(group-1)*2)
		if err != nil {
			return false
		}
		base, err := bankedOffset(bank, ptr)
		if err != nil {
			return false
		}
		for i, info := range maps {
			headerOff := base + i*mapHeaderLen
			if mustInROM(rom, headerOff, mapHeaderLen, "map header") != nil {
				return false
			}
			attrBank := rom[headerOff]
			attrAddr := uint16(rom[headerOff+3]) | uint16(rom[headerOff+4])<<8
			attrOff, err := bankedOffset(attrBank, attrAddr)
			if err != nil || mustInROM(rom, attrOff, mapAttrLen, "map attributes") != nil {
				return false
			}
			if rom[attrOff+1] != info.HeightBlocks || rom[attrOff+2] != info.WidthBlocks {
				return false
			}
		}
	}
	return true
}

func mapsByGroup() map[uint8][]gsdata.MapInfo {
	out := make(map[uint8][]gsdata.MapInfo)
	for _, info := range gsdata.Maps() {
		out[info.Group] = append(out[info.Group], info)
	}
	return out
}

func locateMoves(rom []byte) (int, error) {
	// Pound: anim 1, effect 0, power 40, type NORMAL, acc 100%, PP 35, chance 0.
	pound := []byte{0x01, 0x00, 40, 0x00, 0xff, 35, 0x00}
	found := -1
	for i := 0; i+moveEntryLen <= len(rom); i++ {
		if !bytesEqual(rom[i:i+moveEntryLen], pound) {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("gs/rom: ambiguous Moves at %#x and %#x", found, i)
		}
		found = i
	}
	if found < 0 {
		return 0, fmt.Errorf("gs/rom: Moves table not found")
	}
	return found, nil
}

func locateEvosAttacks(rom []byte) (int, error) {
	found := -1
	// The EvosAttacks section is packed after other ROMX data, so the
	// pointer table is not word-aligned in the file.
	for off := 0; off+2*gen2PokemonCount <= len(rom); off++ {
		bank, _, err := offsetBankAddr(off)
		if err != nil || bank == 0 {
			continue
		}
		first, err := readU16(rom, off)
		if err != nil || first < romWindowAddr || first >= romWindowEnd {
			continue
		}
		rec, err := bankedOffset(bank, first)
		if err != nil || rec+4 > len(rom) {
			continue
		}
		// Bulbasaur: EVOLVE_LEVEL, 16, IVYSAUR, 0
		if rom[rec] != EvoLevel || rom[rec+1] != 16 || rom[rec+2] != 2 || rom[rec+3] != 0 {
			continue
		}
		if !plausibleEvosTable(rom, off, bank) {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("gs/rom: ambiguous EvosAttacksPointers at %#x and %#x", found, off)
		}
		found = off
	}
	if found < 0 {
		return 0, fmt.Errorf("gs/rom: EvosAttacksPointers not found")
	}
	return found, nil
}

func plausibleEvosTable(rom []byte, off int, bank uint8) bool {
	seen := 0
	for species := 1; species <= gen2PokemonCount; species++ {
		addr, err := readU16(rom, off+(species-1)*2)
		if err != nil || addr < romWindowAddr || addr >= romWindowEnd {
			return false
		}
		rec, err := bankedOffset(bank, addr)
		if err != nil || rec >= len(rom) {
			return false
		}
		seen++
	}
	return seen == gen2PokemonCount
}

func locateItemAttributes(rom []byte) (int, error) {
	// Item ids are 1-based. Entry 0 is unused; MASTER_BALL (id 1) is first.
	// ULTRA_BALL (id 2) costs 1200; POKE_BALL (id 5) costs 200.
	found := -1
	for i := 0; i+6*itemAttrLen <= len(rom); i++ {
		if itemPrice(rom, i+1*itemAttrLen) != 1200 {
			continue
		}
		if itemPrice(rom, i+4*itemAttrLen) != 200 {
			continue
		}
		if itemPrice(rom, i+0*itemAttrLen) != 0 {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("gs/rom: ambiguous ItemAttributes at %#x and %#x", found, i)
		}
		found = i
	}
	if found < 0 {
		return 0, fmt.Errorf("gs/rom: ItemAttributes not found")
	}
	return found, nil
}

func itemPrice(rom []byte, off int) int {
	if off+2 > len(rom) {
		return -1
	}
	return int(rom[off]) | int(rom[off+1])<<8
}

func locateMarts(rom []byte) (int, error) {
	found := -1
	for off := 0; off+2*numMarts <= len(rom); off += 2 {
		bank, _, err := offsetBankAddr(off)
		if err != nil || bank == 0 {
			continue
		}
		first, err := readU16(rom, off)
		if err != nil || first < romWindowAddr || first >= romWindowEnd {
			continue
		}
		rec, err := bankedOffset(bank, first)
		if err != nil || rec+6 > len(rom) {
			continue
		}
		// MartCherrygrove: 4 items, Potion, Antidote, Parlyz Heal, Awakening, -1
		if rom[rec] != 4 || rom[rec+1] != 0x12 || rom[rec+2] != 0x09 ||
			rom[rec+3] != 0x0d || rom[rec+4] != 0x0c || rom[rec+5] != 0xff {
			continue
		}
		if !plausibleMartTable(rom, off, bank) {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("gs/rom: ambiguous Marts at %#x and %#x", found, off)
		}
		found = off
	}
	if found < 0 {
		return 0, fmt.Errorf("gs/rom: Marts table not found")
	}
	return found, nil
}

func plausibleMartTable(rom []byte, off int, bank uint8) bool {
	for i := 0; i < numMarts; i++ {
		addr, err := readU16(rom, off+i*2)
		if err != nil || addr < romWindowAddr || addr >= romWindowEnd {
			return false
		}
		rec, err := bankedOffset(bank, addr)
		if err != nil || rec >= len(rom) {
			return false
		}
		count := int(rom[rec])
		if count > 16 || rec+1+count >= len(rom) || rom[rec+1+count] != 0xff {
			return false
		}
	}
	return true
}

func locateTMHMMoves(rom []byte) (int, error) {
	// TM01 DynamicPunch ($df) … HM07 Waterfall ($7f), then 0.
	found := -1
	for i := 0; i+numMachines+1 <= len(rom); i++ {
		if rom[i] != 0xdf || rom[i+50] != 0x0f || rom[i+55] != 0xfa || rom[i+56] != 0x7f || rom[i+57] != 0 {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("gs/rom: ambiguous TMHMMoves at %#x and %#x", found, i)
		}
		found = i
	}
	if found < 0 {
		return 0, fmt.Errorf("gs/rom: TMHMMoves not found")
	}
	return found, nil
}

func locateWildTables(rom []byte, recLen int, grass bool) ([]int, error) {
	var starts []int
	for off := 0; off+recLen+1 <= len(rom); {
		if !looksLikeWildRecord(rom, off, recLen, grass) {
			off++
			continue
		}
		start := off
		n := 0
		for off+recLen <= len(rom) && looksLikeWildRecord(rom, off, recLen, grass) {
			off += recLen
			n++
		}
		if n >= 8 && off < len(rom) && rom[off] == 0xff {
			starts = append(starts, start)
			if len(starts) > maxWildTables {
				return nil, fmt.Errorf("gs/rom: too many %s wild tables", wildKind(grass))
			}
			off++
			continue
		}
		off = start + 1
	}
	if len(starts) == 0 {
		return nil, fmt.Errorf("gs/rom: %s wild tables not found", wildKind(grass))
	}
	return starts, nil
}

func wildKind(grass bool) string {
	if grass {
		return "grass"
	}
	return "water"
}

func looksLikeWildRecord(rom []byte, off, recLen int, grass bool) bool {
	if off+recLen > len(rom) {
		return false
	}
	group, number := rom[off], rom[off+1]
	if _, ok := gsdata.Map(gsdata.NativeMapID(group, number)); !ok {
		return false
	}
	if grass {
		if rom[off+2] == 0 && rom[off+3] == 0 && rom[off+4] == 0 {
			return false
		}
		for i := 0; i < 21; i++ {
			species := rom[off+5+i*2+1]
			if species == 0 || int(species) > gen2PokemonCount {
				return false
			}
		}
		return true
	}
	for i := 0; i < 3; i++ {
		species := rom[off+3+i*2+1]
		if species == 0 || int(species) > gen2PokemonCount {
			return false
		}
	}
	return true
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
