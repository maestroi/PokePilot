// Package sym owns the verified retail Gold/Silver RAM layout used by the GS profile.
// Addresses correspond to the supported USA/Europe rev0 ROMs and pret/pokegold
// 0f087a51e36cbd38f33e5055754614578246ceff.
package sym

const (
	GoldSHA1   = "d8b8a3600a465308c9953dfa04f0081c05bdcb94"
	SilverSHA1 = "49b163f7e57702bc939d642a18f591de55d92dae"
	ROMSize    = 2 * 1024 * 1024
	WRAMBank   = 1

	// Fixed WRAM used by overworld rendering/movement.
	OverworldMap    uint16 = 0xC700 // wOverworldMapBlocks
	OverworldMapLen        = 1300
	PlayerStepFlags uint16 = 0xCE85 // wPlayerStepFlags

	// Shared menu / intro state in fixed WRAM.
	WindowStackPointer     uint16 = 0xCEA8 // wWindowStackPointer
	MenuJoypad             uint16 = 0xCEAA // wMenuJoypad
	MenuSelection          uint16 = 0xCEAB // wMenuSelection
	WhichIndexSet          uint16 = 0xCEAD // wWhichIndexSet
	TwoDMenuCursorInitY    uint16 = 0xCED8 // w2DMenuCursorInitY
	TwoDMenuCursorInitX    uint16 = 0xCED9 // w2DMenuCursorInitX
	TwoDMenuNumRows        uint16 = 0xCEDA // w2DMenuNumRows
	TwoDMenuNumCols        uint16 = 0xCEDB // w2DMenuNumCols
	MenuJoypadFilter       uint16 = 0xCEDF // wMenuJoypadFilter
	MenuCursorY            uint16 = 0xCEE0 // wMenuCursorY
	MenuCursorX            uint16 = 0xCEE1 // wMenuCursorX
	JumptableIndex         uint16 = 0xCE63 // wJumptableIndex
	TitleScreenSelected    uint16 = 0xCE64 // wTitleScreenSelectedOption
	TitleScreenTimer       uint16 = 0xCE65 // wTitleScreenTimer
	NamingScreenDestination uint16 = 0xC5D0 // wNamingScreenDestinationPointer
	NamingScreenCurNameLen  uint16 = 0xC5D2 // wNamingScreenCurNameLength
	NamingScreenMaxNameLen  uint16 = 0xC5D3 // wNamingScreenMaxNameLength
	NamingScreenType        uint16 = 0xC5D4 // wNamingScreenType
	TimeSetBuffer          uint16 = 0xC508 // wTimeSetBuffer
	InitHourBuffer         uint16 = 0xC51C // wInitHourBuffer
	InitMinuteBuffer       uint16 = 0xC526 // wInitMinuteBuffer
	StringBuffer2          uint16 = 0xCF7E // wStringBuffer2

	// Bank-1 live map/script state.
	MapHeight       uint16 = 0xD087 // wMapHeight
	MapWidth        uint16 = 0xD088 // wMapWidth
	MapStatus       uint16 = 0xD159 // wMapStatus
	MapEventStatus  uint16 = 0xD15A // wMapEventStatus
	ScriptFlags     uint16 = 0xD15B // wScriptFlags
	ScriptMode      uint16 = 0xD15E // wScriptMode
	ScriptRunning   uint16 = 0xD15F // wScriptRunning
	PlayerDirection uint16 = 0xD205 // wPlayerDirection

	ObjectStructs    uint16 = 0xD1FD // wObjectStructs
	ObjectStructLen         = 0x28
	NumObjectStructs        = 13
	PlayerState      uint16 = 0xD682 // wPlayerState

	// Early Johto story state.
	StatusFlags            uint16 = 0xD571 // wStatusFlags
	ElmsLabSceneID         uint16 = 0xD6CC // wElmsLabSceneID
	Route29SceneID         uint16 = 0xD6CE // wRoute29SceneID
	CherrygroveCitySceneID uint16 = 0xD6CF // wCherrygroveCitySceneID
	MrPokemonsHouseSceneID uint16 = 0xD6D0 // wMrPokemonsHouseSceneID
	EventFlags             uint16 = 0xD7B7 // wEventFlags

	// Fresh-game / RTC state.
	Options         uint16 = 0xD199 // wOptions
	SaveFileExists  uint16 = 0xD19A // wSaveFileExists
	PlayerName      uint16 = 0xD1A3 // wPlayerName
	PlayerNameLen          = 11
	RivalName       uint16 = 0xD1B9 // wRivalName
	RivalNameLen           = 11
	StartDay        uint16 = 0xD1DC // wStartDay
	StartHour       uint16 = 0xD1DD // wStartHour
	StartMinute     uint16 = 0xD1DE // wStartMinute
	StartSecond     uint16 = 0xD1DF // wStartSecond
	CurDay          uint16 = 0xD1F2 // wCurDay
	GameTimerPaused uint16 = 0xD8B8 // wGameTimerPaused

	MapGroup  uint16 = 0xDA00
	MapNumber uint16 = 0xDA01
	YCoord    uint16 = 0xDA02
	XCoord    uint16 = 0xDA03

	PartyCount   uint16 = 0xDA22
	PartySpecies uint16 = 0xDA23
	PartyMon1    uint16 = 0xDA2A
	PartyMonSize uint16 = 0x30

	BattleMode   uint16 = 0xD116
	BattleResult uint16 = 0xCFE9
	Money       uint16 = 0xD573
	JohtoBadges uint16 = 0xD57C
	KantoBadges uint16 = 0xD57D
	TMsHMs      uint16 = 0xD57E
	TMsHMsCount        = 57
	NumItems    uint16 = 0xD5B7
	Items       uint16 = 0xD5B8
	MaxItems           = 20
	NumKeyItems uint16 = 0xD5E1
	KeyItems    uint16 = 0xD5E2
	MaxKeyItems        = 25
	NumBalls    uint16 = 0xD5FC
	Balls       uint16 = 0xD5FD
	MaxBalls           = 12
)
