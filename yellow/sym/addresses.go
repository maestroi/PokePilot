// Package sym holds the Pokémon Yellow WRAM/HRAM symbol addresses.
//
// Yellow shares the Gen I engine *shape* with Red but not its layout: most
// addressables sit one byte earlier than their Red counterparts. Every value
// here was read from the vendored pokeyellow symbol map (pokeyellow.sym),
// which is generated from a tree that builds a ROM byte-identical to
// roms/pokemon_yellow.gb (sha1 cc7d03262ebfaf2f06772c1a480c7d9d5f4a38e1).
//
// Do not copy values from red/sym. Do not hand-count event flags: use
// yellow/state's replay of pokeyellow's const_def counter.
package sym

// ROMSHA1 identifies the supported English Yellow image. It matches
// pokeyellow/roms.sha1 and the ROM under roms/.
const ROMSHA1 = "cc7d03262ebfaf2f06772c1a480c7d9d5f4a38e1"

// Overworld position and current map. Yellow's wCurMap block is one byte
// lower than Red's; wJoyIgnore is the one shared address.
const (
	CurMap             uint16 = 0xD35D // wCurMap
	CurMapTileset      uint16 = 0xD366 // wCurMapTileset
	CurMapHeight       uint16 = 0xD367 // wCurMapHeight
	CurMapWidth        uint16 = 0xD368 // wCurMapWidth
	YCoord             uint16 = 0xD360 // wYCoord
	XCoord             uint16 = 0xD361 // wXCoord
	SpritePlayerFacing uint16 = 0xC109 // wSpritePlayerStateData1FacingDirection; shared with Red
	JoyIgnore          uint16 = 0xCD6B // wJoyIgnore; shared with Red
)

// Party. Mons are a packed array: PartyMon1 + n*PartyMonSize. The
// party_struct macro is byte-identical to Red's, so every field offset within
// a mon carries over; only the base addresses differ.
const (
	PartyCount   uint16 = 0xD162 // wPartyCount
	PartyMon1    uint16 = 0xD16A // wPartyMon1
	PartyMon1HP  uint16 = 0xD16B // wPartyMon1HP
	PartyMonSize uint16 = 0x2C   // 44 bytes; wPartyMon2 is at 0xD196

	// Offsets within one party mon. Shared with Red.
	MonSpecies uint16 = 0x00
	MonHP      uint16 = 0x01 // 2 bytes, BIG-endian
	MonStatus  uint16 = 0x04
	MonType1   uint16 = 0x05
	MonType2   uint16 = 0x06
	MonMoves   uint16 = 0x08 // 4 bytes
	MonPP      uint16 = 0x1D // 4 bytes
	MonLevel   uint16 = 0x21
	MonMaxHP   uint16 = 0x22 // 2 bytes, BIG-endian
	MonAttack  uint16 = 0x24 // 2 bytes, BIG-endian
	MonDefense uint16 = 0x26 // 2 bytes, BIG-endian
	MonSpeed   uint16 = 0x28 // 2 bytes, BIG-endian
	MonSpecial uint16 = 0x2A // 2 bytes, BIG-endian
)

// Inventory and money.
const (
	NumBagItems uint16 = 0xD31C // wNumBagItems
	BagItems    uint16 = 0xD31D // wBagItems
	PlayerMoney uint16 = 0xD346 // 3 bytes, binary-coded decimal
)

// Progress and story.
const (
	ObtainedBadges  uint16 = 0xD355 // wObtainedBadges
	EventFlags      uint16 = 0xD746 // wEventFlags; shared with Red
	StatusFlags4    uint16 = 0xD72D // wStatusFlags4
	LastBlackoutMap uint16 = 0xD718 // wLastBlackoutMap
)

// Battle. The battle_struct layout is shared with Red; every address differs.
const (
	IsInBattle   uint16 = 0xD056 // wIsInBattle
	BattleType   uint16 = 0xD059 // wBattleType
	BattleResult uint16 = 0xCF0B // wBattleResult; shared with Red

	// Stat stages. Shared with Red: biased so 7 is neutral.
	PlayerMonAttackMod  uint16 = 0xCD1A
	PlayerMonDefenseMod uint16 = 0xCD1B
	EnemyMonAttackMod   uint16 = 0xCD2E
	EnemyMonDefenseMod  uint16 = 0xCD2F

	// Enemy mon.
	EnemyMonSpecies uint16 = 0xCFE4 // wEnemyMonSpecies
	EnemyMonHP      uint16 = 0xCFE5 // wEnemyMonHP
	EnemyMonLevel   uint16 = 0xCFF2 // wEnemyMonLevel
	EnemyMonMaxHP   uint16 = 0xCFF3 // wEnemyMonMaxHP
	EnemyMonAttack  uint16 = 0xCFF5
	EnemyMonDefense uint16 = 0xCFF7
	EnemyMonSpecial uint16 = 0xCFFB
	EnemyMonType1   uint16 = 0xCFE9
	EnemyMonType2   uint16 = 0xCFEA
	EnemyPartyCount uint16 = 0xD89B

	// Active player mon.
	BattleMon          uint16 = 0xD013
	BattleMonSpecies   uint16 = 0xD013
	BattleMonHP        uint16 = 0xD014
	BattleMonLevel     uint16 = 0xD021
	BattleMonMaxHP     uint16 = 0xD022
	BattleMonAttack    uint16 = 0xD024
	BattleMonDefense   uint16 = 0xD026
	BattleMonSpecial   uint16 = 0xD02A
	BattleMonType1     uint16 = 0xD018
	BattleMonType2     uint16 = 0xD019
	BattleMonMoves     uint16 = 0xD01B // 4 bytes, move ids, 0 = empty slot
	BattleMonPP        uint16 = 0xD02C // 4 bytes, parallel to BattleMonMoves
	PlayerDisabledMove uint16 = 0xD06C
)

