package game

// InventoryItem is one item stack using the game's native item identifier.
type InventoryItem struct {
	NativeItemID uint16
	Quantity     int
	// Pocket names the carried-item pocket that lists this item. Games with a
	// single item list leave it empty; list positions are pocket-relative.
	Pocket MachinePocket
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
