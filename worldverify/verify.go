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
	SeverityInfo    Severity = "info"
)

type Finding struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	Map      MapID    `json:"map,omitempty"`
	Edge     string   `json:"edge,omitempty"`
}

type Stats struct {
	Maps                               int  `json:"maps"`
	Edges                              int  `json:"edges"`
	Components                         int  `json:"components"`
	Capabilities                       int  `json:"capabilities"`
	CapabilityStatesChecked            int  `json:"capability_states_checked"`
	ExhaustiveCapabilities             bool `json:"exhaustive_capabilities"`
	FullReachableMaps                  int  `json:"full_reachable_maps,omitempty"`
	FullReachableComponents            int  `json:"full_reachable_components,omitempty"`
	FullUnreachableMaps                int  `json:"full_unreachable_maps,omitempty"`
	RequiredUnreachableMaps            int  `json:"required_unreachable_maps,omitempty"`
	OptionalUnreachableMaps            int  `json:"optional_unreachable_maps,omitempty"`
	ExpectedUnreachableMaps            int  `json:"expected_unreachable_maps,omitempty"`
	StoryStateDependentUnreachableMaps int  `json:"story_state_dependent_unreachable_maps,omitempty"`
	SuspiciousUnreachableMaps          int  `json:"suspicious_unreachable_maps,omitempty"`
	InactiveStaticEdges                int  `json:"inactive_static_edges,omitempty"`
	SemanticDeadPortEdges              int  `json:"semantic_dead_port_edges,omitempty"`
	ExecutableEdges                    int  `json:"executable_edges,omitempty"`
	DynamicExecutionEdges              int  `json:"dynamic_execution_edges,omitempty"`
}

