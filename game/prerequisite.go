package game

import (
	"fmt"
	"strings"
)

// Prerequisite is one portable semantic condition that must hold before an
// objective can run. Capability and Progress are deliberately separate: route
// topology and story-stage ordering share recovery plumbing without becoming
// the same kind of fact.
type Prerequisite struct {
	Capability      CapabilityID `json:"capability,omitempty"`
	FieldCapability CapabilityID `json:"field_capability,omitempty"`
	Progress        ProgressID   `json:"progress,omitempty"`
}

func CapabilityPrerequisite(id CapabilityID) Prerequisite {
	return Prerequisite{Capability: id}
}

func FieldCapabilityPrerequisite(id CapabilityID) Prerequisite {
	return Prerequisite{FieldCapability: id}
}

func ProgressionPrerequisite(id ProgressID) Prerequisite {
	return Prerequisite{Progress: id}
}

// PrerequisiteMissingError is adapter/native evidence that objective execution
// is blocked on semantic prerequisites. Missing preserves declared order so
// recovery can satisfy one prerequisite, re-observe, and then choose the next.
type PrerequisiteMissingError struct {
	Missing []Prerequisite
}

func (e *PrerequisiteMissingError) Error() string {
	if e == nil || len(e.Missing) == 0 {
		return "game: semantic prerequisite missing"
	}
	parts := make([]string, 0, len(e.Missing))
	for _, prerequisite := range e.Missing {
		switch {
		case prerequisite.Progress != "":
			parts = append(parts, "progress:"+string(prerequisite.Progress))
		case prerequisite.FieldCapability != "":
			parts = append(parts, "field_capability:"+string(prerequisite.FieldCapability))
		case prerequisite.Capability != "":
			parts = append(parts, "capability:"+string(prerequisite.Capability))
		default:
			parts = append(parts, "unknown")
		}
	}
	return fmt.Sprintf("game: semantic prerequisites missing [%s]", strings.Join(parts, ", "))
}

func NewFieldCapabilityPrerequisiteMissing(ids ...CapabilityID) error {
	missing := make([]Prerequisite, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			missing = append(missing, FieldCapabilityPrerequisite(id))
		}
	}
	return &PrerequisiteMissingError{Missing: missing}
}

func NewProgressionPrerequisiteMissing(ids ...ProgressID) error {
	missing := make([]Prerequisite, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			missing = append(missing, ProgressionPrerequisite(id))
		}
	}
	return &PrerequisiteMissingError{Missing: missing}
}
