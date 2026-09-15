package agent

import (
	"errors"
	"sort"

	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// routeBlockageCap bounds the planner-facing JSON projection. Runtime keeps
// every semantic blockage so Offer can never re-offer a progression-locked
// destination merely because an unrelated set of blocked places filled this
// prompt budget first.
const routeBlockageCap = 12

// RoutePrerequisiteLink connects a missing portable route capability to
// planner-visible state that already describes how close the current snapshot
// is to satisfying it. It names conditions, never a game-specific recipe.
type RoutePrerequisiteLink struct {
	Capability      CapabilityID `json:"capability"`
	FieldCapability CapabilityID `json:"field_capability,omitempty"`
	Progress        ProgressID   `json:"progress,omitempty"`
	Badge           string       `json:"badge,omitempty"`
}

// RouteBlockage is the planner-facing projection of either a structured
// world.RouteBlockedError or an adapter-owned semantic story requirement.
// Destination is always portable; native map ids never cross this contract.
type RouteBlockage struct {
	Destination   PlaceID                 `json:"destination"`
	Transitions   []string                `json:"transitions,omitempty"`
	Missing       []CapabilityID          `json:"missing,omitempty"`
	Prerequisites []RoutePrerequisiteLink `json:"prerequisites,omitempty"`
}

type routeAvailability struct {
	Unroutable []string
	Blockages  []RouteBlockage
}

type routeReachability interface {
	Reachability(skill.Destination) error
}

type routePrerequisiteLinker func(CapabilityID) (RoutePrerequisiteLink, bool)

func collectRouteAvailability(planner routeReachability, names []string, link routePrerequisiteLinker) routeAvailability {
	if planner == nil {
		return routeAvailability{}
	}
	names = append([]string(nil), names...)
	sort.Strings(names)
	out := routeAvailability{Unroutable: []string{}, Blockages: []RouteBlockage{}}
	for _, name := range names {
		destination, ok := skill.Place(name)
		if !ok {
			continue
		}
		err := planner.Reachability(destination)
		if err == nil {
			continue
		}
		out.Unroutable = append(out.Unroutable, name)

		var blocked *world.RouteBlockedError
		if !errors.As(err, &blocked) {
			continue
		}
		blockage := plannerRouteBlockage(semanticPlace(name), blocked, link)
		if len(blockage.Missing) != 0 {
			// Do not cap here. Offer consumes the runtime Observation and needs
			// the COMPLETE semantic-blocked set. The JSON boundary is where the
			// prompt is bounded; see Observation.MarshalJSON.
			out.Blockages = append(out.Blockages, blockage)
		}
	}
	return out
}

func plannerRouteBlockage(destination PlaceID, blocked *world.RouteBlockedError, link routePrerequisiteLinker) RouteBlockage {
	result := RouteBlockage{Destination: destination, Transitions: []string{}, Missing: []CapabilityID{}}
	if blocked == nil {
		return result
	}
	transitionSeen := map[string]bool{}
	missingSeen := map[CapabilityID]bool{}
	for _, blockage := range blocked.Blockages {
		if id := blockage.Transition.ID; id != "" && !transitionSeen[id] {
			transitionSeen[id] = true
			result.Transitions = append(result.Transitions, id)
		}
		for _, raw := range blockage.Missing {
			id := CapabilityID(raw)
			if id == "" || missingSeen[id] {
				continue
			}
			missingSeen[id] = true
			result.Missing = append(result.Missing, id)
		}
	}
	sort.Strings(result.Transitions)
	sort.Slice(result.Missing, func(i, j int) bool { return result.Missing[i] < result.Missing[j] })
	if link != nil {
		for _, id := range result.Missing {
			if prerequisite, ok := link(id); ok {
				result.Prerequisites = append(result.Prerequisites, prerequisite)
			}
		}
	}
	return result
}
