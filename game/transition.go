package game

import "fmt"

// ResourceID names a consumable transition resource without tying the core to
// one game's item/money encoding.
type ResourceID string

// CapabilitySet is the set of semantic permissions usable in the current game
// state. Adapters decide how those permissions are derived.
type CapabilitySet map[CapabilityID]bool

func NewCapabilitySet(ids ...CapabilityID) CapabilitySet {
	out := make(CapabilitySet, len(ids))
	for _, id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	return out
}

func (s CapabilitySet) Has(id CapabilityID) bool {
	return id != "" && s[id]
}

// ResourceCost keeps resource-consuming transitions representable without
// teaching routing what a ticket, coin, item, or charge means in one game.
type ResourceCost struct {
	Resource ResourceID `json:"resource"`
	Amount   int        `json:"amount"`
}

// Transition is the portable description of one semantic world edge. From and
// To identify game-semantic locations; Requires says what must be usable now.
// The remaining fields describe ownership/effect semantics that execution may
// need even when pathfinding only cares about prerequisites.
type Transition struct {
	ID             string         `json:"id,omitempty"`
	From           PlaceID        `json:"from"`
	To             PlaceID        `json:"to"`
	Requires       []CapabilityID `json:"requires,omitempty"`
	OneWay         bool           `json:"one_way,omitempty"`
	ChoiceRequired bool           `json:"choice_required,omitempty"`
	Consumes       []ResourceCost `json:"consumes,omitempty"`
	Effects        []ProgressID   `json:"effects,omitempty"`
}

// TransitionBlockage is structured evidence that a known transition exists
// geometrically but cannot currently be used because capabilities are absent.
type TransitionBlockage struct {
	Transition Transition     `json:"transition"`
	Missing    []CapabilityID `json:"missing"`
}

func (b TransitionBlockage) Error() string {
	return fmt.Sprintf("transition %q from %q to %q is missing capabilities %v",
		b.Transition.ID, b.Transition.From, b.Transition.To, b.Missing)
}

// MissingCapabilities returns requirements that are not usable now, preserving
// transition declaration order so diagnostics and planner inputs stay stable.
func MissingCapabilities(t Transition, caps CapabilitySet) []CapabilityID {
	var missing []CapabilityID
	for _, id := range t.Requires {
		if !caps.Has(id) {
			missing = append(missing, id)
		}
	}
	return missing
}

// EvaluateTransition reports whether t is currently usable. A false result is
// accompanied by structured prerequisite evidence; it never guesses how to
// satisfy the missing capability.
func EvaluateTransition(t Transition, caps CapabilitySet) (TransitionBlockage, bool) {
	missing := MissingCapabilities(t, caps)
	if len(missing) == 0 {
		return TransitionBlockage{}, true
	}
	return TransitionBlockage{Transition: t, Missing: missing}, false
}
