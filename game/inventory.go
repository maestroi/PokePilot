package game

// InventoryItem is one item stack using the game's native item identifier.
type InventoryItem struct {
	NativeItemID uint16
	Quantity     int
}

// InventoryState is the portable runtime view of the player's carried items.
type InventoryState struct {
	Items []InventoryItem
	Money uint32
}

// InventoryDecoder hides game-specific bag layout and item-id width.
type InventoryDecoder interface {
	DecodeInventory(MemoryReader) InventoryState
}

// InventoryProfile is a game profile that exposes portable carried inventory.
type InventoryProfile interface {
	GameProfile
	InventoryDecoder
}
