package game

// PromptKind identifies a gameplay choice by meaning rather than by the
// concrete menu controller or screen text that happens to render it.
type PromptKind string

const (
	PromptUnknown  PromptKind = ""
	PromptNickname PromptKind = "nickname"
)

// PromptState is the semantic state of a live choice prompt. Visible is kept
// separate from Kind so adapters can report an unclassified prompt without
// generic code guessing what accepting it would mean.
type PromptState struct {
	Visible bool
	Kind    PromptKind
}

// PromptDecoder hides game-specific prompt text, flags and controller state
// from reusable execution code.
type PromptDecoder interface {
	DecodePrompt(MemoryReader) PromptState
}

// PromptProfile is a game profile that exposes semantic prompt identity.
type PromptProfile interface {
	GameProfile
	PromptDecoder
}