// Walking animation counter.
const (
	WalkCounter uint16 = 0xCFC4 // wWalkCounter
)

// Menus and text.
const (
	CurrentMenuItem  uint16 = 0xCC26 // wCurrentMenuItem; shared with Red
	MaxMenuItem      uint16 = 0xCC28 // wMaxMenuItem; shared with Red
	ListScrollOffset uint16 = 0xCC36 // wListScrollOffset; shared with Red
	FontLoaded       uint16 = 0xCFC3 // wFontLoaded
)

// Pikachu follower. Yellow-only: a trailing overworld sprite with its own
// state. The blocker model must account for it when it is spawned.
const (
	PikachuOverworldStateFlags uint16 = 0xD42F // wPikachuOverworldStateFlags
	PikachuSpawnState          uint16 = 0xD430 // wPikachuSpawnState
)

// Map ROM tables. Formats are shared with Red; only the addresses moved.
// See docs/POKEYELLOW.md.
const (
	MapHeaderPointersBank uint8  = 0x3F
	MapHeaderPointersAddr uint16 = 0x41F2
	MapHeaderBanksBank    uint8  = 0x3F
	MapHeaderBanksAddr    uint16 = 0x43E4
	TilesetsBank          uint8  = 0x03
	TilesetsAddr          uint16 = 0x4558 // Tilesets (pokeyellow.sym): 03:4558
)

// Remaining WRAM the skill layer reads. Every value is read from
// pokeyellow.sym; those that differ from Red are one byte lower because
// Yellow inserted a byte above this region and shifted it down. The shared
// ones (BoxMonCounts, FieldMoves, RodResponse, Sprite*, TileMap, the menu
// coords, OverworldMap) are spelled out anyway so the skill layer has one
// complete address set per game and never mixes the two.
const (
	BoxMonCounts            uint16 = 0xCD3D // wBoxMonCounts; shared
	FieldMoves              uint16 = 0xCD3D // wFieldMoves; shared
	RodResponse             uint16 = 0xCD3D // wRodResponse; shared
	SpritePlayerStateData1  uint16 = 0xC100 // wSpritePlayerStateData1; shared
	SpriteStateData2        uint16 = 0xC200 // wSpriteStateData2; shared
	TileMap                 uint16 = 0xC3A0 // wTileMap; shared
	OverworldMap            uint16 = 0xC6E8 // wOverworldMap; shared
	TopMenuItemY            uint16 = 0xCC24 // wTopMenuItemY; shared
	TopMenuItemX            uint16 = 0xCC25 // wTopMenuItemX; shared
	MenuWatchedKeys         uint16 = 0xCC29 // wMenuWatchedKeys; shared
	MoveMenuType            uint16 = 0xCCDB // wMoveMenuType; shared
	PlayerMonNumber         uint16 = 0xCC2F // wPlayerMonNumber; shared
	WhichPokemon            uint16 = 0xCF91 // wWhichPokemon
	ListMenuID              uint16 = 0xCF93 // wListMenuID
	ItemList                uint16 = 0xCF7A // wItemList
	ItemQuantity            uint16 = 0xCF95 // wItemQuantity
	MaxItemQuantity         uint16 = 0xCF96 // wMaxItemQuantity
	TextBoxID               uint16 = 0xD124 // wTextBoxID
	NumRunAttempts          uint16 = 0xD11F // wNumRunAttempts
	MoveNum                 uint16 = 0xD0DF // wMoveNum
	PlayerName              uint16 = 0xD157 // wPlayerName
	MapPalOffset            uint16 = 0xD35C // wMapPalOffset
	RivalName               uint16 = 0xD349 // wRivalName
	PartySpecies            uint16 = 0xD163 // wPartySpecies
	NumberOfWarps           uint16 = 0xD3AD // wNumberOfWarps
	WarpEntries             uint16 = 0xD3AE // wWarpEntries
	StatusFlags1            uint16 = 0xD727 // wStatusFlags1
	FirstLockTrashCanIndex  uint16 = 0xD742 // wFirstLockTrashCanIndex
	SecondLockTrashCanIndex uint16 = 0xD743 // wSecondLockTrashCanIndex
	WalkBikeSurfState       uint16 = 0xD6FF // wWalkBikeSurfState
	MtMoonB2FCurScript      uint16 = 0xD606 // wMtMoonB2FCurScript
	NumSafariBalls          uint16 = 0xDA46 // wNumSafariBalls
	TileInFrontOfPlayer     uint16 = 0xCFC5 // wTileInFrontOfPlayer
)

// Storage, Pokedex, coins and menus. Each is one byte lower than Red's
// counterpart (see the matching const in red/sym); verified against
// pokeyellow.sym.
const (
	CurrentBoxNum         uint16 = 0xD59F // wCurrentBoxNum; low 7 bits are the active box
	BoxCount              uint16 = 0xDA7F // wBoxCount
	BoxMon1               uint16 = 0xDA95 // wBoxMon1; BoxMonSize as in Red
	PokedexOwned          uint16 = 0xD2F6 // wPokedexOwned
	PokedexSeen           uint16 = 0xD309 // wPokedexSeen
	PlayerCoins           uint16 = 0xD5A3 // 2 bytes, binary-coded decimal
	TwoOptionMenuID       uint16 = 0xD12B // wTwoOptionMenuID; low 7 bits select the menu
	ToggleableObjectFlags uint16 = 0xD5A5 // 256-bit global hidden-object array
	ToggleableObjectList  uint16 = 0xD5CD // maps current-map object IDs to flags
)
