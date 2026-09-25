package agent

// ProgressionPlanner is the game-specific half of progression planning. Core
// offering knows only that these are semantic state changes; a concrete game
// decides which goals are currently meaningful and executable.
type ProgressionPlanner interface {
	ProgressionObjectives(Observation) []Objective
}

type progressionObjectiveProvider struct {
	planner ProgressionPlanner
}

func (progressionObjectiveProvider) Family() ObjectiveFamily { return ObjectiveFamilyProgression }

func (p progressionObjectiveProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	if p.planner == nil || ctx == nil {
		return objectiveProviderResult{}
	}
	return objectiveProviderResult{Candidates: p.planner.ProgressionObjectives(ctx.obs)}
}

// withoutKnownUnroutableJourneys removes only travel objectives the live route
// planner has positively rejected. Generic Offer deliberately fails open when
// every journey is reported unroutable because a bad live-map overlay must not
// be able to empty the menu. Once a real progression objective exists, however,
// keeping those rejected journeys just gives the strategist known-bad choices
// instead of the semantic state change that can actually unlock the route.
func withoutKnownUnroutableJourneys(out []Objective, unroutable []string) []Objective {
	if len(unroutable) == 0 {
		return out
	}
	blocked := make(map[PlaceID]bool, len(unroutable))
	for _, name := range unroutable {
		blocked[PlaceID(name)] = true
	}
	filtered := make([]Objective, 0, len(out))
	for _, o := range out {
		if o.Kind == KindGoTo && blocked[o.Place] {
			continue
		}
		filtered = append(filtered, o)
	}
	return filtered
}

// OfferWithProgressionEvidence composes the portable provider menu with one
// game-owned progression provider. Adapter-owned route requirements and typed
// objective catalogs are projected before generic providers run, so provider
// policy never has to discover Red maps, gyms, starters, shops or encounters.
func OfferWithProgressionEvidence(obs Observation, known *Knowledge, p ProgressionPlanner) ObjectiveOffer {
	if requirements, ok := p.(RouteRequirementProvider); ok && requirements != nil {
		obs = observationWithRouteRequirements(obs, requirements.RouteRequirements(obs))
	}
	if catalogs, ok := p.(ObjectiveCatalogProvider); ok && catalogs != nil {
		obs.Catalog = catalogs.ObjectiveCatalog(obs)
	}
	base := OfferWithEvidence(obs, known)
	if p == nil {
		return base
	}
	if known == nil {
		known = NewKnowledge(nil)
	}
	ctx := newObjectiveOfferContext(obs, known)
	provided := (progressionObjectiveProvider{planner: p}).Provide(ctx)
	progress := annotate(provided.Candidates, known)
	base.Blocked = append(base.Blocked, provided.Blocked...)
	if len(progress) == 0 {
		return base
	}

	base.Candidates = withoutKnownUnroutableJourneys(base.Candidates, obs.Unroutable)
	journeyAt := len(base.Candidates)
	for i, o := range base.Candidates {
		if o.Kind == KindGoTo {
			journeyAt = i
			break
		}
	}
	combined := make([]Objective, 0, len(base.Candidates)+len(progress))
	combined = append(combined, base.Candidates[:journeyAt]...)
	combined = append(combined, progress...)
	combined = append(combined, base.Candidates[journeyAt:]...)
	base.Candidates = combined
	base.Readiness = challengeReadinessForOffer(obs, known, base)
	return base
}

// OfferWithProgression is the presentation-compatible candidate-only API.
func OfferWithProgression(obs Observation, known *Knowledge, p ProgressionPlanner) []Objective {
	return OfferWithProgressionEvidence(obs, known, p).Candidates
}

func (a *redObjectiveAdapter) ProgressionObjectives(obs Observation) []Objective {
	out := redRequireRainbowForPostCeladon(obs, redProgressionObjectives(obs))
	out = append(out, redCascadeBadgeObjectives(obs)...)
	return append(out, redMarshBadgeObjectives(obs)...)
}
