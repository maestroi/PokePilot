package skill

// Gen 1 item IDs from pokered/constants/item_constants.asm. The automatic
// battle policy intentionally covers ordinary medicine only.
const (
	itemAntidote    uint8 = 0x0b
	itemBurnHeal    uint8 = 0x0c
	itemIceHeal     uint8 = 0x0d
	itemAwakening   uint8 = 0x0e
	itemParlyzHeal  uint8 = 0x0f
	itemFullRestore uint8 = 0x10
	itemMaxPotion   uint8 = 0x11
	itemHyperPotion uint8 = 0x12
	itemSuperPotion uint8 = 0x13
	itemPotion      uint8 = 0x14
	itemFullHeal    uint8 = 0x34
	itemFreshWater  uint8 = 0x3c
	itemSodaPop     uint8 = 0x3d
	itemLemonade    uint8 = 0x3e
)

// Even if an enemy repeatedly knocks the active mon back below the healing
// line, automatic medicine can consume only this many battle turns.
const battleItemUseCap = 4

type battleMedicineChoice struct {
	Item   uint8
	Slot   int
	Reason string
}

type hpMedicine struct {
	item uint8
	heal int
}

// Ordered by heal size so the first sufficient item minimizes waste. Full
// heals come last, which preserves them when an ordinary finite heal suffices.
var hpMedicines = []hpMedicine{
	{item: itemPotion, heal: 20},
	{item: itemSuperPotion, heal: 50},
	{item: itemFreshWater, heal: 50},
	{item: itemSodaPop, heal: 60},
	{item: itemLemonade, heal: 80},
	{item: itemHyperPotion, heal: 200},
	{item: itemMaxPotion, heal: 1 << 30},
	{item: itemFullRestore, heal: 1 << 30},
}
