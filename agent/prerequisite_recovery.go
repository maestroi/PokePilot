package agent

// PrerequisiteRecoveryLink is adapter-owned knowledge that connects a portable
// progression fact to the objective that can satisfy it. RecoverySafe permits
// synthesis only when the adapter explicitly says the objective is safe to run
// outside the normal offered menu.
type PrerequisiteRecoveryLink struct {
	Objective    Objective
	RecoverySafe bool
}

type ProgressionPrerequisiteRecoveryProvider interface {
	RecoveryForProgressionPrerequisite(ProgressID, Observation) (PrerequisiteRecoveryLink, bool)
}

// prerequisiteRecovery turns typed semantic prerequisite evidence into one
// deterministic recovery objective. Route capabilities keep their existing
// planner-visible links; progression facts are matched directly to an offered
// progress objective or through adapter-owned recovery links.
//
// Exactly one prerequisite is repaired per call. Completed progression facts
// are pruned from the pending list against the fresh observation, so a failure
// that reports several missing facts becomes a sequence of re-observed,
// individually verified objective transactions rather than one stale plan.
func (f *runFailurePolicy) prerequisiteRecovery(
	obs Observation,
	offered []Objective,
	providers ...ProgressionPrerequisiteRecoveryProvider,
) (Objective, []Prerequisite, bool) {
	if f == nil || len(f.pendingPrerequisites) == 0 {
		return Objective{}, nil, false
	}

	// Prune prerequisites satisfied by the previous recovery transaction.
	// Direct field-capability requirements, like progression facts, survive a
	// successful recovery objective long enough to be re-observed here. Route
	// capability prerequisites retain their historical lifetime semantics and
	// are cleared by success().
	pending := f.pendingPrerequisites[:0]
	for _, prerequisite := range f.pendingPrerequisites {
		if prerequisite.Progress != "" && obs.Story.Has(prerequisite.Progress) {
			continue
		}
		if prerequisite.FieldCapability != "" && fieldCapabilityUsable(obs, prerequisite.FieldCapability) {
			continue
		}
		pending = append(pending, prerequisite)
	}
	f.pendingPrerequisites = pending
	if len(f.pendingPrerequisites) == 0 {
		return Objective{}, nil, false
	}

	var provider ProgressionPrerequisiteRecoveryProvider
	if len(providers) > 0 {
		provider = providers[0]
	}

	routePrerequisiteFor := map[CapabilityID]RoutePrerequisiteLink{}
	for _, blockage := range obs.RouteBlockages {
		for _, prerequisite := range blockage.Prerequisites {
			if prerequisite.Capability == "" {
				continue
			}
			routePrerequisiteFor[prerequisite.Capability] = prerequisite
		}
	}

	// Preserve normalized failure-context order. Repair one semantic
	// prerequisite at a time and rebuild the offered menu from live state.
	for _, prerequisite := range f.pendingPrerequisites {
		if prerequisite.Progress != "" {
			// The generic direct mapping needs no game knowledge: a progress
			// objective for the exact missing fact is its natural owner.
			for _, objective := range offered {
				if objective.Kind == KindProgress && objective.Progress == prerequisite.Progress {
					return objective, []Prerequisite{prerequisite}, true
				}
			}

			// Non-progress owners (for example a gym objective that earns a
			// badge fact) and recovery-only synthesis stay adapter-owned.
			if provider != nil {
				link, ok := provider.RecoveryForProgressionPrerequisite(prerequisite.Progress, obs)
				if ok {
					for _, objective := range offered {
						if objective.Key() == link.Objective.Key() {
							return objective, []Prerequisite{prerequisite}, true
						}
					}
					if link.RecoverySafe {
						return link.Objective, []Prerequisite{prerequisite}, true
					}
				}
			}
			continue
		}

		if prerequisite.FieldCapability != "" {
			if fieldCapabilityRepairReady(obs, prerequisite.FieldCapability) {
				return Objective{
					Kind:            KindRepairFieldCapability,
					FieldCapability: prerequisite.FieldCapability,
				}, []Prerequisite{prerequisite}, true
			}
			continue
		}

		if prerequisite.Capability == "" {
			continue
		}
		route, ok := routePrerequisiteFor[prerequisite.Capability]
		if !ok {
			continue
		}

		if route.Progress != "" {
			for _, objective := range offered {
				if objective.Kind == KindProgress && objective.Progress == route.Progress {
					return objective, []Prerequisite{prerequisite}, true
				}
			}
			if route.RecoveryOnly {
				return Objective{Kind: KindProgress, Progress: route.Progress}, []Prerequisite{prerequisite}, true
			}
		}

		if route.FieldCapability != "" && fieldCapabilityRepairReady(obs, route.FieldCapability) {
			return Objective{
				Kind:            KindRepairFieldCapability,
				FieldCapability: route.FieldCapability,
			}, []Prerequisite{prerequisite}, true
		}
	}

	return Objective{}, nil, false
}

func fieldCapabilityUsable(obs Observation, capability CapabilityID) bool {
	for _, field := range obs.FieldCapabilities {
		if field.Name == capability {
			return field.Usable
		}
	}
	return false
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
