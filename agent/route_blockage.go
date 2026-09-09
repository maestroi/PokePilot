package agent

import (
	"errors"
	"sort"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

const routeBlockageCap = 12

// RoutePrerequisiteLink connects a missing portable route capability to
// planner-visible state that already describes how close the current snapshot
// is to satisfying it. It names conditions, never a game-specific recipe.
type RoutePrerequisiteLink struct {
	Capability      CapabilityID `json:"capability"`
	FieldCapability CapabilityID `json:"field_capability,omitempty"`
	Progress        ProgressID   `json:"progress,omitempty"`
}

// RouteBlockage is the bounded planner-facing projection of a structured
// world.RouteBlockedError. Destination is the requested semantic place;
// Transitions and Missing preserve semantic identities without leaking map IDs
// or parsing error prose.
type RouteBlockage struct {
	Destination   PlaceID                 `json:"destination"`
	Transitions   []string                `json:"transitions,omitempty"`
	Missing       []CapabilityID          `json:"missing"`
	Prerequisites []RoutePrerequisiteLink `json:"prerequisites,omitempty"`
}

type routeAvailability struct {
	Unroutable []string
	Blockages  []RouteBlockage
}

type routeReachability interface {
	Reachability(skill.Destination) error
}

func routeAvailabilityFor(m *emu.Emu, romData []byte) routeAvailability {
	planner, err := skill.NewRoutePlanner(m, romData)
	if err != nil {
		// nil means the question was never reliably asked. Offer deliberately
		// fails open on that distinction.
		return routeAvailability{}
	}
	return collectRouteAvailability(planner, skill.PlaceNames())
}

func collectRouteAvailability(planner routeReachability, names []string) routeAvailability {
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
		if !errors.As(err, &blocked) || len(out.Blockages) >= routeBlockageCap {
			continue
		}
		blockage := plannerRouteBlockage(semanticPlace(name), blocked)
		if len(blockage.Missing) != 0 {
			out.Blockages = append(out.Blockages, blockage)
		}
	}
	return out
}

func plannerRouteBlockage(destination PlaceID, blocked *world.RouteBlockedError) RouteBlockage {
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
	for _, id := range result.Missing {
		if link, ok := redRoutePrerequisiteLink(id); ok {
			result.Prerequisites = append(result.Prerequisites, link)
		}
	}
	return result
}

func redRoutePrerequisiteLink(id CapabilityID) (RoutePrerequisiteLink, bool) {
	switch gameruntime.CapabilityID(id) {
	case "can_exit_mt_moon":
		return RoutePrerequisiteLink{Capability: id, Progress: redProgressMtMoonFossilAcquired}, true
	case "can_cut":
		return RoutePrerequisiteLink{Capability: id, FieldCapability: "cut"}, true
	case "can_surf":
		return RoutePrerequisiteLink{Capability: id, FieldCapability: "surf"}, true
	case "can_move_boulders":
		return RoutePrerequisiteLink{Capability: id, FieldCapability: "strength"}, true
	case "can_clear_snorlax":
		return RoutePrerequisiteLink{Capability: id, Progress: redProgressPokeFluteAcquired}, true
	default:
		// Unknown capabilities stay explicit in Missing. Not having a known
		// preparation link is evidence we do not know a recipe yet.
		return RoutePrerequisiteLink{}, false
	}
}
