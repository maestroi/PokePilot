package game

import "strings"

// Semantic identifiers are stable planner/runtime vocabulary. Concrete game
// adapters translate them to ROM indexes, RAM encodings, script ids, or other
// implementation details only at the game boundary.
type (
	PlaceID      string
	SpeciesID    string
	ItemID       string
	CapabilityID string
	ProgressID   string
)

// CanonicalID normalizes human-facing names into the stable spelling used by
// semantic identifiers. It deliberately does not know any game vocabulary.
func CanonicalID(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// ProgressFact is one game-independent progression fact. Complete covers
// boolean gates; Value carries monotonic/counting progress where useful. A
// fact may use both (for example, seven badge checks passed and complete).
type ProgressFact struct {
	ID       ProgressID `json:"id"`
	Complete bool       `json:"complete,omitempty"`
	Value    int        `json:"value,omitempty"`
}

// ProgressState is the portable progression snapshot exposed by an adapter.
// It is intentionally a list of semantic facts rather than a game-specific
// struct whose fields would have to grow for every supported title.
type ProgressState []ProgressFact

// Has reports whether id is present and complete.
func (s ProgressState) Has(id ProgressID) bool {
	for _, fact := range s {
		if fact.ID == id {
			return fact.Complete
		}
	}
	return false
}

// Value returns the numeric value carried by id when the fact is present.
func (s ProgressState) Value(id ProgressID) (int, bool) {
	for _, fact := range s {
		if fact.ID == id {
			return fact.Value, true
		}
	}
	return 0, false
}
