package state

import "github.com/maestroi/pokepilot/red/sym"

const toggleableObjectListLen = 16*2 + 1

// HiddenObjectIDs returns the current map's object IDs whose global
// toggleable-object flag is set. Object IDs are 1-based indexes into the
// current map header's object list. Unlike sprite image state, this list is
// map-wide, so an off-screen object is not mistaken for a removed one.
func HiddenObjectIDs(m *Mem) map[uint8]bool {
	hidden := map[uint8]bool{}
	for offset := uint16(0); offset+1 < toggleableObjectListLen; offset += 2 {
		objectID := m.U8(sym.ToggleableObjectList + offset)
		if objectID == 0xff {
			break
		}
		flag := m.U8(sym.ToggleableObjectList + offset + 1)
		if m.U8(sym.ToggleableObjectFlags+uint16(flag)/8)&(1<<(flag%8)) != 0 {
			hidden[objectID] = true
		}
	}
	return hidden
}
