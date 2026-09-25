package game

// BattleMenuEntry is a semantic entry in the ordinary turn-action menu.
// Concrete profiles map these identities onto their game-specific layout.
type BattleMenuEntry string

const (
	BattleMenuFight   BattleMenuEntry = "fight"
	BattleMenuItems   BattleMenuEntry = "items"
	BattleMenuPokemon BattleMenuEntry = "pokemon"
	BattleMenuRun     BattleMenuEntry = "run"
)

// BattleMenuPosition is a semantic grid coordinate. Column/row are deliberately
// layout-neutral: a profile may use a different ordering or shape than Gen I.
type BattleMenuPosition struct {
	Column int
	Row    int
}

// BattleMainMenuState is the minimum live state generic menu navigation needs.
type BattleMainMenuState struct {
	Visible bool
	Cursor  BattleMenuPosition
}

// BattleMenuDecoder is the optional capability used to navigate the ordinary
// battle action menu without exposing native RAM addresses or cursor columns.
type BattleMenuDecoder interface {
	DecodeBattleMainMenu(MemoryReader) BattleMainMenuState
	BattleMainMenuEntryPosition(BattleMenuEntry) (BattleMenuPosition, bool)
}

// BattleMenuProfile is a game profile that exposes semantic battle-menu state.
type BattleMenuProfile interface {
	GameProfile
	BattleMenuDecoder
}
