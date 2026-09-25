package skill

import "github.com/maestroi/pokepilot/red/state"

// bagEntry is the temporary Gen-I compatibility seam for callers that still
// carry a Red state snapshot. New generic item execution uses inventoryEntry
// over game.InventoryState instead.
func bagEntry(mem *state.Mem, item uint8) (int, int) {
	for i, it := range state.DecodeInventory(mem).Items {
		if it.ID == item {
			return i, int(it.Quantity)
		}
	}
	return -1, 0
}
