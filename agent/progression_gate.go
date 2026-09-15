package agent

// RouteRequirementProvider is the game-owned source of semantic route/story
// requirements. Generic offering only consumes the resulting portable
// RouteBlockage values; it never needs native map ids, badge enums, event bits,
// or named story rules from a concrete game.
type RouteRequirementProvider interface {
	RouteRequirements(Observation) []RouteBlockage
}

// observationWithRouteRequirements merges adapter-owned semantic requirements
// into the live route blockage view. Runtime route-planner evidence wins only
// by accumulation: multiple independent reasons may block the same destination
// and all prerequisite links remain visible to the strategist.
func observationWithRouteRequirements(obs Observation, requirements []RouteBlockage) Observation {
	if len(requirements) == 0 {
		return obs
	}
	seen := make(map[string]bool, len(obs.RouteBlockages)+len(requirements))
	key := func(b RouteBlockage) string {
		return string(b.Destination) + "\x00" + routeBlockageIdentity(b)
	}
	merged := append([]RouteBlockage(nil), obs.RouteBlockages...)
	for _, blockage := range merged {
		seen[key(blockage)] = true
	}
	for _, requirement := range requirements {
		if requirement.Destination == "" || seen[key(requirement)] {
			continue
		}
		seen[key(requirement)] = true
		merged = append(merged, requirement)
	}
	obs.RouteBlockages = merged
	return obs
}

func routeBlockageIdentity(blockage RouteBlockage) string {
	id := ""
	for _, transition := range blockage.Transitions {
		id += "t:" + transition + ";"
	}
	for _, missing := range blockage.Missing {
		id += "m:" + string(missing) + ";"
	}
	for _, prerequisite := range blockage.Prerequisites {
		id += "p:" + string(prerequisite.Capability) + ":" + string(prerequisite.FieldCapability) + ":" + string(prerequisite.Progress) + ":" + prerequisite.Badge + ";"
	}
	return id
}

func routePlaceBlocked(obs Observation, place PlaceID) bool {
	if place == "" || obs.Location == place {
		return false
	}
	for _, blockage := range obs.RouteBlockages {
		if blockage.Destination == place {
			return true
		}
	}
	return false
}
