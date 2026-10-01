package game

// FieldActionState is the portable live state used to validate and verify
// out-of-battle field actions. Profiles own the RAM encodings and concrete
// object/tile identities behind these semantic facts. TargetKnown separates a
// positively decoded "nothing there" from a profile that cannot cheaply decode
// the current facing target; generic execution may let the cartridge decide in
// the latter case but must reject a known-invalid target before opening menus.
type FieldActionState struct {
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
