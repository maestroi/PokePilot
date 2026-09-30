package game

// FieldActionState is the portable live state used to validate and verify
// out-of-battle field actions. Profiles own the RAM encodings and concrete
// object/tile identities behind these semantic facts.
type FieldActionState struct {
	Controllable     bool
	CuttableAhead    bool
	BoulderAhead     bool
	Surfing          bool
	StrengthActive   bool
	Lit              bool
	ActionSucceeded  bool
	ResultTextActive bool
	ChoiceVisible    bool
	DebugText        string
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

// FieldActionSupportDecoder lets a profile that is migrating field actions
// incrementally advertise only the actions whose live verification semantics
// are implemented. Profiles that do not implement it retain the historical
// all-actions behavior.
type FieldActionSupportDecoder interface {
	SupportsFieldAction(FieldMoveID) bool
}
