// Package worldverify validates a portable world-model snapshot without
// depending on one game, ROM layout, or emulator implementation.
package worldverify

// MapID is an opaque adapter-owned map identity. Adapters should use a stable
// value for one game/revision; the verifier never interprets it.
type MapID string

// CapabilityID names one portable traversal capability such as can_cut or
// can_surf. The concrete source of the capability remains adapter-owned.
type CapabilityID string

// EdgeKind classifies topology for diagnostics only. Traversal semantics are
// carried by Transition.
type EdgeKind string

const (
	EdgeWarp       EdgeKind = "warp"
	EdgeConnection EdgeKind = "connection"
	EdgeOther      EdgeKind = "other"
)

// Map describes the geometry relevant to verification. Components are stable
// positive ids for mutually reachable standing regions. GeometryKnown=false
// means the adapter could not prove collision for this map and the verifier
// must remain conservative rather than inventing failures.
type Map struct {
	ID            MapID  `json:"id"`
	Label         string `json:"label,omitempty"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	GeometryKnown bool   `json:"geometry_known,omitempty"`
	Components    []int  `json:"components,omitempty"`
}

// Point is a concrete tile coordinate on a map.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Span is an inclusive border-coordinate interval. Limit is the exclusive
// axis length, so 0 <= Start <= End < Limit must hold.
type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
	Limit int `json:"limit"`
}

// Port captures the component evidence for one end of an edge. Known means
// the adapter had enough geometry to make an authoritative statement; a known
// port with zero Components is therefore a real dead/phantom port.
type Port struct {
	Known      bool   `json:"known,omitempty"`
	Components []int  `json:"components,omitempty"`
	Point      *Point `json:"point,omitempty"`
}

// Transition is the portable semantic overlay on one geometric edge.
// Gate means requirements are preconditions on ordinary geometry. PivotOnly
// means missing requirements still leave the ordinary geometric edge usable;
// when present, the capability may bypass the static component boundary.
// PortBypass means the semantic action itself intentionally creates a usable
// map-edge port where pristine standing collision has none (for example Surf).
type Transition struct {
	ID         string         `json:"id,omitempty"`
	Requires   []CapabilityID `json:"requires,omitempty"`
	Gate       bool           `json:"gate,omitempty"`
	PivotOnly  bool           `json:"pivot_only,omitempty"`
	PortBypass bool           `json:"port_bypass,omitempty"`
}

// Edge is one directed topology transition.
type Edge struct {
	ID         string      `json:"id"`
	Kind       EdgeKind    `json:"kind"`
	From       MapID       `json:"from"`
	To         MapID       `json:"to"`
	Exit       Port        `json:"exit"`
	Entry      Port        `json:"entry"`
	BorderSpan *Span       `json:"border_span,omitempty"`
	Transition *Transition `json:"transition,omitempty"`
}

// Snapshot is the complete portable input to Verify. StartMaps is optional;
// when present, capability-state exploration begins from every component in
// those maps. RequiredMaps, when present, must be reachable with the full
// capability set.
type Snapshot struct {
	Game         string  `json:"game,omitempty"`
	Maps         []Map   `json:"maps"`
	Edges        []Edge  `json:"edges"`
	StartMaps    []MapID `json:"start_maps,omitempty"`
	RequiredMaps []MapID `json:"required_maps,omitempty"`
}
