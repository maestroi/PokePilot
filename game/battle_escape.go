package game

// BattleEscapeMenuKind identifies a battle menu that exposes a RUN action.
// Ordinary and Safari menus intentionally remain distinct because their other
// entries differ even when RUN occupies the same semantic grid position.
type BattleEscapeMenuKind string

const (
	BattleEscapeMenuOrdinary BattleEscapeMenuKind = "ordinary"
	BattleEscapeMenuSafari   BattleEscapeMenuKind = "safari"
)

// BattleEscapeMenuState is the portable live state needed by Flee. Cursor
// coordinates are semantic rows/columns; profiles own native cursor bytes.
type BattleEscapeMenuState struct {
	Visible bool
	Kind    BattleEscapeMenuKind
	Cursor  BattleMenuPosition
}

// BattleEscapeMenuDecoder hides game-specific RUN-capable battle-menu
// recognition and cursor layout from reusable escape execution.
type BattleEscapeMenuDecoder interface {
	DecodeBattleEscapeMenu(MemoryReader) BattleEscapeMenuState
	BattleEscapeRunPosition(BattleEscapeMenuKind) (BattleMenuPosition, bool)
}

// BattleEscapeMenuProfile is a game profile exposing escape-menu semantics.
type BattleEscapeMenuProfile interface {
	GameProfile
	BattleEscapeMenuDecoder
}
