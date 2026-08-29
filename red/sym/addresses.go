package sym

// Expected ROM identity.
const ROMSHA1 = "ea9bcae617fdf159b045185467ae58b2e4a48b9a"
const ROMTitle = "POKEMON RED"

// Player and world
const (
	CurMap                  uint16 = 0xD35E
	YCoord                  uint16 = 0xD361
	XCoord                  uint16 = 0xD362
	YBlockCoord             uint16 = 0xD363
	XBlockCoord             uint16 = 0xD364
	CurMapTileset           uint16 = 0xD367
	CurMapHeight            uint16 = 0xD368
	CurMapWidth             uint16 = 0xD369
	PlayerMovingDirection   uint16 = 0xD528
	PlayerLastStopDirection uint16 = 0xD529
	PlayerDirection         uint16 = 0xD52A
	WalkCounter             uint16 = 0xCFC5
	PlayerName              uint16 = 0xD158
	SpritePlayerStateData1  uint16 = 0xC100
	SpritePlayerFacing      uint16 = 0xC109 // wSpritePlayerStateData1 + 9, SPRITE_FACING_* values
	SpriteStateData2        uint16 = 0xC200
)

// Party. Mons are a packed array: PartyMon1 + n*PartyMonSize.
const (
	PartyCount   uint16 = 0xD163
	PartySpecies uint16 = 0xD164
	PartyMon1    uint16 = 0xD16B
	PartyMon1HP  uint16 = 0xD16C // wPartyMon1HP
	PartyMonSize uint16 = 0x2C   // 44 bytes; wPartyMon2 is at 0xD197
)

// Offsets within one party mon struct.
const (
	MonSpecies uint16 = 0x00
	MonHP      uint16 = 0x01 // 2 bytes, BIG-endian
	MonStatus  uint16 = 0x04
	MonMoves   uint16 = 0x08 // 4 bytes
	MonPP      uint16 = 0x1D // 4 bytes
	MonLevel   uint16 = 0x21
	MonMaxHP   uint16 = 0x22 // 2 bytes, BIG-endian
)

// Inventory and progress
const (
	NumBagItems    uint16 = 0xD31D
	BagItems       uint16 = 0xD31E
	PlayerMoney    uint16 = 0xD347 // 3 bytes, binary-coded decimal
	ObtainedBadges uint16 = 0xD356
	EventFlags     uint16 = 0xD747
	StatusFlags4   uint16 = 0xD72E // wStatusFlags4
)

// Battle
const (
	IsInBattle      uint16 = 0xD057
	EnemyMonSpecies uint16 = 0xCFE5
	EnemyMonHP      uint16 = 0xCFE6
	BattleMonHP     uint16 = 0xD015
	BattleMonLevel  uint16 = 0xD022

	BattleResult uint16 = 0xCF0B // wBattleResult

	// Stat stages. Gen 1 stores them biased: 7 is neutral, 1 is -6 and 13 is
	// +6. A move like GROWL that lowers our Attack shows up here and nowhere
	// else, so a move policy that ignores these cannot tell it is being
	// ground down.
	PlayerMonAttackMod  uint16 = 0xCD1A // wPlayerMonAttackMod
	PlayerMonDefenseMod uint16 = 0xCD1B // wPlayerMonDefenseMod
	EnemyMonAttackMod   uint16 = 0xCD2E // wEnemyMonAttackMod
	EnemyMonDefenseMod  uint16 = 0xCD2F // wEnemyMonDefenseMod
	BattleMonSpecies    uint16 = 0xD014 // wBattleMonSpecies
	BattleMonMoves      uint16 = 0xD01C // wBattleMonMoves: 4 bytes, move ids, 0 = empty slot
	BattleMonMaxHP      uint16 = 0xD023 // wBattleMonMaxHP
	BattleMonPP         uint16 = 0xD02D // wBattleMonPP: 4 bytes, parallel to wBattleMonMoves
	EnemyMonLevel       uint16 = 0xCFF3 // wEnemyMonLevel
	EnemyMonMaxHP       uint16 = 0xCFF4 // wEnemyMonMaxHP
)

// Menus, text and input state
const (
	TopMenuItemY    uint16 = 0xCC24
	TopMenuItemX    uint16 = 0xCC25
	CurrentMenuItem uint16 = 0xCC26
	// PlayerMonNumber is wPlayerMonNumber: the party slot that is currently
	// out in battle (InitBattleVariables zeroes it; SwitchPlayerMon and
	// ChooseNextMon write it).
	PlayerMonNumber uint16 = 0xCC2F
	MaxMenuItem     uint16 = 0xCC28
	// ListScrollOffset is wListScrollOffset, the list menu's scroll offset:
	// the selected bag entry is ListScrollOffset + CurrentMenuItem.
	ListScrollOffset uint16 = 0xCC36
	MenuWatchedKeys uint16 = 0xCC29
	ListMenuID      uint16 = 0xCF94
	TextBoxID       uint16 = 0xD125
	// TileMap is wTileMap, the WRAM shadow of the background tilemap
	// (pokered.sym: 00:c3a0). 20x18 tiles; while a text box is up its rows
	// hold font tile IDs, which are the same values charmap.asm uses.
	TileMap    uint16 = 0xC3A0
	TileMapLen        = 20 * 18
	FontLoaded uint16 = 0xCFC4
	JoyIgnore  uint16 = 0xCD6B
)

// Shop / mart. The buy flow (pokered/engine/events/pokemart.asm) loads the
// clerk's stock from the ROM into wItemList and works through a sequence of
// menus; these are the live values that flow drives and verifies against.
const (
	// ItemList is wItemList: the clerk's stock copied from the ROM by
	// LoadItemList, item ids in menu order, $ff-terminated. 16 bytes.
	ItemList uint16 = 0xCF7B
	// ItemQuantity is wItemQuantity, the quantity selector's scrolling number
	// (DisplayChooseQuantityMenu): 1..MaxItemQuantity, UP increments, DOWN
	// decrements. It is NOT wCurrentMenuItem, so SelectMenuItem cannot drive it.
	ItemQuantity uint16 = 0xCF96
	// MaxItemQuantity is wMaxItemQuantity: 99 on the buy path.
	MaxItemQuantity uint16 = 0xCF97
	// ChosenMenuItem and MenuExitMethod are written by each menu on exit:
	// ChosenMenuItem is the selected index, MenuExitMethod is CHOSE_MENU_ITEM,
	// CHOSE_SECOND_ITEM or CANCELLED_MENU.
	ChosenMenuItem uint16 = 0xD12D
	MenuExitMethod uint16 = 0xD12E
)

// HRAM. Money (hMoney) is the total price the quantity selector shows: the
// buy path multiplies the item price by the quantity into these 3 BCD bytes.
const (
	Money uint16 = 0xFF9F // hMoney, 3 bytes BCD
)

// HRAM joypad mirrors, in Gen 1 bit order:
// A=0x01 B=0x02 Select=0x04 Start=0x08 Right=0x10 Left=0x20 Up=0x40 Down=0x80
const (
	JoyLast     uint16 = 0xFFB1
	JoyReleased uint16 = 0xFFB2
	JoyPressed  uint16 = 0xFFB3
	JoyHeld     uint16 = 0xFFB4
	JoyInput    uint16 = 0xFFF8
)
