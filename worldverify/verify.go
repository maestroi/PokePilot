package worldverify

import (
	"fmt"
	"sort"
	"strings"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type Finding struct {
	Severity Severity `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Map      MapID `json:"map,omitempty"`
	Edge     string `json:"edge,omitempty"`
}

type Stats struct {
	Maps                    int `json:"maps"`
	Edges                   int `json:"edges"`
	Components              int `json:"components"`
	Capabilities            int `json:"capabilities"`
	CapabilityStatesChecked int `json:"capability_states_checked"`
	ExhaustiveCapabilities  bool `json:"exhaustive_capabilities"`
	FullReachableMaps       int `json:"full_reachable_maps,omitempty"`
	FullReachableComponents int `json:"full_reachable_components,omitempty"`
}

type Report struct {
	Game     string `json:"game,omitempty"`
	Stats    Stats `json:"stats"`
	Findings []Finding `json:"findings,omitempty"`
}

func (r Report) HasErrors() bool {
	for _, finding := range r.Findings {
		if finding.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (r Report) ErrorCount() int {
	n := 0
	for _, finding := range r.Findings {
		if finding.Severity == SeverityError {
			n++
		}
	}
	return n
}

func (r Report) WarningCount() int {
	n := 0
	for _, finding := range r.Findings {
		if finding.Severity == SeverityWarning {
			n++
		}
	}
	return n
}

type Options struct {
	// MaxExhaustiveCapabilities bounds 2^N enumeration. Above the bound the
	// verifier checks empty/full, each singleton, and full-minus-one states.
	// Zero uses 12 (4096 states).
	MaxExhaustiveCapabilities int
}

func Verify(snapshot Snapshot, options Options) Report {
	report := Report{Game: snapshot.Game}
	if options.MaxExhaustiveCapabilities <= 0 {
		options.MaxExhaustiveCapabilities = 12
	}

	maps := make(map[MapID]Map, len(snapshot.Maps))
	componentSet := make(map[MapID]map[int]bool, len(snapshot.Maps))
	for _, m := range snapshot.Maps {
		if m.ID == "" {
			report.add(SeverityError, "empty_map_id", "map has an empty id", "", "")
			continue
		}
		if _, exists := maps[m.ID]; exists {
			report.add(SeverityError, "duplicate_map", fmt.Sprintf("map %q is declared more than once", m.ID), m.ID, "")
			continue
		}
		if m.GeometryKnown && (m.Width <= 0 || m.Height <= 0) {
			report.add(SeverityError, "invalid_map_dimensions", fmt.Sprintf("map %q has known geometry with size %dx%d", m.ID, m.Width, m.Height), m.ID, "")
		}
		set := map[int]bool{}
		for _, component := range m.Components {
			if component <= 0 {
				report.add(SeverityError, "invalid_component", fmt.Sprintf("map %q contains non-positive component %d", m.ID, component), m.ID, "")
				continue
			}
			if set[component] {
				report.add(SeverityWarning, "duplicate_component", fmt.Sprintf("map %q repeats component %d", m.ID, component), m.ID, "")
				continue
			}
			set[component] = true
		}
		maps[m.ID] = m
		componentSet[m.ID] = set
		report.Stats.Components += len(set)
	}
	report.Stats.Maps = len(maps)

	for _, start := range snapshot.StartMaps {
		if _, ok := maps[start]; !ok {
			report.add(SeverityError, "unknown_start_map", fmt.Sprintf("start map %q does not exist", start), start, "")
		}
	}
	for _, required := range snapshot.RequiredMaps {
		if _, ok := maps[required]; !ok {
			report.add(SeverityError, "unknown_required_map", fmt.Sprintf("required map %q does not exist", required), required, "")
		}
	}

	capabilitySet := map[CapabilityID]bool{}
	edgeIDs := map[string]bool{}
	edgesByMap := make(map[MapID][]Edge)
	for i, edge := range snapshot.Edges {
		report.Stats.Edges++
		if edge.ID == "" {
			edge.ID = fmt.Sprintf("edge-%d", i)
			report.add(SeverityError, "empty_edge_id", fmt.Sprintf("edge %d has an empty id", i), edge.From, edge.ID)
		}
		if edgeIDs[edge.ID] {
			report.add(SeverityError, "duplicate_edge", fmt.Sprintf("edge id %q is declared more than once", edge.ID), edge.From, edge.ID)
		}
		edgeIDs[edge.ID] = true

		from, fromOK := maps[edge.From]
		to, toOK := maps[edge.To]
		if !fromOK {
			report.add(SeverityError, "unknown_edge_source", fmt.Sprintf("edge %q references unknown source map %q", edge.ID, edge.From), edge.From, edge.ID)
		}
		if !toOK {
			report.add(SeverityError, "unknown_edge_destination", fmt.Sprintf("edge %q references unknown destination map %q", edge.ID, edge.To), edge.From, edge.ID)
		}
		if !fromOK || !toOK {
			continue
		}

		validatePoint(&report, "exit", edge.Exit.Point, from, edge)
		validatePoint(&report, "entry", edge.Entry.Point, to, edge)
		validatePort(&report, "exit", edge.Exit, componentSet[edge.From], edge)
		validatePort(&report, "entry", edge.Entry, componentSet[edge.To], edge)

		if edge.BorderSpan != nil {
			span := edge.BorderSpan
			if span.Limit <= 0 || span.Start < 0 || span.End < span.Start || span.End >= span.Limit {
				report.add(SeverityError, "invalid_border_span", fmt.Sprintf("edge %q has invalid border span [%d,%d] with limit %d", edge.ID, span.Start, span.End, span.Limit), edge.From, edge.ID)
			}
		}

		ordinaryGeometry := edge.Transition == nil || edge.Transition.Gate
		if ordinaryGeometry {
			if edge.Exit.Known && len(edge.Exit.Components) == 0 {
				report.add(SeverityError, "dead_exit_port", fmt.Sprintf("edge %q has a proven non-walkable exit port", edge.ID), edge.From, edge.ID)
			}
			if edge.Entry.Known && len(edge.Entry.Components) == 0 {
				report.add(SeverityError, "dead_entry_port", fmt.Sprintf("edge %q has a proven non-walkable entry port", edge.ID), edge.To, edge.ID)
			}
		} else {
			// A semantic action may deliberately create traversal absent from
			// pristine collision (Surf, Cut, switches). Keep this visible but
			// non-fatal; the game adapter can tighten the edge when the action
			// itself does not own the dead port.
			if edge.Exit.Known && len(edge.Exit.Components) == 0 {
				report.add(SeverityWarning, "semantic_dead_exit_port", fmt.Sprintf("semantic edge %q bypasses a proven non-walkable exit port", edge.ID), edge.From, edge.ID)
			}
			if edge.Entry.Known && len(edge.Entry.Components) == 0 {
				report.add(SeverityWarning, "semantic_dead_entry_port", fmt.Sprintf("semantic edge %q lands on a proven non-walkable entry port", edge.ID), edge.To, edge.ID)
			}
		}

		if transition := edge.Transition; transition != nil {
			if transition.Gate && transition.PivotOnly {
				report.add(SeverityError, "invalid_transition_mode", fmt.Sprintf("transition %q cannot be both gate and pivot-only", transition.ID), edge.From, edge.ID)
			}
			seen := map[CapabilityID]bool{}
			for _, capability := range transition.Requires {
				if capability == "" {
					report.add(SeverityError, "empty_capability", fmt.Sprintf("transition %q has an empty capability requirement", transition.ID), edge.From, edge.ID)
					continue
				}
				if seen[capability] {
					report.add(SeverityWarning, "duplicate_capability", fmt.Sprintf("transition %q repeats capability %q", transition.ID, capability), edge.From, edge.ID)
				}
				seen[capability] = true
				capabilitySet[capability] = true
			}
		}
		edgesByMap[edge.From] = append(edgesByMap[edge.From], edge)
	}

	capabilities := sortedCapabilities(capabilitySet)
	report.Stats.Capabilities = len(capabilities)
	states, exhaustive := capabilityStates(capabilities, options.MaxExhaustiveCapabilities)
	report.Stats.ExhaustiveCapabilities = exhaustive

	if len(snapshot.StartMaps) > 0 {
		var full reachableResult
		for _, caps := range states {
			result := reachable(maps, edgesByMap, snapshot.StartMaps, caps)
			report.Stats.CapabilityStatesChecked++
			if len(caps) == len(capabilities) {
				full = result
			}
		}
		report.Stats.FullReachableMaps = len(full.maps)
		report.Stats.FullReachableComponents = len(full.states)
		for _, required := range snapshot.RequiredMaps {
			if maps[required].ID != "" && !full.maps[required] {
				report.add(SeverityError, "required_map_unreachable", fmt.Sprintf("required map %q is unreachable even with every declared capability", required), required, "")
			}
		}
	}

	sort.SliceStable(report.Findings, func(i, j int) bool {
		a, b := report.Findings[i], report.Findings[j]
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Map != b.Map {
			return a.Map < b.Map
		}
		return a.Edge < b.Edge
	})
	return report
}

func (r *Report) add(severity Severity, code, message string, mapID MapID, edge string) {
	r.Findings = append(r.Findings, Finding{Severity: severity, Code: code, Message: message, Map: mapID, Edge: edge})
}

func validatePoint(report *Report, side string, point *Point, m Map, edge Edge) {
	if point == nil || !m.GeometryKnown {
		return
	}
	if point.X < 0 || point.Y < 0 || point.X >= m.Width || point.Y >= m.Height {
		report.add(SeverityError, "point_out_of_bounds", fmt.Sprintf("edge %q %s point (%d,%d) is outside map %q size %dx%d", edge.ID, side, point.X, point.Y, m.ID, m.Width, m.Height), m.ID, edge.ID)
	}
}

func validatePort(report *Report, side string, port Port, valid map[int]bool, edge Edge) {
	if !port.Known {
		return
	}
	seen := map[int]bool{}
	for _, component := range port.Components {
		if component <= 0 || !valid[component] {
			report.add(SeverityError, "unknown_port_component", fmt.Sprintf("edge %q %s port references unknown component %d", edge.ID, side, component), edge.From, edge.ID)
		}
		if seen[component] {
			report.add(SeverityWarning, "duplicate_port_component", fmt.Sprintf("edge %q %s port repeats component %d", edge.ID, side, component), edge.From, edge.ID)
		}
		seen[component] = true
	}
}

func sortedCapabilities(set map[CapabilityID]bool) []CapabilityID {
	out := make([]CapabilityID, 0, len(set))
	for capability := range set {
		out = append(out, capability)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func capabilityStates(capabilities []CapabilityID, maxExhaustive int) ([]map[CapabilityID]bool, bool) {
	n := len(capabilities)
	if n <= maxExhaustive {
		states := make([]map[CapabilityID]bool, 0, 1<<n)
		for mask := 0; mask < 1<<n; mask++ {
			caps := map[CapabilityID]bool{}
			for i, capability := range capabilities {
				if mask&(1<<i) != 0 {
					caps[capability] = true
				}
			}
			states = append(states, caps)
		}
		return states, true
	}

	var states []map[CapabilityID]bool
	add := func(caps map[CapabilityID]bool) {
		key := capabilityKey(caps)
		for _, existing := range states {
			if capabilityKey(existing) == key {
				return
			}
		}
		states = append(states, caps)
	}
	add(map[CapabilityID]bool{})
	full := map[CapabilityID]bool{}
	for _, capability := range capabilities {
		full[capability] = true
	}
	add(full)
	for _, capability := range capabilities {
		add(map[CapabilityID]bool{capability: true})
		minus := map[CapabilityID]bool{}
		for _, other := range capabilities {
			if other != capability {
				minus[other] = true
			}
		}
		add(minus)
	}
	return states, false
}

func capabilityKey(caps map[CapabilityID]bool) string {
	var ids []string
	for capability := range caps {
		ids = append(ids, string(capability))
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

type reachState struct {
	mapID MapID
	comp  int
}

type reachableResult struct {
	states map[reachState]bool
	maps   map[MapID]bool
}

func reachable(maps map[MapID]Map, edgesByMap map[MapID][]Edge, starts []MapID, caps map[CapabilityID]bool) reachableResult {
	result := reachableResult{states: map[reachState]bool{}, maps: map[MapID]bool{}}
	var queue []reachState
	add := func(state reachState) {
		if result.states[state] {
			return
		}
		result.states[state] = true
		result.maps[state.mapID] = true
		queue = append(queue, state)
	}
	for _, start := range starts {
		m, ok := maps[start]
		if !ok {
			continue
		}
		if m.GeometryKnown && len(m.Components) > 0 {
			for _, component := range m.Components {
				if component > 0 {
					add(reachState{mapID: start, comp: component})
				}
			}
		} else {
			add(reachState{mapID: start})
		}
	}

	for i := 0; i < len(queue); i++ {
		current := queue[i]
		for _, edge := range edgesByMap[current.mapID] {
			pivot, usable := transitionMode(edge.Transition, caps)
			if !usable {
				continue
			}
			if !pivot && edge.Exit.Known {
				if len(edge.Exit.Components) == 0 || (current.comp != 0 && !contains(edge.Exit.Components, current.comp)) {
					continue
				}
			}

			destination, ok := maps[edge.To]
			if !ok {
				continue
			}
			var next []int
			switch {
			case pivot && destination.GeometryKnown && len(destination.Components) > 0:
				// This mirrors semantic routing: an action can rewrite live
				// collision, so the pristine landing component is not authoritative.
				next = destination.Components
			case edge.Entry.Known:
				next = edge.Entry.Components
			case destination.GeometryKnown && len(destination.Components) > 0:
				next = destination.Components
			default:
				next = []int{0}
			}
			for _, component := range next {
				add(reachState{mapID: edge.To, comp: component})
			}
		}
	}
	return result
}

func transitionMode(transition *Transition, caps map[CapabilityID]bool) (pivot bool, usable bool) {
	if transition == nil {
		return false, true
	}
	missing := false
	for _, capability := range transition.Requires {
		if !caps[capability] {
			missing = true
			break
		}
	}
	if missing {
		if transition.PivotOnly {
			return false, true
		}
		return false, false
	}
	if transition.Gate {
		return false, true
	}
	return true, true
}

func contains(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
