package rom

import (
	"github.com/maestroi/pokepilot/gen1rom"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	textStartAsm     = 0x08 // text_asm: the rest of the entry is code
	itemGiftScanSpan = 48   // CheckEvent .. call GiveItem fits well inside
	eventFlagBytes   = 0x140
)

// ItemGiftActor is a person whose text_asm hands over an item once: it tests
// an event flag (CheckEvent EVENT_GOT_...) and then runs
// `lb bc, ITEM, qty; call GiveItem`. EventByte/EventMask are canonical RAM
// coordinates of that flag; it is set once the gift has been received.
type ItemGiftActor struct {
	X, Y      uint8
	Item      uint8
	EventByte uint16
	EventMask uint8
}

// ItemGiftActors finds the map's free-gift NPCs from their own script bytes,
// so no hand-maintained list of TM givers is needed. Trainers (gym leaders who
// hand over a TM after a battle) are excluded.
func ItemGiftActors(romData []byte, mapID uint8) ([]ItemGiftActor, error) {
	h, err := ParseMap(romData, mapID)
	if err != nil {
		return nil, err
	}
	layout := Tables(romData)
	// Only the map's own text table: alternate tables selected by map scripts
	// re-index objects, which would attribute one NPC's gift to another.
	table, err := bankedOffset(h.Bank, h.TextsAddr)
	if err != nil {
		return nil, nil
	}
	var out []ItemGiftActor
	for _, obj := range h.Objects {
		if obj.TextID == 0 || obj.TextID&0xc0 != 0 {
			continue
		}
		off, ok := textEntryOffset(romData, h.Bank, table, int(obj.TextID)-1)
		if !ok || romData[off] != textStartAsm {
			continue
		}
		if gift, ok := scanItemGift(romData, off+1, layout); ok {
			gift.X, gift.Y = obj.X, obj.Y
			out = append(out, gift)
		}
	}
	return out, nil
}

func textEntryOffset(romData []byte, mapBank uint8, table, index int) (int, bool) {
	at := table + index*2
	if index < 0 || at < 0 || at+2 > len(romData) {
		return 0, false
	}
	ptr := uint16(romData[at]) | uint16(romData[at+1])<<8
	bank := mapBank
	if ptr < 0x4000 {
		bank = 0
	}
	off, err := bankedOffset(bank, ptr)
	if err != nil || off < 0 || off >= len(romData) {
		return 0, false
	}
	return off, true
}

// scanItemGift matches the CheckEvent macro (`ld a,[wEventFlags+n]` then
// `bit b,a`, or with its carry form `add a` for bit 7 / (b+1)×`rrca`)
// followed by `ld bc, ITEM<<8|qty; call GiveItem` within a short window.
func scanItemGift(romData []byte, start int, layout gen1rom.TableLayout) (ItemGiftActor, bool) {
	if layout.GiveItem == 0 {
		return ItemGiftActor{}, false
	}
	end := min(start+itemGiftScanSpan, len(romData))
	var gift ItemGiftActor
	flag := false
	for i := start; i+6 <= end; i++ {
		b := romData[i:]
		if b[0] == 0xC3 || b[0] == 0xC9 {
			// jp/ret ends this text_asm (`jp TextScriptEnd`); scanning on would
			// read the next NPC's script. An operand byte matching only costs a
			// missed gift, never a wrong one.
			break
		}
		if !flag && b[0] == 0xFA {
			if bit, ok := checkEventBit(b[3:]); ok {
				if canon, mask, ok := eventFlagRef(layout, uint16(b[1])|uint16(b[2])<<8, bit); ok {
					gift.EventByte, gift.EventMask, flag = canon, mask, true
				}
			}
			continue
		}
		if flag && b[0] == 0x01 && b[3] == 0xCD && uint16(b[4])|uint16(b[5])<<8 == layout.GiveItem && b[2] != 0 {
			gift.Item = b[2]
			return gift, true
		}
	}
	return ItemGiftActor{}, false
}

func checkEventBit(code []byte) (uint8, bool) {
	switch {
	case len(code) >= 2 && code[0] == 0xCB && code[1]&0xC7 == 0x47: // bit b, a
		return (code[1] >> 3) & 7, true
	case len(code) >= 1 && code[0] == 0x87: // add a: bit 7 into carry
		return 7, true
	}
	n := 0
	for n < len(code) && n < 8 && code[n] == 0x0F { // rrca × (b+1)
		n++
	}
	if n >= 1 && n <= 7 {
		return uint8(n - 1), true
	}
	return 0, false
}

func eventFlagRef(layout gen1rom.TableLayout, addr uint16, bit uint8) (uint16, uint8, bool) {
	if layout.EventFlag != nil {
		return layout.EventFlag(addr, bit)
	}
	if addr < sym.EventFlags || addr >= sym.EventFlags+eventFlagBytes {
		return 0, 0, false
	}
	return addr, 1 << bit, true
}
