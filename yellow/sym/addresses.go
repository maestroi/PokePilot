// Package sym owns RAM and ROM identity for the supported Pokémon Yellow revision.
package sym

// Expected ROM identity.
//
// This is the English USA/Europe Pokémon Yellow image built by pret/pokeyellow.
// Keep detection hash-exact: a title match alone is not enough to prove a RAM
// layout is compatible with these addresses.
const (
	ROMSHA1  = "cc7d03262ebfaf2f06772c1a480c7d9d5f4a38e1"
	ROMTitle = "POKEMON YELLOW"
)

// Phase-0 baseline symbols.
//
// Yellow is similar to Red/Blue but it is not layout-identical. In particular,
// the party/inventory/player-world block is shifted and wIsInBattle is at a
// different address. Keep these constants game-owned instead of deriving them
// from red/sym offsets.
//
// Later Yellow phases should extend this table from the vendored pokeyellow
// decomp rather than importing red/sym.
const (
	// Player/world.
	CurMap             uint16 = 0xD35D // wCurMap
	YCoord             uint16 = 0xD360 // wYCoord
	XCoord             uint16 = 0xD361 // wXCoord
	CurMapHeight       uint16 = 0xD367 // wCurMapHeight
	CurMapWidth        uint16 = 0xD368 // wCurMapWidth
	PlayerName         uint16 = 0xD157 // wPlayerName
	RivalName          uint16 = 0xD349 // wRivalName
	SpritePlayerFacing uint16 = 0xC109 // wSpritePlayerStateData1 + 9

	// Party. PartyMon1 is a packed 44-byte Gen-I party-mon struct.
	PartyCount   uint16 = 0xD162 // wPartyCount
	PartyMon1    uint16 = 0xD16A // wPartyMons / first party mon
	PartyMonSize uint16 = 0x2C

	// Menus/text/control. These stay profile-owned even though their semantics
	// are shared with Red/Blue.
	TileMap         uint16 = 0xC3A0 // wTileMap
	TileMapLen             = 20 * 18
	CurrentMenuItem uint16 = 0xCC26 // wCurrentMenuItem
	MaxMenuItem     uint16 = 0xCC28 // wMaxMenuItem
	FontLoaded      uint16 = 0xCFC3 // wFontLoaded
	WalkCounter     uint16 = 0xCFC4 // wWalkCounter
	JoyIgnore       uint16 = 0xCD6B // wJoyIgnore

	// Battle/menu runtime. These addresses are from the generated
	// pokeyellow.sym for the exact supported ROM revision above. The battle
	// struct is shifted relative to Pokémon Red, so keep every native address
	// Yellow-owned even where the byte format is shared.
	TopMenuItemX           uint16 = 0xCC25 // wTopMenuItemX
	TopMenuItemY           uint16 = 0xCC24 // wTopMenuItemY
	MenuJoypadPollCount    uint16 = 0xCC34 // wMenuJoypadPollCount
	MenuWatchedKeys        uint16 = 0xCC29 // wMenuWatchedKeys
	ItemQuantity           uint16 = 0xCF95 // wItemQuantity
	MoneyTemp              uint16 = 0xFF9F // hMoney, 3-byte BCD menu price/total
	ListScrollOffset       uint16 = 0xCC36 // wListScrollOffset
	ListCount              uint16 = 0xD129 // wListCount
	ListMenuID             uint16 = 0xCF93 // wListMenuID
	PartyMenuTypeOrMessage uint16 = 0xD07C // wPartyMenuTypeOrMessageID
	FieldMoves             uint16 = 0xCD3D // wFieldMoves
	RodResponse            uint16 = 0xCD3D // wRodResponse; aliases field-move scratch outside fishing
	FlyLocationsList       uint16 = 0xCD3E // wFlyLocationsList, NUM_CITY_MAPS entries
	DestinationMap         uint16 = 0xD719 // wDestinationMap
	ActionResult           uint16 = 0xCD6A // wActionResultOrTookBattleTurn
	TileInFrontOfPlayer    uint16 = 0xCFC5 // wTileInFrontOfPlayer
	WalkBikeSurfState      uint16 = 0xD6FF // wWalkBikeSurfState
	MapPalOffset           uint16 = 0xD35C // wMapPalOffset
	MoveMenuType           uint16 = 0xCCDB // wMoveMenuType
	NumMovesMinusOne       uint16 = 0xCD6C // wNumMovesMinusOne
	ForcePlayerToChooseMon uint16 = 0xD11E // wForcePlayerToChooseMon
	BattleMonSpecies       uint16 = 0xD013 // wBattleMonSpecies
	BattleMonHP            uint16 = 0xD014 // wBattleMonHP, big-endian
	BattleMonMoves         uint16 = 0xD01B // wBattleMonMoves, 4 move ids
	BattleMonLevel         uint16 = 0xD021 // wBattleMonLevel
	BattleMonMaxHP         uint16 = 0xD022 // wBattleMonMaxHP, big-endian
	BattleMonPP            uint16 = 0xD02C // wBattleMonPP, low 6 bits are PP
	EnemyMonSpecies        uint16 = 0xCFE4 // wEnemyMonSpecies
	EnemyMonHP             uint16 = 0xCFE5 // wEnemyMonHP, big-endian
	EnemyMonLevel          uint16 = 0xCFF2 // wEnemyMonLevel
	EnemyMonMaxHP          uint16 = 0xCFF3 // wEnemyMonMaxHP, big-endian
	BattleResult           uint16 = 0xCF0B // wBattleResult
	PlayerMonNumber        uint16 = 0xCC2F // wPlayerMonNumber
	WhichPokemon           uint16 = 0xCF91 // wWhichPokemon
	MoveNum                uint16 = 0xD0DF // wMoveNum
	PlayerMoveNum          uint16 = 0xCFD1 // wPlayerMoveNum
	PlayerSelectedMove     uint16 = 0xCCDC // wPlayerSelectedMove
	EnemyMoveNum           uint16 = 0xCFCB // wEnemyMoveNum
	TrainerClass           uint16 = 0xD030 // wTrainerClass
	TrainerNo              uint16 = 0xD05C // wTrainerNo
	BattleType             uint16 = 0xD059 // wBattleType
	CurOpponent            uint16 = 0xD058 // wCurOpponent
	EnemyMonPartyPos       uint16 = 0xCFE7 // wEnemyMonPartyPos
	IsInBattle             uint16 = 0xD056 // wIsInBattle

	// Inventory/progress.
	PokedexOwned   uint16 = 0xD2F6 // wPokedexOwned, 19 bytes
	PokedexSeen    uint16 = 0xD309 // wPokedexSeen, 19 bytes
	PokedexBytes          = 19
	NumBagItems    uint16 = 0xD31C // wNumBagItems
	BagItems       uint16 = 0xD31D // first item id in wBagItems
	PlayerMoney    uint16 = 0xD346 // wPlayerMoney, 3-byte BCD
	ObtainedBadges uint16 = 0xD355 // wObtainedBadges

	// Persistent/story state. Yellow's main-data block is shifted one byte
	// earlier than Red in this region.
	StatusFlags1    uint16 = 0xD727 // wStatusFlags1
	StatusFlags4    uint16 = 0xD72D // wStatusFlags4
	Elite4Flags     uint16 = 0xD733 // wElite4Flags
	EventFlags      uint16 = 0xD746 // wEventFlags
	RivalStarter    uint16 = 0xD714 // wRivalStarter: 1=Jolteon, 2=Flareon, 3=Vaporeon path
	PlayerStarter   uint16 = 0xD716 // wPlayerStarter
	LastBlackoutMap uint16 = 0xD718 // wLastBlackoutMap

	// Yellow-only Pikachu state.
	PikachuHappiness       uint16 = 0xD46F // wPikachuHappiness
	PikachuMood            uint16 = 0xD470 // wPikachuMood
	PikachuSpawnStateFlags uint16 = 0xD471 // wPikachuSpawnStateFlags
)
