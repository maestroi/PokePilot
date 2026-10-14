package game

// Weekday is portable schedule vocabulary. The adapter interprets the
// cartridge's native calendar and supplies the timetable, not the router.
type Weekday string

const (
	Sunday    Weekday = "sunday"
	Monday    Weekday = "monday"
	Tuesday   Weekday = "tuesday"
	Wednesday Weekday = "wednesday"
	Thursday  Weekday = "thursday"
	Friday    Weekday = "friday"
	Saturday  Weekday = "saturday"
)

// RegionTransport is a directed semantic connection between regions.
// One route in each direction is declared separately, because a return trip
// can have different prerequisites or a different timetable.
type RegionTransport struct {
	Service       string     `json:"service"`
	FromRegion    RegionID   `json:"from_region"`
	ToRegion      RegionID   `json:"to_region"`
	Leg           Transition `json:"leg"`
	DepartureDays []Weekday  `json:"departure_days,omitempty"`
	AnyDay        bool       `json:"any_day,omitempty"`
}

// RegionTransportOption is a planner-facing, evaluated route prerequisite.
// Missing holds typed capabilities, never prose interpreted by policy.
type RegionTransportOption struct {
	Service       string         `json:"service"`
	FromRegion    RegionID       `json:"from_region"`
	ToRegion      RegionID       `json:"to_region"`
	From          PlaceID        `json:"from"`
	To            PlaceID        `json:"to"`
	DepartureDays []Weekday      `json:"departure_days,omitempty"`
	AnyDay        bool           `json:"any_day,omitempty"`
	OneWay        bool           `json:"one_way,omitempty"`
	Available     bool           `json:"available"`
	Missing       []CapabilityID `json:"missing,omitempty"`
}

// AssessRegionTransports consumes the same transition capability contract as
// routing. It does not know which generation, vehicle, item or event is involved.
func AssessRegionTransports(routes []RegionTransport, caps CapabilitySet) []RegionTransportOption {
	if len(routes) == 0 {
		return nil
	}
	out := make([]RegionTransportOption, 0, len(routes))
	for _, route := range routes {
		missing := MissingCapabilities(route.Leg, caps)
		out = append(out, RegionTransportOption{
			Service: route.Service, FromRegion: route.FromRegion, ToRegion: route.ToRegion,
			From: route.Leg.From, To: route.Leg.To,
			DepartureDays: append([]Weekday(nil), route.DepartureDays...),
			AnyDay:        route.AnyDay, OneWay: route.Leg.OneWay,
			Available: len(missing) == 0, Missing: missing,
		})
	}
	return out
}

// RegionRoutes returns all transport options between the given regions,
// including blocked options so the caller can repair missing prerequisites.
func RegionRoutes(options []RegionTransportOption, from, to RegionID) []RegionTransportOption {
	var out []RegionTransportOption
	for _, option := range options {
		if option.FromRegion == from && option.ToRegion == to {
			out = append(out, option)
		}
	}
	return out
}

// RegionTransportProvider is optional. Clockless and single-region games need
// not implement it and therefore do not publish any transport routes.
type RegionTransportProvider interface {
	DecodeRegionTransports(MemoryReader) []RegionTransportOption
}
