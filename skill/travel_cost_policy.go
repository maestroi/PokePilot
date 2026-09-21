package skill

import (
	"sync"

	"github.com/maestroi/pokepilot/emu"
)

// TravelCostPolicy selects how local navigation compares legal routes. The
// default preserves the historical conservative behavior: minimize field
// actions first, then walking. TravelCostFastest instead prices movement and
// field actions in shared coarse route-cost units so a short Cut/Surf/Strength
// shortcut may beat a much longer zero-action detour.
type TravelCostPolicy uint8

const (
	TravelCostConservative TravelCostPolicy = iota
	TravelCostFastest
)

const (
	fieldTravelMoveCost           = 1
	fieldTravelCutActionCost      = 8
	fieldTravelSurfActionCost     = 10
	fieldTravelStrengthActionCost = 10
)

// scopedTravelCostPolicies is keyed by emulator so concurrent farm runs do not
// leak play-style policy into one another. WithTravelCostPolicy always restores
// the previous value at the objective boundary, so completed objectives leave
// no process-global routing state behind.
var scopedTravelCostPolicies sync.Map

// WithTravelCostPolicy applies policy to one emulator until the returned
// restore function is called. Conservative is represented by absence from the
// map, keeping every legacy/direct skill caller on historical behavior.
func WithTravelCostPolicy(m *emu.Emu, policy TravelCostPolicy) func() {
	if m == nil {
		return func() {}
	}
	previous, hadPrevious := scopedTravelCostPolicies.Load(m)
	if policy == TravelCostFastest {
		scopedTravelCostPolicies.Store(m, policy)
	} else {
		scopedTravelCostPolicies.Delete(m)
	}
	return func() {
		if hadPrevious {
			scopedTravelCostPolicies.Store(m, previous)
		} else {
			scopedTravelCostPolicies.Delete(m)
		}
	}
}

func travelCostPolicyFor(m *emu.Emu) TravelCostPolicy {
	if m == nil {
		return TravelCostConservative
	}
	if raw, ok := scopedTravelCostPolicies.Load(m); ok {
		if policy, ok := raw.(TravelCostPolicy); ok {
			return policy
		}
	}
	return TravelCostConservative
}

type fieldPathCostPolicy struct {
	weighted     bool
	moveCost     int
	cutCost      int
	surfCost     int
	strengthCost int
}

func conservativeFieldPathCostPolicy() fieldPathCostPolicy {
	return fieldPathCostPolicy{moveCost: fieldTravelMoveCost}
}

func fastestFieldPathCostPolicy() fieldPathCostPolicy {
	return fieldPathCostPolicy{
		weighted:     true,
		moveCost:     fieldTravelMoveCost,
		cutCost:      fieldTravelCutActionCost,
		surfCost:     fieldTravelSurfActionCost,
		strengthCost: fieldTravelStrengthActionCost,
	}
}

func fieldPathCostPolicyFor(m *emu.Emu) fieldPathCostPolicy {
	if travelCostPolicyFor(m) == TravelCostFastest {
		return fastestFieldPathCostPolicy()
	}
	return conservativeFieldPathCostPolicy()
}

func (p fieldPathCostPolicy) less(a, b fieldPathCost) bool {
	if !p.weighted {
		if a.actions != b.actions {
			return a.actions < b.actions
		}
		return a.moves < b.moves
	}
	if a.weighted != b.weighted {
		return a.weighted < b.weighted
	}
	if a.actions != b.actions {
		return a.actions < b.actions
	}
	return a.moves < b.moves
}

func (p fieldPathCostPolicy) add(base fieldPathCost, action fieldPathAction, moves int) fieldPathCost {
	out := base
	out.moves += moves
	switch action {
	case fieldPathCut:
		out.actions++
		if p.weighted {
			out.weighted += p.cutCost
		}
	case fieldPathSurf:
		out.actions++
		if p.weighted {
			out.weighted += p.surfCost
		}
	}
	if p.weighted {
		out.weighted += moves * p.moveCost
	}
	return out
}
