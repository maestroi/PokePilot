package game

// FieldActionState is the portable live state used to validate and verify
// out-of-battle field actions. Profiles own the RAM encodings and concrete
// object/tile identities behind these semantic facts. TargetKnown separates a
// positively decoded "nothing there" from a profile that cannot cheaply decode
// the current facing target; generic execution may let the cartridge decide in
// the latter case but must reject a known-invalid target before opening menus.
type FieldActionState struct {
	// PositionKnown is true when an adapter can positively report the
	// cartridge's current map and coordinates. Scripted movement field
	// actions (Waterfall) must verify displacement from their start tile,
	// not only a success byte set when the script was queued.
	PositionKnown       bool
	NativeMapID         uint16
	X, Y                uint8
	Controllable       bool
	CutTargetKnown     bool
	CuttableAhead      bool
	BoulderTargetKnown bool
	BoulderAhead       bool
	Surfing            bool
	StrengthActive     bool
	Lit                bool
	ActionSucceeded    bool
	ResultTextActive   bool
	InBattle           bool // a field action can start a wild battle (Rock Smash)
	ChoiceVisible      bool
	DebugText          string
}

// FieldActionDecoder hides game-specific field-action state encodings.
type FieldActionDecoder interface {
	DecodeFieldAction(MemoryReader) FieldActionState
}

// FieldActionProfile is a game profile that exposes portable field-action
// runtime state.
type FieldActionProfile interface {
	GameProfile
	FieldActionDecoder
}
