package agent

// ProgressionPlanner is the game-specific half of progression planning. Core
// offering knows only that these are semantic state changes; a concrete game
// decides which goals are currently meaningful and executable.
type ProgressionPlanner interface {
	ProgressionObjectives(Observation) []Objective
}

// CombatRecoveryProvider reports whether the game owns a proven preparation
// corridor (an executable wild-grind site) at the current story boundary. A
// game without one cannot offer training after a combat loss, so the generic
// combat gate must promote the loss to retry instead of withholding the fight
// forever (run-36loaokprjdtw2j8om666lswgf: first-badge loss before any corridor
// is proven). Games that never offer combat recovery do not implement it; the
// gate then falls back to the sealed-topology escape alone.
type CombatRecoveryProvider interface {
	CombatRecoveryAvailable(obs Observation) bool
}

// ObjectiveSupportProvider lets a game whose controller covers only part of
// the portable objective vocabulary withhold generic offers it cannot execute,
// instead of the planner spending a decision on a guaranteed validation
// failure (run-2hib8c1hui0mz2dj8xi6egl81b: Gold offered "use a POTION").
type ObjectiveSupportProvider interface {
	SupportsObjective(Objective) bool
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
	offer := offerWithProgressionEvidence(obs, known, p)
	if support, ok := p.(ObjectiveSupportProvider); ok && support != nil {
		kept := offer.Candidates[:0:0]
		for _, o := range offer.Candidates {
			if support.SupportsObjective(o) {
				kept = append(kept, o)
			}
		}
		offer.Candidates = kept
	}
	return offer
}

func offerWithProgressionEvidence(obs Observation, known *Knowledge, p ProgressionPlanner) ObjectiveOffer {
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
	// Progression is appended after OfferWithEvidence filtered the provider
	// menu, so it needs the same combat-loss gate: without it a progression
	// battle lost with an unchanged party is re-offered every round
	// (run-1auv5rq62ou1i16n25pxcc2izv: 70 Burned Tower blackouts).
	progress := annotate(filterCombatRecoveryBlocked(provided.Candidates, known), known)
	base.Blocked = append(base.Blocked, provided.Blocked...)
	if len(progress) == 0 {
		// Sealed corridors (Elite Four rooms, one-way script pockets) report live
		// route refusals while local grass is absent, so the combat-loss gate
		// empties both menus and the run dies with "nothing is possible from
		// here" (run-a4wn4o17ztx91zgnezctug9t3). An ordinary commute with an
		// empty synthetic menu must keep the fight locked — only a sealed
		// topology (unroutable / route-blockage evidence) plus no remaining
		// preparation candidate earns the retry promotion.
		//
		// A game that owns no proven preparation corridor at this story boundary
		// is the same dead end from the other side: the fight is withheld, no
		// training can be offered, and the base menu is empty. The game answers
		// for its own corridor, so the gate only asks whether one exists
		// (run-36loaokprjdtw2j8om666lswgf: first-badge loss before Slowpoke Well).
		sealed := len(obs.Unroutable) > 0 || len(obs.RouteBlockages) > 0
		noRecoveryCorridor := false
		if provider, ok := p.(CombatRecoveryProvider); ok && provider != nil {
			noRecoveryCorridor = !provider.CombatRecoveryAvailable(obs)
		}
		// A sealed topology only fail-opens when the menu is otherwise empty:
		// journeys may still be real preparation. A game-reported missing
		// recovery corridor means preparation cannot advance this loss at all
		// (no site yet, or Gen2 lead-only grind already at the readiness
		// ceiling), so promote even when unrelated base candidates remain
		// (run-3v7t9gqr8gnks2efu8haxz0j2c).
		canPromote := owesCombatReadiness(known, obs) && !combatPreparationNeedsCoverage(obs, known)
		if canPromote && (noRecoveryCorridor || (sealed && len(base.Candidates) == 0)) {
			known.promoteCombatLossesToRetry()
			progress = annotate(filterCombatRecoveryBlocked(provided.Candidates, known), known)
		}
		if len(progress) == 0 {
			return base
		}
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
