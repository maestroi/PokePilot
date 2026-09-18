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

	// Battle.
	IsInBattle uint16 = 0xD056 // wIsInBattle

	// Inventory/progress.
	NumBagItems    uint16 = 0xD31C // wNumBagItems
	BagItems       uint16 = 0xD31D // first item id in wBagItems
	PlayerMoney    uint16 = 0xD346 // wPlayerMoney, 3-byte BCD
	ObtainedBadges uint16 = 0xD355 // wObtainedBadges

	// Persistent/story state. Yellow's main-data block is shifted one byte
	// earlier than Red in this region.
	StatusFlags1   uint16 = 0xD727 // wStatusFlags1
	StatusFlags4   uint16 = 0xD72D // wStatusFlags4
	Elite4Flags    uint16 = 0xD733 // wElite4Flags
	EventFlags     uint16 = 0xD746 // wEventFlags
	RivalStarter   uint16 = 0xD714 // wRivalStarter: 1=Jolteon, 2=Flareon, 3=Vaporeon path
	PlayerStarter  uint16 = 0xD716 // wPlayerStarter
	LastBlackoutMap uint16 = 0xD718 // wLastBlackoutMap

	// Yellow-only Pikachu state.
	PikachuHappiness       uint16 = 0xD46F // wPikachuHappiness
	PikachuMood            uint16 = 0xD470 // wPikachuMood
	PikachuSpawnStateFlags uint16 = 0xD471 // wPikachuSpawnStateFlags
)
