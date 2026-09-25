package game

// BattleRuntimeState is the portable execution/settlement view around a
// battle. It carries only semantic facts reusable controllers need while
// entering, debugging, or settling a fight.
type BattleRuntimeState struct {
	InBattle         bool
	Controllable     bool
	TextActive       bool
	CampaignComplete bool

	NativeMapID uint16
	X, Y        uint8
	DebugText   string
	MenuCursor  MenuCursorState
}

// BattleRuntimeDecoder hides game-specific battle-boundary, text-engine,
// position and campaign-ending state from reusable battle controllers.
type BattleRuntimeDecoder interface {
	DecodeBattleRuntime(MemoryReader) BattleRuntimeState
}

type BattleRuntimeProfile interface {
	GameProfile
	BattleRuntimeDecoder
}
