package game

import "strings"

// Place names were already semantic strings before the adapter migration, so
// PlaceID is an alias that documents the portable contract without forcing a
// repository-wide conversion of existing name variables. Species/item IDs are
// intentionally distinct types because those contracts previously carried raw
// Red bytes and need the compiler to prevent that regression.
type PlaceID = string

type (
	SpeciesID    string
	ItemID       string
	CapabilityID string
	ProgressID   string
)

func CanonicalID(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

type ProgressFact struct {
	ID       ProgressID `json:"id"`
	Complete bool       `json:"complete,omitempty"`
	Value    int        `json:"value,omitempty"`
}

type ProgressState []ProgressFact

func (s ProgressState) Has(id ProgressID) bool {
	fact, _ := s.Lookup(id)
	return fact.Complete
}

// Lookup reports whether the adapter projected id at all. A projected fact is
// that progression goal's positive verifier; an absent one means no verifier
// is registered, which must not be confused with "projected and false".
func (s ProgressState) Lookup(id ProgressID) (ProgressFact, bool) {
	for _, fact := range s {
		if fact.ID == id {
			return fact, true
		}
	}
	return ProgressFact{}, false
}

func (s ProgressState) Value(id ProgressID) (int, bool) {
	for _, fact := range s {
		if fact.ID == id {
			return fact.Value, true
		}
	}
	return 0, false
}
