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
	WindowStackPointer      uint16 = 0xCEA8 // wWindowStackPointer
	MenuJoypad              uint16 = 0xCEAA // wMenuJoypad
	MenuSelection           uint16 = 0xCEAB // wMenuSelection
	WhichIndexSet           uint16 = 0xCEAD // wWhichIndexSet
	TwoDMenuCursorInitY     uint16 = 0xCED8 // w2DMenuCursorInitY
	TwoDMenuCursorInitX     uint16 = 0xCED9 // w2DMenuCursorInitX
	TwoDMenuNumRows         uint16 = 0xCEDA // w2DMenuNumRows
	TwoDMenuNumCols         uint16 = 0xCEDB // w2DMenuNumCols
	MenuJoypadFilter        uint16 = 0xCEDF // wMenuJoypadFilter
	MenuCursorY             uint16 = 0xCEE0 // wMenuCursorY
	MenuCursorX             uint16 = 0xCEE1 // wMenuCursorX
	BattleMenuCursor        uint16 = 0xCFC4 // wBattleMenuCursorPosition
	CurBattleMon            uint16 = 0xCFC6 // wCurBattleMon
	CurMoveNum              uint16 = 0xCFC7 // wCurMoveNum
	PartyMenuCursor         uint16 = 0xCFC9 // wPartyMenuCursor
	SwitchMon               uint16 = 0xCFD3 // wSwitchMon
	NumMoves                uint16 = 0xCFE3 // wNumMoves
	BattlePlayerAction      uint16 = 0xCFE4 // wBattlePlayerAction
	StateFlags              uint16 = 0xCFE5 // wStateFlags
	TileMap                 uint16 = 0xC4A0 // wTileMap, 20x18
	TileMapLen                     = 20 * 18
	JumptableIndex          uint16 = 0xCE63 // wJumptableIndex
	TitleScreenSelected     uint16 = 0xCE64 // wTitleScreenSelectedOption
	TitleScreenTimer        uint16 = 0xCE65 // wTitleScreenTimer
	NamingScreenDestination uint16 = 0xC5D0 // wNamingScreenDestinationPointer
	NamingScreenCurNameLen  uint16 = 0xC5D2 // wNamingScreenCurNameLength
	NamingScreenMaxNameLen  uint16 = 0xC5D3 // wNamingScreenMaxNameLength
	NamingScreenType        uint16 = 0xC5D4 // wNamingScreenType
	TimeSetBuffer           uint16 = 0xC508 // wTimeSetBuffer
	InitHourBuffer          uint16 = 0xC51C // wInitHourBuffer
	InitMinuteBuffer        uint16 = 0xC526 // wInitMinuteBuffer
	StringBuffer2           uint16 = 0xCF7E // wStringBuffer2

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
	TextboxFlags    uint16 = 0xD19C // wTextboxFlags
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

	BattleMode            uint16 = 0xD116
	BattleType            uint16 = 0xD119
	ForcedSwitch          uint16 = 0xD11C
	MoveSelectionMenuType uint16 = 0xD11F
	BattleResult          uint16 = 0xCFE9
	CurPartyMon           uint16 = 0xD005

	// Live Gen-II battle structs. battle_struct is defined by the pinned
	// pret/pokegold macros/ram.asm layout: species, item, moves, DVs, PP,
	// happiness, level, status, HP/max HP, calculated stats, then types.
	BattleMon        uint16 = 0xCB0C // wBattleMon
	BattleMonSpecies        = BattleMon + 0x00
	BattleMonMoves          = BattleMon + 0x02
	BattleMonPP             = BattleMon + 0x08
	BattleMonLevel          = BattleMon + 0x0D
	BattleMonHP             = BattleMon + 0x10
	BattleMonMaxHP          = BattleMon + 0x12
	BattleMonAttack         = BattleMon + 0x14
	BattleMonDefense        = BattleMon + 0x16
	BattleMonSpeed          = BattleMon + 0x18
	BattleMonSpclAtk        = BattleMon + 0x1A
	BattleMonSpclDef        = BattleMon + 0x1C
	BattleMonType1          = BattleMon + 0x1E
	BattleMonType2          = BattleMon + 0x1F

	PlayerStatLevels uint16 = 0xCBAA // wPlayerStatLevels

	EnemyMon        uint16 = 0xD0EF // wEnemyMon
	EnemyMonSpecies        = EnemyMon + 0x00
	EnemyMonMoves          = EnemyMon + 0x02
	EnemyMonPP             = EnemyMon + 0x08
	EnemyMonLevel          = EnemyMon + 0x0D
	EnemyMonHP             = EnemyMon + 0x10
	EnemyMonMaxHP          = EnemyMon + 0x12
	EnemyMonAttack         = EnemyMon + 0x14
	EnemyMonDefense        = EnemyMon + 0x16
	EnemyMonSpeed          = EnemyMon + 0x18
	EnemyMonSpclAtk        = EnemyMon + 0x1A
	EnemyMonSpclDef        = EnemyMon + 0x1C
	EnemyMonType1          = EnemyMon + 0x1E
	EnemyMonType2          = EnemyMon + 0x1F

	EnemyStatLevels uint16 = 0xCBB2 // wEnemyStatLevels

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
