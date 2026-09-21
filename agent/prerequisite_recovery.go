package agent

// prerequisiteRecovery turns a typed route blockage into deterministic recovery
// when the active game adapter has already linked the missing route capability
// to an executable semantic prerequisite.
//
// Progress links retain their historical behavior: ordinary progression must be
// present in the offered menu, while RecoveryOnly links may synthesize it. Field
// capability links are different: when the observation proves the badge and HM
// are already owned but the move is not usable, the runtime synthesizes an
// explicit field-capability repair objective. That keeps party/PC/wild-carrier
// selection inside the game adapter instead of asking the strategist to invent
// a Pokemon that might not even be HM-compatible.
//
// Unknown or still-locked prerequisites fall through to the strategist.
func (f *runFailurePolicy) prerequisiteRecovery(obs Observation, offered []Objective) (Objective, []CapabilityID, bool) {
	if f == nil || len(f.pendingPrerequisites) == 0 {
		return Objective{}, nil, false
	}

	pending := make(map[CapabilityID]bool, len(f.pendingPrerequisites))
	for _, capability := range f.pendingPrerequisites {
		pending[capability] = true
	}

	prerequisiteFor := map[CapabilityID]RoutePrerequisiteLink{}
	for _, blockage := range obs.RouteBlockages {
		for _, prerequisite := range blockage.Prerequisites {
			if prerequisite.Capability == "" || !pending[prerequisite.Capability] {
				continue
			}
			prerequisiteFor[prerequisite.Capability] = prerequisite
		}
	}

	// Preserve normalized failure-context order. When a transition reports
	// multiple missing capabilities, repair one semantic prerequisite at a time
	// and then rebuild reachability from fresh live state.
	for _, capability := range f.pendingPrerequisites {
		prerequisite, ok := prerequisiteFor[capability]
		if !ok {
			continue
		}

		if prerequisite.Progress != "" {
			for _, objective := range offered {
				if objective.Kind == KindProgress && objective.Progress == prerequisite.Progress {
					return objective, []CapabilityID{capability}, true
				}
			}
			if prerequisite.RecoveryOnly {
				return Objective{Kind: KindProgress, Progress: prerequisite.Progress}, []CapabilityID{capability}, true
			}
		}

		if prerequisite.FieldCapability != "" && fieldCapabilityRepairReady(obs, prerequisite.FieldCapability) {
			return Objective{
				Kind:            KindRepairFieldCapability,
				FieldCapability: prerequisite.FieldCapability,
			}, []CapabilityID{capability}, true
		}
	}

	return Objective{}, nil, false
}

func fieldCapabilityRepairReady(obs Observation, capability CapabilityID) bool {
	for _, field := range obs.FieldCapabilities {
		if field.Name != capability {
			continue
		}
		// RepairFieldCapabilities can teach the current party, withdraw a boxed
		// carrier, or catch a ROM-compatible wild carrier. It cannot conjure the
		// badge or HM, so those remain progression prerequisites.
		return field.BadgeOwned && field.HMOwned && !field.Usable
	}
	return false
}
