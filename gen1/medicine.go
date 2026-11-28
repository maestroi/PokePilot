package gen1

import "github.com/maestroi/pokepilot/game"

// Medicines is the Gen-I battle medicine table (pokered
// constants/item_constants.asm, engine/items/item_effects.asm), ordered by HP
// restored so the first sufficient heal minimizes waste.
var Medicines = []game.Medicine{
	{NativeItemID: 0x14, HP: 20},                                                // POTION
	{NativeItemID: 0x13, HP: 50},                                                // SUPER_POTION
	{NativeItemID: 0x3c, HP: 50},                                                // FRESH_WATER
	{NativeItemID: 0x3d, HP: 60},                                                // SODA_POP
	{NativeItemID: 0x3e, HP: 80},                                                // LEMONADE
	{NativeItemID: 0x12, HP: 200},                                               // HYPER_POTION
	{NativeItemID: 0x11, HP: game.MedicineFullHP},                               // MAX_POTION
	{NativeItemID: 0x10, HP: game.MedicineFullHP, Cures: game.MedicineCuresAny}, // FULL_RESTORE
	{NativeItemID: 0x0b, Cures: "poisoned"},                                     // ANTIDOTE
	{NativeItemID: 0x0c, Cures: "burned"},                                       // BURN_HEAL
	{NativeItemID: 0x0d, Cures: "frozen"},                                       // ICE_HEAL
	{NativeItemID: 0x0e, Cures: "asleep"},                                       // AWAKENING
	{NativeItemID: 0x0f, Cures: "paralyzed"},                                    // PARLYZ_HEAL
	{NativeItemID: 0x34, Cures: game.MedicineCuresAny},
	{NativeItemID: 0x35, RevivePercent: 50},  // REVIVE
	{NativeItemID: 0x36, RevivePercent: 100}, // MAX_REVIVE                          // FULL_HEAL
}
