package game

// FieldActionState is the portable live state used to validate and verify
// out-of-battle field actions. Profiles own the RAM encodings and concrete
// object/tile identities behind these semantic facts.
type FieldActionState struct {
	Controllable    bool
	CuttableAhead   bool
	BoulderAhead    bool
	Surfing         bool
	StrengthActive  bool
	Lit             bool
	ActionSucceeded bool
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
