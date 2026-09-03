package agent

// economyCatalog fills the observation vocabulary that pre-dates later
// progression slices. Observe uses ItemName while decoding both the bag and
// mart shelves; without these names, strategically important inventory simply
// disappears before the economy policy can classify it. These entries are
// observation-only: adding them to itemByID must not widen ItemByName, which is
// also the planner's executable argument whitelist.
func init() {
	catalog := []ItemEconomySpec{
		{Name: "master ball", ID: 0x01, Category: InventoryCapture},
		{Name: "town map", ID: 0x05, Category: InventoryProgressionCritical},
		{Name: "bicycle", ID: 0x06, Category: InventoryProgressionCritical},
		{Name: "safari ball", ID: 0x08, Category: InventoryCapture, UnitPrice: 1000},
		{Name: "pokedex", ID: 0x09, Category: InventoryProgressionCritical},
		{Name: "moon stone", ID: 0x0A, Category: InventoryEvolution},
		{Name: "old amber", ID: 0x1F, Category: InventoryProgressionCritical},
		{Name: "hp up", ID: 0x23, Category: InventoryOptionalUtility, UnitPrice: 9800},
		{Name: "protein", ID: 0x24, Category: InventoryOptionalUtility, UnitPrice: 9800},
		{Name: "iron", ID: 0x25, Category: InventoryOptionalUtility, UnitPrice: 9800},
		{Name: "carbos", ID: 0x26, Category: InventoryOptionalUtility, UnitPrice: 9800},
		{Name: "calcium", ID: 0x27, Category: InventoryOptionalUtility, UnitPrice: 9800},
		{Name: "rare candy", ID: 0x28, Category: InventoryOptionalUtility, UnitPrice: 4800},
		{Name: "dome fossil", ID: 0x29, Category: InventoryProgressionCritical},
		{Name: "helix fossil", ID: 0x2A, Category: InventoryProgressionCritical},
		{Name: "secret key", ID: 0x2B, Category: InventoryProgressionCritical},
		{Name: "bike voucher", ID: 0x2D, Category: InventoryProgressionCritical},
		{Name: "x accuracy", ID: 0x2E, Category: InventoryBattleConsumable, UnitPrice: 950},
		{Name: "card key", ID: 0x30, Category: InventoryProgressionCritical},
		{Name: "nugget", ID: 0x31, Category: InventoryOptionalUtility, UnitPrice: 10000},
		{Name: "coin", ID: 0x3B, Category: InventoryOptionalUtility, UnitPrice: 10},
		{Name: "fresh water", ID: 0x3C, Category: InventoryProgressionCritical, UnitPrice: 200},
		{Name: "soda pop", ID: 0x3D, Category: InventoryBattleConsumable, UnitPrice: 300},
		{Name: "lemonade", ID: 0x3E, Category: InventoryBattleConsumable, UnitPrice: 350},
		{Name: "s.s. ticket", ID: 0x3F, Category: InventoryProgressionCritical},
		{Name: "gold teeth", ID: 0x40, Category: InventoryProgressionCritical},
		{Name: "coin case", ID: 0x45, Category: InventoryProgressionCritical},
		{Name: "oaks parcel", ID: 0x46, Category: InventoryProgressionCritical},
		{Name: "itemfinder", ID: 0x47, Category: InventoryOptionalUtility},
		{Name: "lift key", ID: 0x4A, Category: InventoryProgressionCritical},
		{Name: "exp all", ID: 0x4B, Category: InventoryOptionalUtility},
		{Name: "old rod", ID: 0x4C, Category: InventoryProgressionCritical},
		{Name: "good rod", ID: 0x4D, Category: InventoryProgressionCritical},
		{Name: "super rod", ID: 0x4E, Category: InventoryProgressionCritical},
		{Name: "hm01", ID: 0xC4, Category: InventoryProgressionCritical},
		{Name: "hm02", ID: 0xC5, Category: InventoryProgressionCritical},
		{Name: "hm05", ID: 0xC8, Category: InventoryProgressionCritical},
	}
	for _, spec := range catalog {
		itemEconomy[spec.Name] = spec
		if _, exists := itemByID[spec.ID]; !exists {
			itemByID[spec.ID] = spec.Name
		}
	}
}
