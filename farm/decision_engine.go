package farm

import (
	"fmt"
	"math"
	"strings"
)

// Typed-decision backends a run may select. The runner resolves endpoints and
// credentials from its own environment; the spec only names the choice, so
// secrets never enter a run spec, the catalog, or an archive.
const (
	DecisionBackendOff       = "off"
	DecisionBackendJev       = "jev"
	DecisionBackendSystemOne = "system-one"
)

// Decision modes. Active lets the backend's accepted answer steer the enabled
// features; shadow asks the backend at the same decision points and records
// its answer and agreement while the existing policy keeps executing. Off is
// the same as backend off.
const (
	DecisionModeOff    = "off"
	DecisionModeShadow = "shadow"
	DecisionModeActive = "active"
)

// DecisionEngineSpec is one run's fast typed-decision selection. A nil spec
// means "not selected by the run": the runner keeps its historical
// environment-driven behavior, so older specs and runners are unchanged.
type DecisionEngineSpec struct {
	Backend string `json:"backend"`
	// Mode is off, shadow or active. Empty means active, which is how every
	// selection made before modes existed behaved.
	Mode string `json:"mode,omitempty"`
	// Battles asks the backend at battle turns. Battle decisions are
	// observational only, so this requires shadow mode.
	Battles bool `json:"battles,omitempty"`
	// Objectives lets the backend pick from the already-valid objective menu.
	Objectives bool `json:"objectives,omitempty"`
	// Failures lets the backend classify failures the runtime already
	// declared recoverable.
	Failures bool `json:"failures,omitempty"`
	// MinConfidence below which an answer falls back to the existing path.
	// Zero means the runner default.
	MinConfidence float64 `json:"min_confidence,omitempty"`
}

// NormalizeDecisionBackend maps accepted aliases to the canonical backend
// name, or "" when the value is unknown.
func NormalizeDecisionBackend(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "", "off", "none", "disabled":
		return DecisionBackendOff
	case "jev", "typesafe", "typesafe-jev":
		return DecisionBackendJev
	case "system-one", "system_one", "local", "openai-compatible":
		return DecisionBackendSystemOne
	}
	return ""
}

// NormalizeDecisionMode maps accepted spellings to the canonical mode, or ""
// when the value is unknown. Empty is active for older selections.
func NormalizeDecisionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "active", "on":
		return DecisionModeActive
	case "shadow", "observe":
		return DecisionModeShadow
	case "off", "none", "disabled":
		return DecisionModeOff
	}
	return ""
}

// Normalized returns a canonical copy, or an error for an unknown backend or
// mode, an out-of-range confidence, or battle decisions outside shadow mode.
// A nil spec stays nil.
func (d *DecisionEngineSpec) Normalized() (*DecisionEngineSpec, error) {
	if d == nil {
		return nil, nil
	}
	out := *d
	out.Backend = NormalizeDecisionBackend(d.Backend)
	if out.Backend == "" {
		return nil, fmt.Errorf("decision_engine.backend must be off, jev, or system-one (got %q)", d.Backend)
	}
	out.Mode = NormalizeDecisionMode(d.Mode)
	if out.Mode == "" {
		return nil, fmt.Errorf("decision_engine.mode must be off, shadow, or active (got %q)", d.Mode)
	}
	if math.IsNaN(out.MinConfidence) || out.MinConfidence < 0 || out.MinConfidence > 1 {
		return nil, fmt.Errorf("decision_engine.min_confidence must be between 0 and 1 (got %v)", d.MinConfidence)
	}
	if out.Backend == DecisionBackendOff || out.Mode == DecisionModeOff {
		return &DecisionEngineSpec{Backend: DecisionBackendOff, Mode: DecisionModeOff}, nil
	}
	if out.Battles && out.Mode != DecisionModeShadow {
		return nil, fmt.Errorf("decision_engine.battles requires mode shadow (got %q)", out.Mode)
	}
	return &out, nil
}

// Clone copies the selection so tiles, leases, clones and successors never
// share one mutable value.
func (d *DecisionEngineSpec) Clone() *DecisionEngineSpec {
	if d == nil {
		return nil
	}
	out := *d
	return &out
}

// Enabled reports whether the run asked for a typed backend.
func (d *DecisionEngineSpec) Enabled() bool {
	return d != nil && d.Backend != DecisionBackendOff && d.Backend != "" && d.Mode != DecisionModeOff
}

// Shadow reports whether the run only observes the backend's answers.
func (d *DecisionEngineSpec) Shadow() bool {
	return d.Enabled() && NormalizeDecisionMode(d.Mode) == DecisionModeShadow
}
