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

// DecisionEngineSpec is one run's fast typed-decision selection. A nil spec
// means "not selected by the run": the runner keeps its historical
// environment-driven behavior, so older specs and runners are unchanged.
type DecisionEngineSpec struct {
	Backend string `json:"backend"`
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

// Normalized returns a canonical copy, or an error for an unknown backend or
// an out-of-range confidence. A nil spec stays nil.
func (d *DecisionEngineSpec) Normalized() (*DecisionEngineSpec, error) {
	if d == nil {
		return nil, nil
	}
	out := *d
	out.Backend = NormalizeDecisionBackend(d.Backend)
	if out.Backend == "" {
		return nil, fmt.Errorf("decision_engine.backend must be off, jev, or system-one (got %q)", d.Backend)
	}
	if math.IsNaN(out.MinConfidence) || out.MinConfidence < 0 || out.MinConfidence > 1 {
		return nil, fmt.Errorf("decision_engine.min_confidence must be between 0 and 1 (got %v)", d.MinConfidence)
	}
	if out.Backend == DecisionBackendOff {
		out.Objectives, out.Failures, out.MinConfidence = false, false, 0
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
	return d != nil && d.Backend != DecisionBackendOff && d.Backend != ""
}
