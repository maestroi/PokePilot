package agent

// prerequisiteRecovery turns a typed route blockage into deterministic recovery
// when the active game adapter has already linked the missing capability to a
// concrete progression fact and that progression objective is currently
// executable.
//
// This is intentionally narrower than general stall fallback. We only bypass the
// planner when all three pieces of evidence agree:
//  1. the previous recoverable failure named a missing capability;
//  2. the current observation links that capability to a progression fact; and
//  3. the normal objective provider is offering the matching progression step,
//     unless the adapter explicitly marks that link recovery-only.
//
// Recovery-only links are useful for optional detours such as Cycling Road: the
// action stays out of the normal campaign menu, but a concrete typed blockage
// can still request exactly the progression needed to satisfy that route.
// Unknown prerequisites still fall through to the strategist/exploration path.
func (f *runFailurePolicy) prerequisiteRecovery(obs Observation, offered []Objective) (Objective, []CapabilityID, bool) {
	if f == nil || len(f.pendingPrerequisites) == 0 || len(offered) == 0 {
		return Objective{}, nil, false
	}

	pending := make(map[CapabilityID]bool, len(f.pendingPrerequisites))
	for _, capability := range f.pendingPrerequisites {
		pending[capability] = true
	}

	progressFor := map[CapabilityID]RoutePrerequisiteLink{}
	for _, blockage := range obs.RouteBlockages {
		for _, prerequisite := range blockage.Prerequisites {
			if prerequisite.Capability == "" || !pending[prerequisite.Capability] || prerequisite.Progress == "" {
				continue
			}
			progressFor[prerequisite.Capability] = prerequisite
		}
	}

	// Preserve the normalized failure-context order. It is stable, and when a
	// transition reports multiple missing capabilities it lets the runtime make
	// one known semantic repair at a time before re-evaluating live state.
	for _, capability := range f.pendingPrerequisites {
		prerequisite, ok := progressFor[capability]
		if !ok || prerequisite.Progress == "" {
			continue
		}
		for _, objective := range offered {
			if objective.Kind == KindProgress && objective.Progress == prerequisite.Progress {
				return objective, []CapabilityID{capability}, true
			}
		}
		if prerequisite.RecoveryOnly {
			return Objective{Kind: KindProgress, Progress: prerequisite.Progress}, []CapabilityID{capability}, true
		}
	}

	return Objective{}, nil, false
}
