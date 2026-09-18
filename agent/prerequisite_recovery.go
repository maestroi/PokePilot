package agent

// prerequisiteRecovery turns a typed route blockage into deterministic recovery
// when the active game adapter has already linked the missing capability to a
// concrete progression fact and that progression objective is currently
// executable.
//
// This is intentionally narrower than general stall fallback. We only bypass the
// planner when all three pieces of evidence agree:
//   1. the previous recoverable failure named a missing capability;
//   2. the current observation links that capability to a progression fact; and
//   3. the normal objective provider is offering the matching progression step.
//
// Unknown prerequisites still fall through to the strategist/exploration path.
func (f *runFailurePolicy) prerequisiteRecovery(obs Observation, offered []Objective) (Objective, []CapabilityID, bool) {
	if f == nil || len(f.pendingPrerequisites) == 0 || len(offered) == 0 {
		return Objective{}, nil, false
	}

	pending := make(map[CapabilityID]bool, len(f.pendingPrerequisites))
	for _, capability := range f.pendingPrerequisites {
		pending[capability] = true
	}

	progressFor := map[CapabilityID]ProgressID{}
	for _, blockage := range obs.RouteBlockages {
		for _, prerequisite := range blockage.Prerequisites {
			if prerequisite.Capability == "" || !pending[prerequisite.Capability] || prerequisite.Progress == "" {
				continue
			}
			progressFor[prerequisite.Capability] = prerequisite.Progress
		}
	}

	// Preserve the normalized failure-context order. It is stable, and when a
	// transition reports multiple missing capabilities it lets the runtime make
	// one known semantic repair at a time before re-evaluating live state.
	for _, capability := range f.pendingPrerequisites {
		progress := progressFor[capability]
		if progress == "" {
			continue
		}
		for _, objective := range offered {
			if objective.Kind == KindProgress && objective.Progress == progress {
				return objective, []CapabilityID{capability}, true
			}
		}
	}

	return Objective{}, nil, false
}
