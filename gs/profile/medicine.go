package profile

import "github.com/maestroi/pokepilot/game"

// gen2Medicines is the Gold/Silver battle medicine table (pokegold
// constants/item_constants.asm, data/items/attributes.asm), ordered by HP
// restored. Gen-II ids differ from Gen-I; never share this table.
var gen2Medicines = []game.Medicine{
	{NativeItemID: 0x12, HP: 20},                                                // POTION
	{NativeItemID: 0x11, HP: 50},                                                // SUPER_POTION
	{NativeItemID: 0x2e, HP: 50},                                                // FRESH_WATER
	{NativeItemID: 0x2f, HP: 60},                                                // SODA_POP
	{NativeItemID: 0x30, HP: 80},                                                // LEMONADE
	{NativeItemID: 0x10, HP: 200},                                               // HYPER_POTION
	{NativeItemID: 0x0f, HP: game.MedicineFullHP},                               // MAX_POTION
	{NativeItemID: 0x0e, HP: game.MedicineFullHP, Cures: game.MedicineCuresAny}, // FULL_RESTORE
	{NativeItemID: 0x09, Cures: "poisoned"},                                     // ANTIDOTE
	{NativeItemID: 0x0a, Cures: "burned"},                                       // BURN_HEAL
	{NativeItemID: 0x0b, Cures: "frozen"},                                       // ICE_HEAL
	{NativeItemID: 0x0c, Cures: "asleep"},                                       // AWAKENING
	{NativeItemID: 0x0d, Cures: "paralyzed"},                                    // PARLYZ_HEAL
	{NativeItemID: 0x26, Cures: game.MedicineCuresAny},                          // FULL_HEAL
}