type Report struct {
	Game            string            `json:"game,omitempty"`
	Stats           Stats             `json:"stats"`
	Findings        []Finding         `json:"findings,omitempty"`
	UnreachableMaps []MapReachability `json:"unreachable_maps,omitempty"`
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

func (r Report) InfoCount() int {
	n := 0
	for _, finding := range r.Findings {
		if finding.Severity == SeverityInfo {
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

	expectations := make(map[MapID]MapExpectation, len(snapshot.MapExpectations)+len(snapshot.RequiredMaps))
	for _, expectation := range snapshot.MapExpectations {
		if expectation.Map == "" {
			report.add(SeverityError, "empty_map_expectation", "reachability expectation has an empty map id", "", "")
			continue
		}
		if _, ok := maps[expectation.Map]; !ok {
			report.add(SeverityError, "unknown_map_expectation", fmt.Sprintf("reachability expectation references unknown map %q", expectation.Map), expectation.Map, "")
			continue
		}
		if !validReachabilityClass(expectation.Class) {
			report.add(SeverityError, "invalid_reachability_class", fmt.Sprintf("map %q has invalid reachability class %q", expectation.Map, expectation.Class), expectation.Map, "")
			continue
		}
		if _, exists := expectations[expectation.Map]; exists {
			report.add(SeverityError, "duplicate_map_expectation", fmt.Sprintf("map %q has more than one reachability expectation", expectation.Map), expectation.Map, "")
			continue
		}
		expectations[expectation.Map] = expectation
	}
	for _, required := range snapshot.RequiredMaps {
		if _, ok := maps[required]; !ok {
			report.add(SeverityError, "unknown_required_map", fmt.Sprintf("required map %q does not exist", required), required, "")
			continue
		}
		if expectation, exists := expectations[required]; exists {
			if expectation.Class != ReachabilityRequired {
				report.add(SeverityError, "conflicting_map_expectation", fmt.Sprintf("required map %q is classified as %q", required, expectation.Class), required, "")
			}
			continue
		}
		expectations[required] = MapExpectation{Map: required, Class: ReachabilityRequired, Reason: "legacy RequiredMaps assertion"}
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
		validateExecution(&report, edge, from, to, componentSet[edge.From], componentSet[edge.To])

		if edge.BorderSpan != nil {
			span := edge.BorderSpan
			if span.Limit <= 0 || span.Start < 0 || span.End < span.Start || span.End >= span.Limit {
				report.add(SeverityError, "invalid_border_span", fmt.Sprintf("edge %q has invalid border span [%d,%d] with limit %d", edge.ID, span.Start, span.End, span.Limit), edge.From, edge.ID)
			}
		}

		deadExit := edge.Exit.Known && len(edge.Exit.Components) == 0
		deadEntry := edge.Entry.Known && len(edge.Entry.Components) == 0
		ordinaryGeometry := edge.Transition == nil || edge.Transition.Gate
		if ordinaryGeometry {
			if deadExit || deadEntry {
				report.Stats.InactiveStaticEdges++
			}
		} else {
			semanticDead := false
			if deadExit {
				report.add(SeverityWarning, "semantic_dead_exit_port", fmt.Sprintf("semantic edge %q bypasses a proven non-walkable exit port", edge.ID), edge.From, edge.ID)
				semanticDead = true
			}
			if deadEntry {
				report.add(SeverityWarning, "semantic_dead_entry_port", fmt.Sprintf("semantic edge %q lands on a proven non-walkable entry port", edge.ID), edge.To, edge.ID)
				semanticDead = true
			}
			if semanticDead {
				report.Stats.SemanticDeadPortEdges++
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
		classifyFullReachability(&report, maps, full.maps, expectations)
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

func validReachabilityClass(class ReachabilityClass) bool {
	switch class {
	case ReachabilityRequired, ReachabilityOptional, ReachabilityExpectedUnreachable, ReachabilityStoryStateDependent, ReachabilitySuspicious:
		return true
	default:
		return false
	}
}

func classifyFullReachability(report *Report, maps map[MapID]Map, reachableMaps map[MapID]bool, expectations map[MapID]MapExpectation) {
	ids := make([]string, 0, len(maps))
	for id := range maps {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, raw := range ids {
		id := MapID(raw)
		if reachableMaps[id] {
			continue
		}
		expectation, ok := expectations[id]
		if !ok {
			expectation = MapExpectation{Map: id, Class: ReachabilitySuspicious, Reason: "adapter supplied no reachability explanation"}
		}
		m := maps[id]
		report.UnreachableMaps = append(report.UnreachableMaps, MapReachability{
			Map: id, Label: m.Label, Class: expectation.Class, Reason: expectation.Reason,
		})
		report.Stats.FullUnreachableMaps++
		name := fmt.Sprintf("map %q", id)
		if m.Label != "" {
			name = fmt.Sprintf("map %q (%s)", id, m.Label)
		}
		switch expectation.Class {
		case ReachabilityRequired:
			report.Stats.RequiredUnreachableMaps++
			report.add(SeverityError, "required_map_unreachable", fmt.Sprintf("%s is unreachable even with every declared capability: %s", name, expectation.Reason), id, "")
		case ReachabilityOptional:
			report.Stats.OptionalUnreachableMaps++
		case ReachabilityExpectedUnreachable:
			report.Stats.ExpectedUnreachableMaps++
		case ReachabilityStoryStateDependent:
			report.Stats.StoryStateDependentUnreachableMaps++
		case ReachabilitySuspicious:
			report.Stats.SuspiciousUnreachableMaps++
			report.add(SeverityWarning, "unclassified_unreachable_map", fmt.Sprintf("%s is unreachable with every declared capability: %s", name, expectation.Reason), id, "")
		}
	}

	for id, expectation := range expectations {
		if expectation.Class != ReachabilityExpectedUnreachable || !reachableMaps[id] {
			continue
		}
		m := maps[id]
		name := fmt.Sprintf("map %q", id)
		if m.Label != "" {
			name = fmt.Sprintf("map %q (%s)", id, m.Label)
		}
		report.add(SeverityWarning, "expected_unreachable_map_reachable", fmt.Sprintf("%s is reachable but manifest marks it expected-unreachable: %s", name, expectation.Reason), id, "")
	}
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

func validateExecution(report *Report, edge Edge, from, to Map, fromComponents, toComponents map[int]bool) {
	execution := edge.Execution
	if execution == nil {
		return
	}
	switch execution.Status {
	case ExecutionDynamicUnknown:
		report.Stats.DynamicExecutionEdges++
		reason := execution.Reason
		if reason == "" {
			reason = "adapter marked local execution as dynamic"
		}
		report.add(SeverityInfo, "dynamic_execution_unknown", fmt.Sprintf("edge %q is intentionally not statically proven executable: %s", edge.ID, reason), edge.From, edge.ID)
		return
	case ExecutionProven:
		report.Stats.ExecutableEdges++
	default:
		report.add(SeverityError, "invalid_execution_status", fmt.Sprintf("edge %q has unknown execution status %q", edge.ID, execution.Status), edge.From, edge.ID)
		return
	}

	if len(execution.Paths) == 0 {
		// A structurally present edge with a proven dead source/entry port is
		// already inactive and cannot be selected by component-aware routing.
		// Executability becomes a hard error only when the graph advertises a
		// usable port that the local navigator cannot realize.
		if (edge.Exit.Known && len(edge.Exit.Components) == 0) ||
			(edge.Entry.Known && len(edge.Entry.Components) == 0) {
			return
		}
		report.add(SeverityError, "edge_not_executable", fmt.Sprintf("edge %q has no statically executable local crossing", edge.ID), edge.From, edge.ID)
		return
	}

	executableExit := map[int]bool{}
	for _, path := range execution.Paths {
		validateExecutionPoint(report, "exit", path.ExitPoint, from, edge)
		validateExecutionPoint(report, "entry", path.EntryPoint, to, edge)
		if path.ExitComponent > 0 {
			executableExit[path.ExitComponent] = true
			if !fromComponents[path.ExitComponent] {
				report.add(SeverityError, "execution_unknown_exit_component", fmt.Sprintf("edge %q executable path references unknown source component %d", edge.ID, path.ExitComponent), edge.From, edge.ID)
			}
		}
		if path.EntryComponent > 0 && !toComponents[path.EntryComponent] {
			report.add(SeverityError, "execution_unknown_entry_component", fmt.Sprintf("edge %q executable path references unknown destination component %d", edge.ID, path.EntryComponent), edge.To, edge.ID)
		}
		if edge.Exit.Known && path.ExitComponent > 0 && !contains(edge.Exit.Components, path.ExitComponent) {
			report.add(SeverityError, "executor_exit_not_advertised", fmt.Sprintf("edge %q local executor can cross from component %d but graph exit port does not advertise it", edge.ID, path.ExitComponent), edge.From, edge.ID)
		}
		if edge.Entry.Known && path.EntryComponent > 0 && !contains(edge.Entry.Components, path.EntryComponent) {
			report.add(SeverityError, "executor_landing_not_advertised", fmt.Sprintf("edge %q local executor lands in component %d but graph entry port does not advertise it", edge.ID, path.EntryComponent), edge.To, edge.ID)
		}
	}
	if edge.Exit.Known {
		for _, component := range edge.Exit.Components {
			if component > 0 && !executableExit[component] {
				report.add(SeverityError, "graph_executor_exit_mismatch", fmt.Sprintf("edge %q graph advertises source component %d but no local execution path can use it", edge.ID, component), edge.From, edge.ID)
			}
		}
	}
}

func validateExecutionPoint(report *Report, side string, point Point, m Map, edge Edge) {
	if !m.GeometryKnown {
		return
	}
	if point.X < 0 || point.Y < 0 || point.X >= m.Width || point.Y >= m.Height {
		report.add(SeverityError, "execution_point_out_of_bounds", fmt.Sprintf("edge %q executable %s point (%d,%d) is outside map %q size %dx%d", edge.ID, side, point.X, point.Y, m.ID, m.Width, m.Height), m.ID, edge.ID)
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
