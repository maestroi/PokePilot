package agent

// teamBuilderRosterSignal adds the mode-specific lifecycle that the generic
// party drive deliberately does not own: fill all six slots first, then bring
// the weakest members up with the rest of the team. It only scores objectives
// that are already legal/offered; catch, travel, shopping and training
// execution stay in the shared deterministic runtime.
func teamBuilderRosterSignal(obs Observation, o Objective, profile PlayStyleProfile) NaturalPlaySignal {
	if profile.Name != PlayStyleTeamBuilder {
		return NaturalPlaySignal{}
	}

	const rosterTarget = 6
	const fillScale = 1.55
	const balanceScale = 1.25

	count := obs.PartyCount
	if len(obs.Party) > count {
		count = len(obs.Party)
	}
	if count > rosterTarget {
		count = rosterTarget
	}

	var s NaturalPlaySignal
	add := func(tag string, bonus float64) {
		bonus *= profile.NaturalPlayScale
		if bonus <= 0 {
			return
		}
		s.Tags = appendNaturalTag(s.Tags, tag)
		s.Bonus += bonus
	}
	penalize := func(tag string, penalty float64) {
		penalty *= profile.NaturalPlayScale
		if penalty <= 0 {
			return
		}
		s.Tags = appendNaturalTag(s.Tags, tag)
		s.RepeatPenalty += penalty
	}

	if count < rosterTarget {
		missing := rosterTarget - count
		switch o.Kind {
		case KindCatch:
			// A 3-5 member roster is still incomplete in Team Builder. Keep this
			// strong enough to beat repeatedly polishing the existing core, even
			// when the catch requires a reasonable cross-map detour.
			bonus := 0.70 + float64(missing-1)*0.04
			if bonus > 0.86 {
				bonus = 0.86
			}
			add("fill-roster", bonus*fillScale)

			// Prefer catches that can contribute soon instead of blindly filling
			// slots with the first six species. Encounter level and evolution
			// potential are deterministic catalog facts; broader type/role fit is
			// left visible to the planner rather than hard-coding favorite species.
			if level := teamBuilderCatchLevel(obs, o); level > 0 {
				weak := weakestPartyLevel(obs)
				if weak == 0 || int(level)+2 >= weak {
					add("battle-ready-catch", 0.12*balanceScale)
				}
			}
			if teamBuilderEvolutionUpside(obs, o.Species) {
				add("evolution-upside", 0.08*balanceScale)
			}

		case KindTrain:
			// Training remains legal and can still win when necessary, but it
			// should not consume the whole run while three roster slots are empty.
			penalty := 0.42 + float64(missing-1)*0.04
			if penalty > 0.58 {
				penalty = 0.58
			}
			penalize("fill-roster-first", penalty*fillScale)

		case KindBuy:
			if spec, ok := ItemEconomy(string(o.Item)); ok && spec.Category == InventoryCapture {
				add("roster-supplies", 0.30*fillScale)
			}

		case KindGoTo:
			if naturalMartPlace(string(o.Place)) && normalBallStock(obs) < minimumCaptureStock {
				add("roster-resupply", 0.34*fillScale)
			}
		}
		return s
	}

	if o.Kind != KindTrain {
		return s
	}
	mon, ok := naturalTrainingTarget(obs, o)
	if !ok {
		return s
	}
	lead := 0
	for _, member := range obs.Party {
		if int(member.Level) > lead {
			lead = int(member.Level)
		}
	}
	gap := lead - int(mon.Level)
	switch {
	case gap >= 4:
		add("balance-full-roster", 0.34*balanceScale)
	case gap >= 2:
		add("balance-full-roster", 0.24*balanceScale)
	case int(mon.Level) == weakestPartyLevel(obs):
		add("develop-full-roster", 0.12*balanceScale)
	}
	return s
}

func teamBuilderCatchLevel(obs Observation, o Objective) uint8 {
	if o.Kind != KindCatch || o.Species == "" {
		return 0
	}
	var best uint8
	for _, wild := range obs.WildGrass {
		sp, ok := SpeciesByName(wild.Name)
		if ok && sp == o.Species && wild.MaxLevel > best {
			best = wild.MaxLevel
		}
	}
	for _, entry := range obs.Dex.Targets {
		if entry.Species != o.Species {
			continue
		}
		for _, src := range entry.Sources {
			if src.Kind != AcquireWildGrass {
				continue
			}
			if o.Place != "" && src.Place != o.Place {
				continue
			}
			if src.Level > best {
				best = src.Level
			}
		}
	}
	return best
}

func teamBuilderEvolutionUpside(obs Observation, species SpeciesID) bool {
	if species == "" {
		return false
	}
	for _, entry := range obs.Dex.Targets {
		for _, src := range entry.Sources {
			if src.From != species {
				continue
			}
			if src.Kind == AcquireLevelEvo || src.Kind == AcquireItemEvo {
				return true
			}
		}
	}
	return false
}

func weakestPartyLevel(obs Observation) int {
	weakest := 0
	for _, mon := range obs.Party {
		level := int(mon.Level)
		if level <= 0 {
			continue
		}
		if weakest == 0 || level < weakest {
			weakest = level
		}
	}
	return weakest
}

func mergeNaturalPlaySignal(base, extra NaturalPlaySignal) NaturalPlaySignal {
	base.Bonus += extra.Bonus
	base.RepeatPenalty += extra.RepeatPenalty
	for _, tag := range extra.Tags {
		base.Tags = appendNaturalTag(base.Tags, tag)
	}
	return base
}
