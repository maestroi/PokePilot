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
	PartyMonSize uint16 = 0x2C // 44 bytes; wPartyMon2 is at 0xD197
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
)

// Battle
const (
	IsInBattle      uint16 = 0xD057
	EnemyMonSpecies uint16 = 0xCFE5
	EnemyMonHP      uint16 = 0xCFE6
	BattleMonHP     uint16 = 0xD015
	BattleMonLevel  uint16 = 0xD022
)

// Menus, text and input state
const (
	CurrentMenuItem uint16 = 0xCC26
	MaxMenuItem     uint16 = 0xCC28
	MenuWatchedKeys uint16 = 0xCC29
	TextBoxID       uint16 = 0xD125
	// TileMap is wTileMap, the WRAM shadow of the background tilemap
	// (pokered.sym: 00:c3a0). 20x18 tiles; while a text box is up its rows
	// hold font tile IDs, which are the same values charmap.asm uses.
	TileMap    uint16 = 0xC3A0
	TileMapLen        = 20 * 18
	FontLoaded uint16 = 0xCFC4
	JoyIgnore  uint16 = 0xCD6B
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
