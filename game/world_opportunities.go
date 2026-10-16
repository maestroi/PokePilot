package game

// PhoneContact is one registered contact, decoded by the cartridge adapter.
// A number's presence is independent of whether a battle is currently offered.
type PhoneContact struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Location PlaceID `json:"location,omitempty"`
}

// BattleSourceKind identifies how a repeatable or one-time battle becomes
// available without exposing the native script that happens to start it.
type BattleSourceKind string

const BattleSourcePhoneRematch BattleSourceKind = "phone_rematch"

// BattleSource is planner-facing battle availability. Ready is a positive
// cartridge-derived condition, not a prediction that a call will arrive soon.
type BattleSource struct {
	ID       string           `json:"id"`
	Kind     BattleSourceKind `json:"kind"`
	Trainer  string           `json:"trainer"`
	Location PlaceID          `json:"location"`
	Ready    bool             `json:"ready"`
}

// RouteGate is a semantic transition assessed against portable capabilities.
// Gates describe prerequisites, not inferred walkable geometry.
type RouteGate struct {
	Transition Transition     `json:"transition"`
	Available  bool           `json:"available"`
	Missing    []CapabilityID `json:"missing,omitempty"`
}

func AssessRouteGate(t Transition, caps CapabilitySet) RouteGate {
	return RouteGate{
		Transition: t,
		Available:  len(MissingCapabilities(t, caps)) == 0,
		Missing:    MissingCapabilities(t, caps),
	}
}

// WorldOpportunities contains optional capabilities and structured sources.
// Games without radio/phone mechanics need not implement this surface.
type WorldOpportunities struct {
	Capabilities []CapabilityID `json:"capabilities,omitempty"`
	PhoneContacts []PhoneContact `json:"phone_contacts,omitempty"`
	BattleSources []BattleSource `json:"battle_sources,omitempty"`
	RouteGates []RouteGate `json:"route_gates,omitempty"`
}

// WorldOpportunityDecoder is optional and owned by the cartridge profile.
type WorldOpportunityDecoder interface {
	DecodeWorldOpportunities(MemoryReader) WorldOpportunities
}
